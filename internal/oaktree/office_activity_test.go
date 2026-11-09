package oaktree

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestOfficeActivityDeduplicatesPollsRenamesAndStatusCycles(t *testing.T) {
	store := NewStore(t.TempDir())
	snapshot := officeSnapshot{UpdatedAt: time.Now(), Agents: []officeAgentState{{
		ID: "one", Name: "Original", Role: "architect", SessionState: "working", ActivitySource: "status",
	}}}
	entries, err := updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 1 || entries[0].Text != "Working" {
		t.Fatalf("initial activity: entries=%+v err=%v", entries, err)
	}
	snapshot.Agents[0].Name = "Renamed"
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 1 || entries[0].Author != "Original · architect" {
		t.Fatalf("rename or repeat added activity: entries=%+v err=%v", entries, err)
	}
	snapshot.Agents[0].SessionState = "ready"
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 2 || entries[1].Text != "Ready" {
		t.Fatalf("status change missing: entries=%+v err=%v", entries, err)
	}
	snapshot.Agents[0].SessionState = "working"
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 2 {
		t.Fatalf("status cycle duplicated activity: entries=%+v err=%v", entries, err)
	}
}

func TestOfficeActivityRestartAndClearKeepSeen(t *testing.T) {
	dir := t.TempDir()
	snapshot := officeSnapshot{UpdatedAt: time.Now(), Agents: []officeAgentState{{
		ID: "one", Project: "project", SessionState: "working", ActivitySource: "task", Activity: "Write code",
	}}}
	entries, err := updateOfficeActivityHistory(NewStore(dir), &snapshot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("initial activity: entries=%+v err=%v", entries, err)
	}
	store := NewStore(dir)
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 1 || entries[0].Text != "Write code" {
		t.Fatalf("restart lost or duplicated activity: entries=%+v err=%v", entries, err)
	}
	if err := clearOfficeActivityHistory(store); err != nil {
		t.Fatal(err)
	}
	entries, err = updateOfficeActivityHistory(NewStore(dir), &snapshot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("clear replayed seen activity: entries=%+v err=%v", entries, err)
	}
	snapshot.Agents[0].Activity = "Review code"
	entries, err = updateOfficeActivityHistory(NewStore(dir), &snapshot)
	if err != nil || len(entries) != 1 || entries[0].Text != "Review code" {
		t.Fatalf("new activity after clear: entries=%+v err=%v", entries, err)
	}
}

func TestOfficeActivityDistinctAgentsAndQuestions(t *testing.T) {
	store := NewStore(t.TempDir())
	snapshot := officeSnapshot{UpdatedAt: time.Now(), Agents: []officeAgentState{
		{ID: "one", ActivitySource: "task", Activity: "Same task"},
		{ID: "two", ActivitySource: "task", Activity: "Same task"},
	}}
	entries, err := updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 2 || entries[0].Key == entries[1].Key {
		t.Fatalf("agents conflated: entries=%+v err=%v", entries, err)
	}
	question := officeQuestion{ID: "first", Title: "Fallback", Questions: json.RawMessage(`[{"prompt":"Which one?"}]`)}
	snapshot.Agents = snapshot.Agents[:1]
	snapshot.Agents[0].Questions = []officeQuestion{question}
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 3 || entries[2].Text != "Which one?" || entries[2].QuestionID != "first" {
		t.Fatalf("question missing: entries=%+v err=%v", entries, err)
	}
	snapshot.Agents[0].Questions[0].ID = "second"
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 4 || entries[3].QuestionID != "second" || entries[3].Key == entries[2].Key {
		t.Fatalf("question IDs conflated: entries=%+v err=%v", entries, err)
	}
}

func TestOfficeActivityRetentionBounds(t *testing.T) {
	store := NewStore(t.TempDir())
	snapshot := officeSnapshot{UpdatedAt: time.Now()}
	for i := 0; i < maxOfficeActivitySeen+5; i++ {
		snapshot.Agents = append(snapshot.Agents, officeAgentState{ID: fmt.Sprintf("agent-%04d", i), ActivitySource: "task", Activity: "Same task"})
	}
	entries, err := updateOfficeActivityHistory(store, &snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maxOfficeActivityEntries {
		t.Fatalf("entry count = %d, want %d", len(entries), maxOfficeActivityEntries)
	}
	if entries[0].AgentID != "agent-1805" || entries[len(entries)-1].AgentID != "agent-2004" {
		t.Fatalf("entries not bounded to newest: first=%+v last=%+v", entries[0], entries[len(entries)-1])
	}
	history, err := readOfficeActivityHistory(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Seen) != maxOfficeActivitySeen || history.Seen[0] != officeActivityForAgent(snapshot.Agents[5], snapshot.UpdatedAt).Key {
		t.Fatalf("seen keys not bounded to newest: count=%d", len(history.Seen))
	}
}
