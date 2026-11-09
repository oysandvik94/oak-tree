package oaktree

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func coordinationFixture(t *testing.T) *Service {
	t.Helper()
	state := t.TempDir()
	store := NewStore(state)
	for _, id := range []string{"organizer", "worker-a", "worker-b", "other"} {
		if err := store.SaveSession(Session{ID: id, Root: "/repo/" + id, AgentStatus: AgentStatusIdle, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(Paths{StateDir: state}, store, nil)
}

func createCoordinationFixture(t *testing.T, svc *Service) CoordinationGroup {
	t.Helper()
	group, err := svc.ApplyCoordination(CoordinationRequest{
		Action: "create", ActorID: "organizer", Name: "Shared validation", OrganizerID: "organizer",
		Assignments: []CoordinationAssignment{{SessionID: "worker-a", Task: "Update validation"}, {SessionID: "worker-b", Task: "Update editor"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return group
}

func TestCoordinationExplicitAssignmentLifecycle(t *testing.T) {
	svc := coordinationFixture(t)
	group := createCoordinationFixture(t, svc)
	if group.Assignments[0].Status != AssignmentPending {
		t.Fatal("new assignment must be pending regardless of agent state")
	}
	if _, err := svc.ApplyCoordination(CoordinationRequest{Action: "accept", GroupID: group.ID, ActorID: "organizer", SessionID: "worker-a"}); err == nil {
		t.Fatal("pending assignment was accepted")
	}
	group, err := svc.ApplyCoordination(CoordinationRequest{Action: "report", GroupID: group.ID, ActorID: "worker-a", SessionID: "worker-a", Status: AssignmentReturned, Result: "Validation updated"})
	if err != nil {
		t.Fatal(err)
	}
	if group.Assignments[0].Status != AssignmentReturned || group.Assignments[0].Result != "Validation updated" {
		t.Fatalf("returned assignment = %#v", group.Assignments[0])
	}
	group, err = svc.ApplyCoordination(CoordinationRequest{Action: "accept", GroupID: group.ID, ActorID: "organizer", SessionID: "worker-a"})
	if err != nil || group.Assignments[0].Status != AssignmentAccepted {
		t.Fatalf("accept = %#v, %v", group, err)
	}
	session, err := svc.Store.LoadSession("worker-a")
	if err != nil || session.AgentStatus != AgentStatusIdle {
		t.Fatalf("assignment changed live session state: %#v, %v", session, err)
	}
	group, err = svc.ApplyCoordination(CoordinationRequest{Action: "close", GroupID: group.ID, ActorID: "organizer"})
	if err != nil || group.ClosedAt == nil {
		t.Fatalf("close = %#v, %v", group, err)
	}
	if sessions, err := svc.Store.ListSessions(); err != nil || len(sessions) != 4 {
		t.Fatalf("closing group removed sessions: %d, %v", len(sessions), err)
	}
	contents, err := os.Stat(filepath.Join(svc.Paths.StateDir, "coordination.json"))
	if err != nil || contents.Mode().Perm() != 0o600 {
		t.Fatalf("coordination state must be private: %v, %v", contents, err)
	}
}

func TestCoordinationRejectsConflictsAndUnauthorizedChanges(t *testing.T) {
	svc := coordinationFixture(t)
	group := createCoordinationFixture(t, svc)
	requests := []CoordinationRequest{
		{Action: "close", GroupID: group.ID, ActorID: "worker-a"},
		{Action: "accept", GroupID: group.ID, ActorID: "worker-a", SessionID: "worker-a"},
		{Action: "report", GroupID: group.ID, ActorID: "worker-a", SessionID: "worker-b", Status: AssignmentWorking},
		{Action: "report", GroupID: group.ID, ActorID: "other", SessionID: "other", Status: AssignmentWorking},
		{Action: "report", GroupID: group.ID, SessionID: "worker-a", Status: AssignmentReturned},
		{Action: "assign", GroupID: group.ID, SessionID: "organizer", Task: "Cannot also participate"},
		{Action: "assign", GroupID: group.ID, SessionID: "missing", Task: "Missing session"},
		{Action: "assign", GroupID: group.ID, SessionID: "../outside", Task: "Invalid identity"},
		{Action: "update", GroupID: group.ID, Name: "Control\x1bcharacter"},
		{Action: "update", GroupID: group.ID, Name: strings.Repeat("x", 81)},
		{Action: "update", GroupID: group.ID, OrganizerID: "other"},
		{Action: "create", OrganizerID: "other", Name: "Conflicting group", Assignments: []CoordinationAssignment{{SessionID: "worker-a", Task: "Already grouped"}}},
		{Action: "create", OrganizerID: "other", Name: "Empty group"},
	}
	for _, request := range requests {
		if _, err := svc.ApplyCoordination(request); err == nil {
			t.Errorf("accepted invalid request: %#v", request)
		}
	}
	groups, err := svc.Store.LoadCoordinationGroups()
	if err != nil || len(groups) != 1 || groups[0].ClosedAt != nil || len(groups[0].Assignments) != 2 {
		t.Fatalf("invalid request changed persisted state: %#v, %v", groups, err)
	}
}

func TestCoordinationManualManagementAndReassignment(t *testing.T) {
	svc := coordinationFixture(t)
	group := createCoordinationFixture(t, svc)
	group, err := svc.ApplyCoordination(CoordinationRequest{Action: "update", GroupID: group.ID, Name: "New name", OrganizerID: "worker-a"})
	if err != nil || group.OrganizerID != "worker-a" || !group.Contains("organizer") || group.Name != "New name" {
		t.Fatalf("manual organizer swap = %#v, %v", group, err)
	}
	group, err = svc.ApplyCoordination(CoordinationRequest{Action: "assign", GroupID: group.ID, ActorID: "worker-a", SessionID: "other", Task: "New task"})
	if err != nil || group.assignmentIndex("other") < 0 {
		t.Fatalf("add = %#v, %v", group, err)
	}
	if _, err := svc.ApplyCoordination(CoordinationRequest{Action: "report", GroupID: group.ID, ActorID: "other", SessionID: "other", Status: AssignmentReturned, Result: "First result"}); err != nil {
		t.Fatal(err)
	}
	group, err = svc.ApplyCoordination(CoordinationRequest{Action: "assign", GroupID: group.ID, SessionID: "other", Task: "Revised task"})
	if err != nil {
		t.Fatal(err)
	}
	assignment := group.Assignments[group.assignmentIndex("other")]
	if assignment.Status != AssignmentPending || assignment.Result != "" || assignment.Task != "Revised task" {
		t.Fatalf("reassignment retained stale completion: %#v", assignment)
	}
	group, err = svc.ApplyCoordination(CoordinationRequest{Action: "remove", GroupID: group.ID, SessionID: "other"})
	if err != nil || group.Contains("other") {
		t.Fatalf("remove = %#v, %v", group, err)
	}
}

func TestCoordinationSessionClosureReleasesParticipants(t *testing.T) {
	svc := coordinationFixture(t)
	group := createCoordinationFixture(t, svc)
	if err := svc.Store.DeleteSession("worker-a"); err != nil {
		t.Fatal(err)
	}
	groups, err := svc.Store.LoadCoordinationGroups()
	if err != nil || groups[0].Contains("worker-a") || groups[0].ClosedAt != nil {
		t.Fatalf("participant cleanup = %#v, %v", groups, err)
	}
	if err := svc.Store.DeleteSession("organizer"); err != nil {
		t.Fatal(err)
	}
	groups, err = svc.Store.LoadCoordinationGroups()
	if err != nil || groups[0].ID != group.ID || groups[0].ClosedAt == nil {
		t.Fatalf("organizer cleanup = %#v, %v", groups, err)
	}
	if _, err := svc.Store.LoadSession("worker-b"); err != nil {
		t.Fatalf("organizer closure removed a participant session: %v", err)
	}
}

func TestCoordinationConcurrentReportsDoNotOverwriteEachOther(t *testing.T) {
	svc := coordinationFixture(t)
	group := createCoordinationFixture(t, svc)
	var workers sync.WaitGroup
	for _, id := range []string{"worker-a", "worker-b"} {
		workers.Add(1)
		go func(id string) {
			defer workers.Done()
			otherStore := NewStore(svc.Paths.StateDir)
			otherService := NewService(svc.Paths, otherStore, nil)
			_, err := otherService.ApplyCoordination(CoordinationRequest{Action: "report", GroupID: group.ID, ActorID: id, SessionID: id, Status: AssignmentReturned, Result: id + " finished"})
			if err != nil {
				t.Error(err)
			}
		}(id)
	}
	workers.Wait()
	groups, err := svc.Store.LoadCoordinationGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, assignment := range groups[0].Assignments {
		if assignment.Status != AssignmentReturned || assignment.Result != assignment.SessionID+" finished" {
			t.Fatalf("lost concurrent report: %#v", assignment)
		}
	}
}

func TestCoordinationExtensionRegistersManagedTool(t *testing.T) {
	for _, fragment := range []string{"registerCoordinationTool();", `name: "oak_coordination"`, `"--oak-session", oak`, `"--tmux-pane", tmuxPane`, "READY or an idle agent is not assignment completion", "truncateHead(result.stdout", "use intercom"} {
		if !strings.Contains(PiExtensionSource, fragment) {
			t.Errorf("extension missing %q", fragment)
		}
	}
	if strings.Index(PiExtensionSource, "registerCoordinationTool();") < strings.Index(PiExtensionSource, `if (!await hook("session_start"`) {
		t.Fatal("coordination must register only after managed-session registration")
	}
}
