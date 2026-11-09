package oaktree

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const officeBridgeMaxBytes = 262144

var officeSocketName = regexp.MustCompile(`^[0-9a-f]{24}\.sock$`)

type officeQuestion struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Source    string          `json:"source"`
	CreatedAt int64           `json:"createdAt"`
	Questions json.RawMessage `json:"questions"`
}

type officeBridgeIdentity struct {
	ManagedSessionID string `json:"managedSessionId"`
	Role             string `json:"role"`
	PiSessionID      string `json:"piSessionId"`
	PaneID           string `json:"paneId"`
	SessionFile      string `json:"sessionFile"`
}

type officeToolFailures struct {
	Generation string `json:"generation"`
	Count      uint64 `json:"count"`
}

type officeBridge struct {
	path         string
	Identity     officeBridgeIdentity `json:"identity"`
	Flows        []officeQuestion     `json:"flows"`
	ToolFailures *officeToolFailures  `json:"toolFailures"`
}

type officeBridgeReply struct {
	OK           bool                 `json:"ok"`
	Error        string               `json:"error"`
	Identity     officeBridgeIdentity `json:"identity"`
	Flows        []officeQuestion     `json:"flows"`
	ToolFailures *officeToolFailures  `json:"toolFailures"`
}

func officeBridgeRequest(socket string, request any, timeout time.Duration) (officeBridgeReply, error) {
	var reply officeBridgeReply
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return reply, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err = json.NewEncoder(conn).Encode(request); err != nil {
		return reply, err
	}
	line, err := bufio.NewReader(io.LimitReader(conn, officeBridgeMaxBytes+1)).ReadBytes('\n')
	if err != nil || len(line) > officeBridgeMaxBytes {
		return reply, errors.New("bridge unavailable")
	}
	if err = json.Unmarshal(line, &reply); err != nil {
		return reply, errors.New("invalid bridge response")
	}
	return reply, nil
}

