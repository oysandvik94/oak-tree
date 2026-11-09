package oaktree

import (
	"context"
	"testing"
)

func TestPairedAgentEventsAggregateAndSelectPane(t *testing.T) {
	store := NewStore(t.TempDir())
	session := Session{
		ID: "pair", TmuxSessionID: "$1", LeftPaneID: "%1", RightPaneID: "%2",
		Agents: map[string]PairedAgent{"architect": {PaneID: "%2"}, "junior": {PaneID: "%3"}},
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: store.StateDir}, store, &stubRunner{})
	ctx := context.Background()
	emit := func(event AgentEvent) {
		t.Helper()
		event.OakSessionID = session.ID
		if err := svc.HandleAgentEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	load := func() Session {
		t.Helper()
		got, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	emit(AgentEvent{AgentRole: "architect", Event: "agent_start", TmuxPaneID: "%2", SessionID: "architect-id", SessionFile: "architect.jsonl"})
	emit(AgentEvent{AgentRole: "junior", Event: "question", TmuxPaneID: "%3", SessionID: "junior-id", SessionFile: "junior.jsonl"})
	got := load()
	if got.AgentStatus != AgentStatusQuestion || got.RightPaneID != "%2" || got.AgentSessionFile != "architect.jsonl" {
		t.Fatalf("question state = %#v", got)
	}
	emit(AgentEvent{AgentRole: "junior", Event: "pane_selected", TmuxPaneID: "%3"})
	got = load()
	if got.RightPaneID != "%3" || got.AgentSessionFile != "junior.jsonl" {
		t.Fatalf("selected agent = %#v", got)
	}
	emit(AgentEvent{AgentRole: "junior", Event: "question_answered", TmuxPaneID: "%3"})
	emit(AgentEvent{AgentRole: "junior", Event: "agent_settled", TmuxPaneID: "%3"})
	if got = load(); got.AgentStatus != AgentStatusWorking {
		t.Fatalf("architect still working, status = %q", got.AgentStatus)
	}
	emit(AgentEvent{AgentRole: "architect", Event: "agent_settled", TmuxPaneID: "%2"})
	if got = load(); got.AgentStatus != AgentStatusAttention || got.RightPaneID != "%3" {
		t.Fatalf("settled state = %#v", got)
	}
	if _, err := svc.AcknowledgeAgentAttention(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if got = load(); got.AgentStatus != AgentStatusIdle {
		t.Fatalf("acknowledged status = %q", got.AgentStatus)
	}
}

func TestPairedAgentTodosCombine(t *testing.T) {
	agents := map[string]PairedAgent{
		"architect": {Todo: &TodoSummary{Total: 1, Pending: 1, Tasks: []TodoTask{{Subject: "design", Status: "pending"}}}},
		"junior":    {Todo: &TodoSummary{Total: 1, Completed: 1, Tasks: []TodoTask{{Subject: "build", Status: "completed"}}}},
	}
	got := pairedAgentTodos(agents)
	if got.Total != 2 || got.Pending != 1 || got.Completed != 1 || len(got.Tasks) != 2 || got.Tasks[0].Subject != "design" {
		t.Fatalf("combined todos = %#v", got)
	}
}
