package oaktree

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEnsurePiExtensionContainsLifecycleAndQuestionTool(t *testing.T) {
	paths := Paths{StateDir: t.TempDir(), PiDir: filepath.Join(t.TempDir(), "pi")}
	path, err := EnsurePiExtension(paths)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"session_start", "agent_settled", "session_shutdown", "registerTool", "getAllTools", "question", "promptGuidelines: [", "promptSnippet:", "executionMode: \"sequential\"", "rpiv:ask-user:prompt", "ask_user_question", "tool_execution_end", "result.code === 0", "todoSummary", "message.toolName === \"todo\"", "event.toolName === \"todo\"", "todo_in_progress", "todo_json", "task.subject.trim()"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("extension missing %q", want)
		}
	}
	if strings.Contains(string(data), "subagent") {
		t.Fatal("extension must not integrate pi-subagents")
	}
	if strings.Contains(string(data), `"--quiet"`) {
		t.Fatal("extension must observe hook failures so startup events can retry")
	}
}

func TestPiExtensionPathDefaultsUnderStateDir(t *testing.T) {
	state := t.TempDir()
	if got, want := PiExtensionPath(Paths{StateDir: state}), filepath.Join(state, "pi", "oak-tree-extension.ts"); got != want {
		t.Fatalf("PiExtensionPath() = %q, want %q", got, want)
	}
}

func TestPiCommandUsesPrivateExtensionAndOakSessionIdentity(t *testing.T) {
	binDir := t.TempDir()
	piBinary := filepath.Join(binDir, "pi")
	if err := os.WriteFile(piBinary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	state := t.TempDir()
	paths := Paths{StateDir: state, PiDir: filepath.Join(state, "pi")}
	command, err := PiCommand(context.Background(), paths, "oak-session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(command) != 6 || command[0] != "env" || command[1] != "OAK_TREE_SESSION_ID=oak-session-1" || !strings.HasPrefix(command[2], "OAK_TREE_HOOK=") || command[3] != "pi" || command[4] != "-e" || command[5] != PiExtensionPath(paths) {
		t.Fatalf("PiCommand() = %#v", command)
	}
	if _, err := os.Stat(PiExtensionPath(paths)); err != nil {
		t.Fatalf("Pi extension not written: %v", err)
	}
}

func TestPiCommandReportsMissingPi(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	paths := Paths{StateDir: t.TempDir(), PiDir: filepath.Join(t.TempDir(), "pi")}
	_, err := PiCommand(context.Background(), paths, "oak-session-1")
	if err == nil || !strings.Contains(err.Error(), "Pi CLI is not installed") {
		t.Fatalf("PiCommand() error = %v, want missing Pi guidance", err)
	}
}

func TestHandleAgentEventUsesDirectOakIdentityAndPiStatus(t *testing.T) {
	state := t.TempDir()
	store := NewStore(state)
	session := Session{ID: "oak-1", Root: "/repo", Workdir: "/repo", TmuxSessionName: "oak-1", RightPaneID: "%2", CreatedAt: time.Now().UTC()}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	notifications := map[string]int{}
	runner := &stubRunner{runFunc: func(name string, args []string) error {
		if name == "notify-send" {
			if len(args) != 2 {
				t.Fatalf("notify-send args = %#v", args)
			}
			notifications[args[0]]++
		}
		return nil
	}}
	svc := NewService(Paths{StateDir: state}, store, runner)
	err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-1", Event: "question", Cwd: "/unrelated", SessionID: "pi-1", SessionFile: "/tmp/pi.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSession("oak-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentStatus != AgentStatusQuestion || len(got.AgentSessionIDs) != 1 || got.AgentSessionIDs[0] != "pi-1" || got.AgentSessionFile != "/tmp/pi.jsonl" {
		t.Fatalf("session = %#v", got)
	}
	if notifications["Pi question waiting"] != 1 {
		t.Fatalf("question notifications = %#v, want one", notifications)
	}

	for _, transition := range []struct {
		event string
		want  AgentStatus
	}{
		{event: "question_answered", want: AgentStatusWorking},
		{event: "agent_settled", want: AgentStatusAttention},
		{event: "session_shutdown", want: AgentStatusIdle},
	} {
		if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-1", Event: transition.event}); err != nil {
			t.Fatalf("HandleAgentEvent(%q): %v", transition.event, err)
		}
		got, err = store.LoadSession("oak-1")
		if err != nil {
			t.Fatal(err)
		}
		if got.AgentStatus != transition.want {
			t.Fatalf("status after %s = %q, want %q", transition.event, got.AgentStatus, transition.want)
		}
	}
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-1", Event: "agent_settled"}); err != nil {
		t.Fatal(err)
	}
	if notifications["Pi finished working"] != 1 {
		t.Fatalf("completion notifications = %#v, want one", notifications)
	}
}

