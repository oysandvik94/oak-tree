package oaktree

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func coordinationDashboardFixture() DashboardModel {
	model := NewDashboardModel(nil, Config{})
	model.width, model.height = 180, 40
	model.kanbanView, model.campfireHidden = true, true
	model.sessions = []Session{
		{ID: "organizer", Root: "/repo/orchard", Branch: "feature/organizer", AgentStatus: AgentStatusWorking},
		{ID: "worker-a", Root: "/repo/orchard", Branch: "feature/hidden-validation", AgentStatus: AgentStatusWorking},
		{ID: "worker-b", Root: "/repo/orchard", Branch: "feature/hidden-editor", AgentStatus: AgentStatusIdle},
		{ID: "question", Root: "/repo/orchard", Branch: "feature/question", AgentStatus: AgentStatusQuestion, PR: &PRInfo{Found: true, Number: 424}},
		{ID: "unrelated", Root: "/repo/toolbox", Branch: "feature/search", AgentStatus: AgentStatusIdle},
	}
	model.coordinationGroups = []CoordinationGroup{{
		ID: "group", Name: "Authorization sync", OrganizerID: "organizer", UpdatedAt: time.Now().UTC(),
		Assignments: []CoordinationAssignment{
			{SessionID: "worker-a", Task: "Update validation", Status: AssignmentWorking},
			{SessionID: "worker-b", Task: "Update editor", Status: AssignmentReturned, Result: "Editor updated"},
			{SessionID: "question", Task: "Update choices", Status: AssignmentPending},
		},
	}}
	return model
}

func TestCoordinationKanbanFoldsParticipantsButPreservesCountsAndQuestions(t *testing.T) {
	model := coordinationDashboardFixture()
	rendered := ansi.Strip(model.renderKanbanPanel(180, 30))
	for _, want := range []string{"5 active", "2 folded", "Authorization sync", "Organizer", "WORKING 2", "READY 2", "QUESTION 1", "needs you", "#424", "toolbox", "1/3 returned"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in %s", want, rendered)
		}
	}
	for _, hidden := range []string{"hidden-validation", "hidden-editor"} {
		if strings.Contains(rendered, hidden) {
			t.Errorf("folded participant %q remains visible", hidden)
		}
	}
	if model.coordinationSelection(1, 0) != 4 || model.coordinationSelection(-1, 0) != 3 {
		t.Fatal("navigation must skip folded cards but reach the question shortcut")
	}
	model.selected = 1
	if got := model.normalizeCoordinationSelection().currentSession().ID; got != "organizer" {
		t.Fatalf("folded selection = %s", got)
	}
}

func TestCoordinationOrganizerCardPrioritizesWrappedName(t *testing.T) {
	model := coordinationDashboardFixture()
	model.coordinationGroups[0].Name = "Bank administration coordination"
	model.sessions[0].Note = "Keep this note icon"
	model.sessions[0].PR = &PRInfo{Found: true, Number: 421, State: "OPEN"}
	lines := model.renderKanbanItem(0, 26, "214")
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Bank administration", "coordination", "✎", "Organizer", "1/3 returned", "1 question", "#421"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("organizer card missing %q: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, "orchard") || strings.Contains(rendered, "…") {
		t.Fatalf("group heading lost priority or was needlessly truncated: %s", rendered)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 26 {
			t.Fatalf("organizer card exceeds its column width: %q", line)
		}
	}
}

func TestCoordinationOrganizerCardOmitsZeroQuestions(t *testing.T) {
	model := coordinationDashboardFixture()
	model.sessions[3].AgentStatus = AgentStatusIdle
	rendered := ansi.Strip(strings.Join(model.renderKanbanItem(0, 30, "214"), "\n"))
	if strings.Contains(rendered, "?0") || strings.Contains(rendered, "question") || !strings.Contains(rendered, "1/3 returned") {
		t.Fatalf("zero-question noise remains on organizer card: %s", rendered)
	}
}

func TestCoordinationColumnExplainsFoldedSessionsLocally(t *testing.T) {
	model := coordinationDashboardFixture()
	rendered := ansi.Strip(model.renderKanbanColumn("READY", "82", []int{4}, 32, 12, 2, 1))
	for _, want := range []string{"READY 2", "1 folded into groups", "toolbox"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("column missing %q: %s", want, rendered)
		}
	}
	model.sessions[1].Tag = SessionTagTesting
	model.sessions[2].Tag = SessionTagTesting
	rendered = ansi.Strip(model.renderKanbanPanel(180, 30))
	if strings.Contains(rendered, "folded into groups") {
		t.Fatalf("testing-strip participants were counted as folded: %s", rendered)
	}
}

func TestCoordinationColumnScrollAccountsForFoldedHeader(t *testing.T) {
	model := coordinationDashboardFixture()
	model.sessions = []Session{
		{ID: "first", Root: "/repo/first", AgentStatus: AgentStatusIdle},
		{ID: "second", Root: "/repo/second", AgentStatus: AgentStatusIdle},
		{ID: "selected", Root: "/repo/selected", AgentStatus: AgentStatusIdle},
	}
	model.selected = 2
	rendered := ansi.Strip(model.renderKanbanColumn("READY", "82", []int{0, 1, 2}, 32, 9, 4, 1))
	if !strings.Contains(rendered, "selected") || strings.Contains(rendered, "first") {
		t.Fatalf("folded header hid the selected card: %s", rendered)
	}
}

