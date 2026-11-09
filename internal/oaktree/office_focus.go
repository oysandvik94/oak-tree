package oaktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var officeTmuxID = regexp.MustCompile(`^\$[0-9]+$`)
var officeClientTTY = regexp.MustCompile(`^/dev/pts/[0-9]+$`)
var officeWindowAddress = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)

type officeFocusTarget struct {
	tty     string
	pid     int
	window  string
	session string
}

type officeWindow struct {
	Address string `json:"address"`
	PID     int    `json:"pid"`
}

func officeProcessParents(pid int) ([]int, error) {
	var parents []int
	seen := map[int]bool{}
	for pid > 1 {
		if seen[pid] || len(parents) >= 128 {
			return nil, errors.New("invalid process ancestry")
		}
		seen[pid] = true
		parents = append(parents, pid)
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return nil, err
		}
		end := strings.LastIndexByte(string(data), ')')
		if end < 0 {
			return nil, errors.New("invalid process metadata")
		}
		fields := strings.Fields(string(data[end+1:]))
		if len(fields) < 2 {
			return nil, errors.New("invalid process metadata")
		}
		pid, err = strconv.Atoi(fields[1])
		if err != nil {
			return nil, err
		}
	}
	return parents, nil
}

func officeResolveFocus(ctx context.Context, runner Runner, session Session) (officeFocusTarget, error) {
	var target officeFocusTarget
	if !officeTmuxID.MatchString(session.TmuxSessionID) {
		return target, errors.New("managed session is unavailable")
	}
	out, err := runner.Output(ctx, "tmux", "display-message", "-p", "-t", session.TmuxSessionID, "#{session_id}\t#{@oak_tree_session_id}")
	if err != nil || strings.TrimSpace(string(out)) != session.TmuxSessionID+"\t"+session.ID {
		return target, errors.New("managed session is unavailable")
	}
	out, err = runner.Output(ctx, "tmux", "list-clients", "-F", "#{client_tty}\t#{client_pid}")
	if err != nil {
		return target, errors.New("tmux client is unavailable")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 1 {
		return target, errors.New("session focus requires exactly one tmux client")
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 2 || !officeClientTTY.MatchString(fields[0]) {
		return target, errors.New("tmux client is unavailable")
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil || pid <= 1 {
		return target, errors.New("tmux client is unavailable")
	}
	parents, err := officeProcessParents(pid)
	if err != nil {
		return target, errors.New("terminal process is unavailable")
	}
	out, err = runner.Output(ctx, "hyprctl", "-j", "clients")
	if err != nil {
		return target, errors.New("Hyprland windows are unavailable")
	}
	var windows []officeWindow
	if json.Unmarshal(out, &windows) != nil {
		return target, errors.New("Hyprland windows are unavailable")
	}
	for _, parent := range parents {
		var matches []string
		for _, window := range windows {
			if window.PID == parent && officeWindowAddress.MatchString(window.Address) {
				matches = append(matches, window.Address)
			}
		}
		if len(matches) > 1 {
			return target, errors.New("multiple terminal windows match the tmux client")
		}
		if len(matches) == 1 {
			return officeFocusTarget{tty: fields[0], pid: pid, window: matches[0], session: session.TmuxSessionID}, nil
		}
	}
	return target, errors.New("no terminal window matches the tmux client")
}

func officeFocusSession(ctx context.Context, store *Store, runner Runner, agentID string) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sessions, err := store.ListSessions()
	if err != nil {
		return 500, "Could not load sessions."
	}
	session, _, ok := officeAgentSession(sessions, agentID)
	if !ok {
		return 404, "Session is no longer available."
	}
	target, err := officeResolveFocus(ctx, runner, session)
	if err != nil {
		return 409, err.Error() + "."
	}
	// Resolve again immediately before switching; never trust an old PID, tty, or window.
	sessions, err = store.ListSessions()
	if err != nil {
		return 500, "Could not load sessions."
	}
	currentSession, _, ok := officeAgentSession(sessions, agentID)
	if !ok || currentSession.ID != session.ID || currentSession.TmuxSessionID != session.TmuxSessionID {
		return 409, "Session or terminal changed before it could be opened."
	}
	current, err := officeResolveFocus(ctx, runner, currentSession)
	if err != nil || current != target {
		return 409, "Session or terminal changed before it could be opened."
	}
	if err := runner.Run(ctx, "tmux", "switch-client", "-c", target.tty, "-t", target.session); err != nil {
		return 502, "Could not switch the tmux session."
	}
	out, err := runner.Output(ctx, "hyprctl", "dispatch", `hl.dsp.focus({ window = "address:`+target.window+`" })`)
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		return 502, "The tmux session was switched, but the terminal window could not be focused."
	}
	return 200, ""
}