func officeBridges(stateDir string) []officeBridge {
	dir := filepath.Join(stateDir, "office-bridges")
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if len(paths) >= 64 {
			break
		}
		if !officeSocketName.MatchString(entry.Name()) || entry.Type()&os.ModeSocket == 0 {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var bridges []officeBridge
	for _, socket := range paths {
		wg.Add(1)
		go func(socket string) {
			defer wg.Done()
			reply, err := officeBridgeRequest(socket, map[string]string{"op": "list"}, 400*time.Millisecond)
			if err != nil || !reply.OK {
				return
			}
			mu.Lock()
			bridges = append(bridges, officeBridge{path: socket, Identity: reply.Identity, Flows: reply.Flows, ToolFailures: reply.ToolFailures})
			mu.Unlock()
		}(socket)
	}
	wg.Wait()
	return bridges
}

func officeBridgeMatches(session Session, role string, identity officeBridgeIdentity) bool {
	if identity.PiSessionID == "" || identity.PaneID == "" || identity.Role != role || (identity.ManagedSessionID != "" && identity.ManagedSessionID != session.ID) {
		return false
	}
	if role != "" {
		a, ok := session.Agents[role]
		return ok && a.PaneID == identity.PaneID && a.SessionID == identity.PiSessionID && a.SessionFile != "" && a.SessionFile == identity.SessionFile
	}
	if len(session.Agents) != 0 || session.RightPaneID != identity.PaneID || session.AgentSessionFile == "" || session.AgentSessionFile != identity.SessionFile {
		return false
	}
	for _, id := range session.AgentSessionIDs {
		if id == identity.PiSessionID {
			return true
		}
	}
	return false
}

func officeMatchingBridge(session Session, role string, bridges []officeBridge) (officeBridge, bool) {
	var match officeBridge
	count := 0
	for _, bridge := range bridges {
		if officeBridgeMatches(session, role, bridge.Identity) {
			match = bridge
			count++
		}
	}
	return match, count == 1
}

func officeAgentSession(sessions []Session, agentID string) (Session, string, bool) {
	var found Session
	var role string
	count := 0
	for _, session := range sessions {
		if len(session.Agents) == 0 && session.ID == agentID {
			found = session
			role = ""
			count++
		}
		for r := range session.Agents {
			if session.ID+":"+r == agentID {
				found = session
				role = r
				count++
			}
		}
	}
	return found, role, count == 1
}

func officeAttachQuestions(snapshot *officeSnapshot, sessions []Session, bridges []officeBridge) {
	for i := range snapshot.Agents {
		a := &snapshot.Agents[i]
		a.Questions = []officeQuestion{}
		a.ToolFailures = nil
		session, role, ok := officeAgentSession(sessions, a.ID)
		if !ok {
			continue
		}
		bridge, ok := officeMatchingBridge(session, role, bridges)
		if !ok {
			continue
		}
		a.ToolFailures = bridge.ToolFailures
		a.Questions = bridge.Flows
		if a.Questions == nil {
			a.Questions = []officeQuestion{}
		}
	}
}

type officeAnswerRequest struct {
	AgentID    string          `json:"agentId"`
	QuestionID string          `json:"questionId"`
	Response   json.RawMessage `json:"response"`
}

// officeSubmitAnswer makes one delivery attempt; failures after dispatch are uncertain.
func officeSubmitAnswer(store *Store, request officeAnswerRequest) (int, string) {
	if request.AgentID == "" || len(request.AgentID) > 1024 || len(request.QuestionID) != 48 {
		return 422, "invalid request"
	}
	if _, err := hex.DecodeString(request.QuestionID); err != nil {
		return 422, "invalid request"
	}
	var response struct {
		Kind    string          `json:"kind"`
		Mode    string          `json:"mode"`
		Answers json.RawMessage `json:"answers"`
	}
	if len(request.Response) == 0 || json.Unmarshal(request.Response, &response) != nil || (response.Kind != "answer" && response.Kind != "cancel") || (response.Mode != "" && response.Mode != "submit" && response.Mode != "elaborate") || (response.Kind == "answer" && len(response.Answers) == 0) {
		return 422, "invalid request"
	}
	sessions, err := store.ListSessions()
	if err != nil {
		return 503, "office unavailable"
	}
	session, role, ok := officeAgentSession(sessions, request.AgentID)
	if !ok {
		return 409, "question is no longer available"
	}
	bridge, ok := officeMatchingBridge(session, role, officeBridges(store.StateDir))
	if !ok {
		return 503, "question bridge unavailable"
	}
	found := false
	for _, flow := range bridge.Flows {
		if flow.ID == request.QuestionID {
			found = true
			break
		}
	}
	if !found {
		return 409, "question is no longer available"
	}
	// Revalidate membership just before sending to the bound live process.
	current, err := store.ListSessions()
	if err != nil {
		return 503, "office unavailable"
	}
	session, role, ok = officeAgentSession(current, request.AgentID)
	if !ok || !officeBridgeMatches(session, role, bridge.Identity) {
		return 409, "question is no longer available"
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return 503, "office unavailable"
	}
	payload := map[string]any{"op": "submit", "id": request.QuestionID, "requestId": hex.EncodeToString(id[:]), "identity": bridge.Identity, "response": request.Response}
	reply, err := officeBridgeRequest(bridge.path, payload, 2200*time.Millisecond)
	if err != nil {
		return 503, "delivery uncertain"
	}
	if reply.OK {
		return 200, ""
	}
	switch reply.Error {
	case "invalid":
		return 422, "invalid answer"
	case "closed", "busy":
		return 409, "question is no longer available"
	default:
		return 503, "delivery uncertain"
	}
}

func officeJSONError(w io.Writer, message string) {
	code := "invalid"
	switch message {
	case "question is no longer available":
		code = "closed"
	case "office unavailable", "question bridge unavailable":
		code = "unavailable"
	case "delivery uncertain":
		code = "uncertain"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": code, "message": message})
}