func TestCoordinationExpansionAndClosingRestoreAllParticipants(t *testing.T) {
	model := coordinationDashboardFixture()
	model = model.toggleCoordinationExpansion()
	visible, _ := model.boardProjection()
	if len(visible) != len(model.sessions) {
		t.Fatal("expanded group still hides participants")
	}
	if got := model.coordinationSelection(0, 1); got != 1 {
		t.Fatalf("expanded navigation = %d, want worker-a", got)
	}
	model.selected = 1
	model = model.toggleCoordinationExpansion()
	if model.currentSession().ID != "organizer" {
		t.Fatal("collapsing selected participant did not select its organizer")
	}
	closed := time.Now().UTC().Add(time.Second)
	group := model.coordinationGroups[0]
	group.ClosedAt, group.UpdatedAt = &closed, closed
	model = model.withCoordinationGroups([]CoordinationGroup{group})
	visible, _ = model.boardProjection()
	if len(visible) != len(model.sessions) {
		t.Fatal("closed group still folds sessions")
	}
}

func TestCoordinationParkedQuestionAndTestingRemainReachable(t *testing.T) {
	model := coordinationDashboardFixture()
	model.sessions[3].Tag = SessionTagBlocked
	model.sessions[1].Tag = SessionTagTesting
	visible, indexes := model.boardProjection()
	questionVisible, testingVisible := false, false
	for i, session := range visible {
		if session.ID == "question" {
			questionVisible = sessionDashboardBucket(session) == 0 && indexes[i] == 3
		}
		if session.ID == "worker-a" {
			testingVisible = session.Tag == SessionTagTesting
		}
	}
	if !questionVisible || !testingVisible {
		t.Fatal("folding hid a live question or manually testing participant")
	}
	model.selected = 3
	if model.normalizeCoordinationSelection().selected != 3 {
		t.Fatal("question shortcut cannot retain selection")
	}
	model.expandedGroups = map[string]bool{"group": true}
	expanded, _ := model.boardProjection()
	for _, session := range expanded {
		if session.ID == "question" && sessionDashboardBucket(session) != 0 {
			t.Fatal("expansion changed the parked participant's question column")
		}
	}
	rendered := ansi.Strip(model.renderKanbanPanel(180, 30))
	if !strings.Contains(rendered, "QUESTION 1") || !strings.Contains(rendered, "BLOCKED 0") || !strings.Contains(rendered, "5 active") {
		t.Fatalf("expansion changed live counts: %s", rendered)
	}
}

func TestCoordinationDialogShowsSeparateAgentAndAssignmentStates(t *testing.T) {
	model := coordinationDashboardFixture()
	model.selected = 2
	updated, _ := model.beginCoordination()
	next := updated.(DashboardModel)
	if next.mode != modeCoordination || next.groupDialog.cursor != 2 {
		t.Fatal("group details did not select the current participant")
	}
	rendered := ansi.Strip(next.View().Content)
	for _, want := range []string{"Authorization sync", "READY · returned", "Editor updated", "1/3 returned", "0 accepted", "1 question", "esc board"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dialog missing %q: %s", want, rendered)
		}
	}
}

func TestCoordinationManualCreateAndMutationFailure(t *testing.T) {
	svc := coordinationFixture(t)
	model := NewDashboardModel(svc, Config{})
	model.sessions = []Session{{ID: "organizer"}, {ID: "worker-a"}, {ID: "worker-b"}}
	updated, _ := model.beginCoordination()
	next := updated.(DashboardModel)
	if next.mode != modeCoordinationName {
		t.Fatal("ungrouped session did not open group creation")
	}
	next.groupDialog.input.SetValue("Shared validation")
	updated, _ = next.updateCoordination(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	next = updated.(DashboardModel)
	if next.mode != modeCoordinationParticipants {
		t.Fatal("name did not advance to participant selection")
	}
	next.groupDialog.chosen["worker-a"] = true
	updated, command := next.updateCoordination(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	next = updated.(DashboardModel)
	if command == nil || !next.groupDialog.busy {
		t.Fatal("group creation did not start")
	}
	updated, _ = next.Update(command())
	next = updated.(DashboardModel)
	if next.err != nil || next.mode != modeCoordination || len(next.coordinationGroups) != 1 {
		t.Fatalf("manual create failed: %#v", next)
	}
	// Rejected requests keep the dialog open and leave the saved group intact.
	updated, _ = next.Update(coordinationUpdateMsg{err: errors.New("group is no longer active")})
	next = updated.(DashboardModel)
	if next.err == nil || next.groupDialog.busy || next.mode != modeCoordination || len(next.coordinationGroups) != 1 {
		t.Fatal("mutation failure lost the group dialog")
	}
}

func TestCoordinationRefreshDoesNotReopenClosedGroupOrHideUnrelatedSessions(t *testing.T) {
	model := coordinationDashboardFixture()
	stale := model.coordinationGroups[0]
	now := stale.UpdatedAt.Add(time.Second)
	closed := stale
	closed.ClosedAt, closed.UpdatedAt = &now, now
	model = model.withCoordinationGroups([]CoordinationGroup{closed})
	model = model.withCoordinationGroups([]CoordinationGroup{stale})
	if model.groupForSession("worker-a") != nil {
		t.Fatal("stale refresh resurrected a closed group")
	}
	model = coordinationDashboardFixture()
	model.sessions = model.sessions[1:]
	visible, _ := model.boardProjection()
	if len(visible) != len(model.sessions) {
		t.Fatal("missing organizer hid the remaining participants")
	}
}
