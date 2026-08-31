package oaktree

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type TmuxSession struct {
	Name        string
	ID          string
	LeftPaneID  string
	RightPaneID string
}

type TmuxPane struct {
	ID      string
	Command string
	Path    string
}

func NewSessionCommandArgs(sessionName, workdir string, command []string) []string {
	args := []string{"new-session", "-d", "-P", "-F", "#{session_id}\t#{pane_id}", "-s", sessionName, "-c", workdir}
	args = append(args, command...)
	return args
}

func SplitWindowCommandArgs(target, workdir string, command []string) []string {
	args := []string{"split-window", "-h", "-b", "-p", "60", "-t", target, "-c", workdir, "-P", "-F", "#{pane_id}"}
	args = append(args, command...)
	return args
}

func PaneIdCommandArgs(target string) []string {
	return []string{"display-message", "-p", "-t", target, "#{pane_id}"}
}

func PaneSessionIdCommandArgs(target string) []string {
	return []string{"display-message", "-p", "-t", target, "#{session_id}"}
}

func CapturePaneArgs(target string, lines int) []string {
	return []string{"capture-pane", "-p", "-J", "-t", target, "-S", fmt.Sprintf("-%d", lines)}
}

func ListPanesArgs(target string) []string {
	return []string{"list-panes", "-s", "-t", target, "-F", "#{pane_id}\t#{pane_current_command}\t#{pane_current_path}"}
}

func KillSessionArgs(target string) []string {
	return []string{"kill-session", "-t", target}
}

func HasSessionArgs(target string) []string {
	return []string{"has-session", "-t", target}
}

func ListSessionNamesArgs() []string {
	return []string{"list-sessions", "-F", "#{session_name}"}
}

func SwitchSessionArgs(target string) []string {
	return []string{"switch-client", "-t", target}
}

func CurrentSessionArgs() []string {
	return []string{"display-message", "-p", "#{client_session}"}
}

func AttachSessionArgs(target string) []string {
	return []string{"attach-session", "-t", target}
}

func DisplayPopupArgs(binary string) []string {
	return []string{"display-popup", "-E", "-w", "90%", "-h", "90%", "--", binary}
}

func CreateTmuxSession(ctx context.Context, runner Runner, sessionName, workdir string) (TmuxSession, error) {
	return CreateTmuxSessionWithCommand(ctx, runner, sessionName, workdir, []string{"pi"})
}

func CreateTmuxSessionWithCommand(ctx context.Context, runner Runner, sessionName, workdir string, right []string) (TmuxSession, error) {
	left := []string{"nvim"}
	if len(right) == 0 {
		right = []string{"pi"}
	}
	idBytes, err := runner.Output(ctx, "tmux", NewSessionCommandArgs(sessionName, workdir, left)...)
	if err != nil {
		return TmuxSession{}, err
	}
	defer func() {
		if err != nil {
			_ = KillTmuxSession(ctx, runner, sessionName)
		}
	}()
	sessionID, leftPaneID, err := parseTmuxSessionOutput(idBytes)
	if err != nil {
		return TmuxSession{}, err
	}
	rightID, err := runner.Output(ctx, "tmux", SplitWindowCommandArgs(leftPaneID, workdir, right)...)
	if err != nil {
		return TmuxSession{}, err
	}
	return TmuxSession{
		Name:        sessionName,
		ID:          sessionID,
		LeftPaneID:  leftPaneID,
		RightPaneID: strings.TrimSpace(string(rightID)),
	}, nil
}

func parseTmuxSessionOutput(output []byte) (string, string, error) {
	fields := strings.SplitN(strings.TrimSpace(string(output)), "\t", 2)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("tmux new-session output did not include session and pane ids: %q", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), nil
}

func CapturePane(ctx context.Context, runner Runner, target string, lines int) (string, error) {
	data, err := runner.Output(ctx, "tmux", CapturePaneArgs(target, lines)...)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ListPanes(ctx context.Context, runner Runner, target string) ([]TmuxPane, error) {
	data, err := runner.Output(ctx, "tmux", ListPanesArgs(target)...)
	if err != nil {
		return nil, err
	}
	return parseTmuxPanes(data), nil
}

func parseTmuxPanes(output []byte) []TmuxPane {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	panes := make([]TmuxPane, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		panes = append(panes, TmuxPane{
			ID:      strings.TrimSpace(fields[0]),
			Command: strings.TrimSpace(fields[1]),
			Path:    strings.TrimSpace(fields[2]),
		})
	}
	return panes
}

func KillTmuxSession(ctx context.Context, runner Runner, target string) error {
	return runner.Run(ctx, "tmux", KillSessionArgs(target)...)
}

func HasTmuxSession(ctx context.Context, runner Runner, target string) bool {
	return runner.Run(ctx, "tmux", HasSessionArgs(target)...) == nil
}

func CurrentTmuxSession(ctx context.Context, runner Runner) (string, error) {
	if os.Getenv("TMUX") == "" {
		return "", nil
	}
	output, err := runner.Output(ctx, "tmux", CurrentSessionArgs()...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func SwitchTmuxSession(ctx context.Context, runner Runner, target string) error {
	return runner.Run(ctx, "tmux", SwitchSessionArgs(target)...)
}

func AttachOrSwitch(ctx context.Context, runner Runner, target string) error {
	if os.Getenv("TMUX") != "" {
		return SwitchTmuxSession(ctx, runner, target)
	}
	return runner.Run(ctx, "tmux", AttachSessionArgs(target)...)
}