func TestHandleAgentEventStoresTodoSummaryWithoutChangingAgentStatus(t *testing.T) {
	state := t.TempDir()
	store := NewStore(state)
	updatedAt := time.Now().UTC().Add(-time.Minute)
	if err := store.SaveSession(Session{ID: "oak-todo", RightPaneID: "%2", AgentStatus: AgentStatusWorking, AgentStatusUpdatedAt: &updatedAt, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: state}, store, &stubRunner{})
	summary := &TodoSummary{
		Total: 4, Pending: 2, InProgress: 1, Completed: 1,
		Tasks: []TodoTask{
			{Subject: "Pending one", Status: "pending"},
			{Subject: "Active task", Status: "in_progress"},
			{Subject: "Done task", Status: "completed"},
			{Subject: "Pending two", Status: "pending"},
		},
	}
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-todo", Event: "todo", Todo: summary}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSession("oak-todo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Todo == nil || !reflect.DeepEqual(got.Todo, summary) {
		t.Fatalf("todo summary = %#v, want %#v", got.Todo, summary)
	}
	if got.AgentStatus != AgentStatusWorking || got.AgentStatusUpdatedAt == nil || !got.AgentStatusUpdatedAt.Equal(updatedAt) {
		t.Fatalf("todo event changed agent status: %#v", got)
	}
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-todo", Event: "todo", Todo: &TodoSummary{Total: 2, Pending: 1}}); err == nil {
		t.Fatal("invalid todo summary was accepted")
	}
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "oak-todo", Event: "todo", Todo: &TodoSummary{Total: 1, Pending: 1, Tasks: []TodoTask{{Subject: "Wrong status", Status: "completed"}}}}); err == nil {
		t.Fatal("todo details inconsistent with counts were accepted")
	}
}

func TestHandleAgentEventWaitsForSessionPane(t *testing.T) {
	state := t.TempDir()
	store := NewStore(state)
	if err := store.SaveSession(Session{ID: "starting", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: state}, store, &stubRunner{})
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: "starting", Event: "session_start"}); err == nil || !strings.Contains(err.Error(), "still starting") {
		t.Fatalf("HandleAgentEvent() error = %v, want still-starting error", err)
	}
}

func TestPiQuestionStatusRemainsAuthoritativeWithoutMatchingPaneText(t *testing.T) {
	state := t.TempDir()
	store := NewStore(state)
	session := Session{
		ID:              "pi-question",
		Root:            "/repo",
		Workdir:         "/repo",
		TmuxSessionName: "oak-pi-question",
		RightPaneID:     "%2",
		AgentStatus:     AgentStatusQuestion,
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{outputFunc: func(name string, args []string) ([]byte, error) {
		if name == "tmux" && len(args) > 0 && args[0] == "capture-pane" {
			return []byte("Pi custom selection UI"), nil
		}
		return nil, nil
	}}
	svc := NewService(Paths{StateDir: state}, store, runner)
	sessions, err := svc.ListSessionsWithAgentStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].AgentStatus != AgentStatusQuestion {
		t.Fatalf("sessions = %#v, want authoritative Pi question", sessions)
	}
	persisted, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.AgentStatus != AgentStatusQuestion {
		t.Fatalf("persisted status = %q, want question", persisted.AgentStatus)
	}
}

func TestPaneLooksLikeAgentQuestionPiFallback(t *testing.T) {
	if !paneLooksLikeAgentQuestion("Please provide your input?\n") {
		t.Fatal("expected conservative Pi question match")
	}
	if paneLooksLikeAgentQuestion("finished successfully") {
		t.Fatal("unexpected Pi question match")
	}
}
