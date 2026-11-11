package oaktree

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOfficeCampfireIsIndependentOfRoleTasks(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ID: "one", Root: "/repo/example", Branch: "feature/names", Agents: map[string]PairedAgent{
		"architect": {Status: AgentStatusWorking, Todo: &TodoSummary{Tasks: []TodoTask{{Subject: "Design the change", Status: "in_progress"}}}},
		"junior":    {Status: AgentStatusWorking, Todo: &TodoSummary{Tasks: []TodoTask{{Subject: "Implement the change", Status: "in_progress"}}}},
	}}
	post := CampfireMessage{At: now.Add(-time.Second), SourceSessionID: session.ID, AgentRole: "junior", Kind: "discovery", Message: "Found the cause."}
	posts := []CampfireMessage{post}
	snapshot := officeSnapshotFrom([]Session{session}, posts, nil, now)
	if snapshot.Agents[0].Name != session.Branch || snapshot.Agents[1].Name != session.Branch {
		t.Fatalf("gym did not receive fallback names: %+v", snapshot.Agents)
	}
	store := NewStore(t.TempDir())
	entries, err := updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 3 {
		t.Fatalf("Campfire masked by tasks: entries=%+v err=%v", entries, err)
	}
	entry := entries[0]
	if entry.AgentID != "one:junior" || entry.Author != "feature/names · junior" || entry.Source != "CAMPFIRE UPDATE" || entry.Text != post.Message || entry.Time != post.At.Format(time.RFC3339Nano) {
		t.Fatalf("Campfire attribution or timestamp lost: %+v", entry)
	}
	posts = append(posts, post)
	snapshot = officeSnapshotFrom([]Session{session}, posts, nil, now)
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 3 {
		t.Fatalf("poll or archive duplication replayed: entries=%+v err=%v", entries, err)
	}
	post.At = now
	posts = append(posts, post)
	snapshot = officeSnapshotFrom([]Session{session}, posts, nil, now)
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 4 || entries[3].Key == entry.Key {
		t.Fatalf("new occurrence of same Campfire text lost: entries=%+v err=%v", entries, err)
	}
	if err := clearOfficeActivityHistory(store); err != nil {
		t.Fatal(err)
	}
	entries, err = updateOfficeActivityHistory(store, &snapshot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("clear replayed Campfire: entries=%+v err=%v", entries, err)
	}
}

func TestCampfirePersistsAgentRole(t *testing.T) {
	store := NewStore(t.TempDir())
	session := Session{ID: "paired", Root: "/repo/example", RightPaneID: "%fake", Agents: map[string]PairedAgent{"architect": {}, "junior": {}}}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: store.StateDir}, store, &stubRunner{})
	if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: session.ID, AgentRole: "junior", Event: "campfire", ActivityKind: "discovery", ActivityMessage: "Found the cause."}); err != nil {
		t.Fatal(err)
	}
	posts, err := store.LoadCampfire()
	if err != nil || len(posts) != 1 || posts[0].AgentRole != "junior" {
		t.Fatalf("Campfire sender lost: posts=%+v err=%v", posts, err)
	}
}

func TestSessionNamingContextRemainsVisible(t *testing.T) {
	session := Session{ID: "1234567890", Root: "/repo/example", Branch: "feature/names"}
	if sessionName(session) != session.Branch || session.Name != "" {
		t.Fatal("fallback was missing or persisted as a real name")
	}
	model := DashboardModel{}
	for _, name := range []string{"", "Useful Session Title"} {
		session.Name = name
		card := strings.Join(model.renderKanbanCard(session, false, 70, "246"), "\n")
		for _, want := range []string{sessionName(session), "example", session.Branch} {
			if !strings.Contains(card, want) {
				t.Errorf("card lost %q: %s", want, card)
			}
		}
	}
	session.Name, session.Branch = "", ""
	if got := sessionName(session); got != "example · 12345678" {
		t.Fatalf("root fallback = %q", got)
	}
}
