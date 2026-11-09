package oaktree

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func officeQuestionFixture(t *testing.T) (*Store, Session, officeBridgeIdentity) {
	t.Helper()
	dir, err := os.MkdirTemp("", "oakq-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	store := NewStore(dir)
	session := Session{ID: "s", Root: "/fake/proj", AgentStatus: AgentStatusQuestion, Agents: map[string]PairedAgent{"architect": {SessionID: "pi-1", SessionFile: "/tmp/pi-session", PaneID: "%1", Status: AgentStatusQuestion}}}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	return store, session, officeBridgeIdentity{ManagedSessionID: "s", Role: "architect", PiSessionID: "pi-1", PaneID: "%1", SessionFile: "/tmp/pi-session"}
}

func officeTestBridge(t *testing.T, store *Store, identity officeBridgeIdentity, questionID string, answer string) {
	t.Helper()
	dir := filepath.Join(store.StateDir, "office-bridges")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, strings.Repeat("a", 24)+".sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var req map[string]json.RawMessage
				if json.NewDecoder(bufio.NewReader(conn)).Decode(&req) != nil {
					return
				}
				var op string
				_ = json.Unmarshal(req["op"], &op)
				if op == "list" {
					_ = json.NewEncoder(conn).Encode(officeBridgeReply{OK: true, Identity: identity, Flows: []officeQuestion{{ID: questionID, Source: "tool", CreatedAt: 123, Questions: json.RawMessage(`[{"id":"q","prompt":"Choose","options":[{"value":"yes","label":"Yes"}]}]`)}}})
					return
				}
				if answer == "ok" {
					_ = json.NewEncoder(conn).Encode(officeBridgeReply{OK: true})
				} else {
					_ = json.NewEncoder(conn).Encode(officeBridgeReply{Error: answer})
				}
			}()
		}
	}()
}

func TestOfficeBridgeMatchesExactCurrentIdentity(t *testing.T) {
	_, session, identity := officeQuestionFixture(t)
	if !officeBridgeMatches(session, "architect", identity) {
		t.Fatal("matching bridge rejected")
	}
	for _, mutate := range []func(*officeBridgeIdentity){func(i *officeBridgeIdentity) { i.PiSessionID = "old" }, func(i *officeBridgeIdentity) { i.PaneID = "%2" }, func(i *officeBridgeIdentity) { i.SessionFile = "old" }, func(i *officeBridgeIdentity) { i.Role = "junior" }, func(i *officeBridgeIdentity) { i.ManagedSessionID = "other" }} {
		copy := identity
		mutate(&copy)
		if officeBridgeMatches(session, "architect", copy) {
			t.Fatalf("mismatched identity accepted: %+v", copy)
		}
	}
	if _, ok := officeMatchingBridge(session, "architect", []officeBridge{{Identity: identity}, {Identity: identity}}); ok {
		t.Fatal("ambiguous bridges accepted")
	}
}

func TestOfficeQuestionsExposeOnlyLiveMatchingBridge(t *testing.T) {
	store, session, identity := officeQuestionFixture(t)
	id := strings.Repeat("1", 48)
	officeTestBridge(t, store, identity, id, "ok")
	result := officeSnapshotFrom([]Session{session}, nil, nil, time.Now())
	officeAttachQuestions(&result, []Session{session}, officeBridges(store.StateDir))
	if len(result.Agents) != 1 || len(result.Agents[0].Questions) != 1 || result.Agents[0].Questions[0].ID != id {
		t.Fatalf("snapshot=%+v", result)
	}
	identity.PaneID = "%stale"
	officeAttachQuestions(&result, []Session{session}, []officeBridge{{Identity: identity, Flows: []officeQuestion{{ID: id}}}})
	if len(result.Agents[0].Questions) != 0 {
		t.Fatal("stale bridge exposed questions")
	}
}

func TestOfficeToolFailuresRequireUniqueMatchingBridge(t *testing.T) {
	_, session, identity := officeQuestionFixture(t)
	snapshot := officeSnapshotFrom([]Session{session}, nil, nil, time.Now())
	bridge := officeBridge{Identity: identity, ToolFailures: &officeToolFailures{Generation: "generation", Count: 3}}
	officeAttachQuestions(&snapshot, []Session{session}, []officeBridge{bridge})
	if got := snapshot.Agents[0].ToolFailures; got == nil || got.Count != 3 {
		t.Fatalf("missing failure count: %+v", got)
	}
	officeAttachQuestions(&snapshot, []Session{session}, []officeBridge{bridge, bridge})
	if snapshot.Agents[0].ToolFailures != nil {
		t.Fatal("ambiguous bridge exposed failure count")
	}
	bridge.Identity.PaneID = "%stale"
	officeAttachQuestions(&snapshot, []Session{session}, []officeBridge{bridge})
	if snapshot.Agents[0].ToolFailures != nil {
		t.Fatal("stale bridge exposed failure count")
	}
}

func TestOfficeSubmitAnswer(t *testing.T) {
	for _, tc := range []struct {
		reply  string
		status int
	}{{"ok", 200}, {"invalid", 422}, {"closed", 409}, {"uncertain", 503}} {
		t.Run(tc.reply, func(t *testing.T) {
			store, _, identity := officeQuestionFixture(t)
			id := strings.Repeat("2", 48)
			officeTestBridge(t, store, identity, id, tc.reply)
			request := officeAnswerRequest{AgentID: "s:architect", QuestionID: id, Response: json.RawMessage(`{"kind":"answer","answers":{"q":{"values":["yes"]}}}`)}
			status, _ := officeSubmitAnswer(store, request)
			if status != tc.status {
				t.Fatalf("status=%d want %d", status, tc.status)
			}
			request.QuestionID = strings.Repeat("3", 48)
			status, _ = officeSubmitAnswer(store, request)
			if status != 409 {
				t.Fatalf("unknown question status=%d", status)
			}
		})
	}
}

func TestOfficeBridgesRejectSymlinkDirectory(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "office-bridges")); err != nil {
		t.Fatal(err)
	}
	if got := officeBridges(dir); len(got) != 0 {
		t.Fatal("symlinked bridge directory was accepted")
	}
}

func TestOfficeBridgeSocketNameIsBounded(t *testing.T) {
	for _, name := range []string{"../x.sock", "not-a-random-socket.sock", fmt.Sprintf("%s.sock/extra", strings.Repeat("a", 24))} {
		if officeSocketName.MatchString(name) {
			t.Fatalf("unexpected socket name %q", name)
		}
	}
}
