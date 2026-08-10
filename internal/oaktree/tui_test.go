package oaktree

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestCompactPathUsesHomePrefixAndEllipsis(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, "dev", "general", "oak-tree", "sessions", "feature", "very-long-name")
	got := compactPath(path, 24)

	if !strings.HasPrefix(got, "~") {
		t.Fatalf("compactPath() = %q, want home-relative path", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("compactPath() = %q, want ellipsis for truncation", got)
	}
	if strings.Contains(got, home) {
		t.Fatalf("compactPath() = %q, want home path hidden", got)
	}
}

func TestCreateFormRenderRootCandidatesShowsFilteredMatches(t *testing.T) {
	candidates := []rootCandidate{
		{Path: "/home/me/dev/general/oak-tree", SearchDir: "/home/me/dev/general"},
		{Path: "/home/me/work/example-repo", SearchDir: "/home/me/work"},
	}
	form := newCreateForm(candidates, 80, 24)
	form.rootSearch.SetValue("example")
	form.applyFilter()

	rendered := form.renderRootCandidates()
	if !strings.Contains(rendered, "example-repo") {
		t.Fatalf("renderRootCandidates() = %q, want filtered candidate", rendered)
	}
	if strings.Contains(rendered, "oak-tree") {
		t.Fatalf("renderRootCandidates() = %q, did not expect unmatched candidate", rendered)
	}
}

func TestCreateRootSearchAcceptsJAndKAsFilterText(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.rootCandidates = []rootCandidate{
		{Path: "/repos/jupiter", SearchDir: "/repos"},
		{Path: "/repos/kotlin", SearchDir: "/repos"},
	}
	model, _ = model.beginCreate()

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	next := updated.(DashboardModel)
	if got := next.form.rootSearch.Value(); got != "k" {
		t.Fatalf("root search after k = %q, want %q", got, "k")
	}
	if got := len(next.form.filtered); got != 1 {
		t.Fatalf("filtered roots after k = %d, want 1", got)
	}

	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	next = updated.(DashboardModel)
	if got := next.form.rootSearch.Value(); got != "kj" {
		t.Fatalf("root search after kj = %q, want %q", got, "kj")
	}
	if got := len(next.form.filtered); got != 0 {
		t.Fatalf("filtered roots after kj = %d, want 0", got)
	}
}

func TestCreateFormBranchModeToggleRendersOpenExisting(t *testing.T) {
	form := newCreateForm(nil, 80, 24)
	form.selectedRootPath = "/repo/oak-tree"
	form.stage = createStageBranch

	rendered := form.render()
	if !strings.Contains(rendered, "CREATE NEW") {
		t.Fatalf("render() = %q, want default create mode", rendered)
	}
	form.toggleBranchMode()
	rendered = form.render()
	if form.branchMode != BranchModeOpenExisting {
		t.Fatalf("branchMode = %q, want %q", form.branchMode, BranchModeOpenExisting)
	}
	if !strings.Contains(rendered, "OPEN EXISTING") {
		t.Fatalf("render() = %q, want open existing mode", rendered)
	}
}

func TestViewPinsFooterToBottom(t *testing.T) {
	for _, width := range []int{80, 160} {
		model := NewDashboardModel(&Service{}, Config{})
		model.width = width
		model.height = 50
		model.sessions = []Session{{PR: &PRInfo{Found: true, Number: 42, Title: "Selected PR", ChecksState: "pass"}}}

		content := model.View().Content
		if got := lipgloss.Height(content); got != model.height {
			t.Fatalf("View() height at width %d = %d, want %d", width, got, model.height)
		}
		lines := strings.Split(content, "\n")
		if !strings.Contains(lines[len(lines)-1], "KEYS") {
			t.Fatalf("View() last line at width %d = %q, want footer", width, lines[len(lines)-1])
		}
	}
}

func TestRenderDashboardUsesDenseSessionTable(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.width, model.height = 140, 24
	model.sessions = []Session{
		{ID: "question", Root: "/repo/identity", Branch: "feature/question", AgentStatus: AgentStatusQuestion, GitStatus: &GitStatus{Clean: true}},
		{ID: "working", Root: "/repo/api", Branch: "feature/work", AgentStatus: AgentStatusWorking, GitStatus: &GitStatus{Clean: false}},
		{ID: "review", Root: "/repo/docs", Branch: "feature/docs", Tag: SessionTagWaitingReview, AgentStatus: AgentStatusIdle},
		{ID: "testing", Root: "/repo/web", Branch: "feature/web", Tag: SessionTagTesting, AgentStatus: AgentStatusIdle},
	}
	rendered := model.View().Content
	for _, want := range []string{"2 active", "needs-you", "working", "ready", "review", "testing", "REVIEW · 1 parked", "TESTING · 1 parked", "STATE", "SESSION", "BRANCH", "GIT", "PR", "COST", "identity", "feature/question", "▌", " QUESTION", "⠋ WORKING", " REVIEW", " TESTING", " clean"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dashboard rendering missing %q: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "Preview") {
		t.Fatalf("dashboard still renders preview: %q", rendered)
	}
}

func TestVToggleRendersKanbanBoard(t *testing.T) {
	stateDir := t.TempDir()
	service := NewService(Paths{StateDir: stateDir}, NewStore(stateDir), &stubRunner{})
	model := NewDashboardModel(service, Config{})
	model.width, model.height = 180, 26
	parkedAt := time.Now().Add(-49 * time.Hour)
	model.sessions = []Session{
		{ID: "question", Root: "/repo/identity", Branch: "feature/question", AgentStatus: AgentStatusQuestion, GitStatus: &GitStatus{Clean: true}},
		{ID: "working", Root: "/repo/api", Branch: "feature/work", AgentStatus: AgentStatusWorking, SubagentCount: 3},
		{ID: "ready", Root: "/repo/oak-tree", Branch: "main", AgentStatus: AgentStatusIdle},
		{ID: "review", Root: "/repo/docs", Branch: "feature/docs", Tag: SessionTagWaitingReview, TagUpdatedAt: &parkedAt, Note: "Jarek review"},
		{ID: "testing", Root: "/repo/web", Branch: "feature/web", Tag: SessionTagTesting},
	}

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'v', Text: "v"}))
	kanban := updated.(DashboardModel)
	if !kanban.kanbanView {
		t.Fatal("v did not enable kanban view")
	}
	rendered := kanban.View().Content
	for _, want := range []string{"KANBAN", "QUESTION 1", "WORKING 1", "READY 1", "REVIEW 1", "TESTING 1", "identity", "feature/question", "✎", "age 2d", " 3", "v", "view"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("kanban rendering missing %q: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "Jarek review") {
		t.Fatalf("kanban card renders note text instead of icon: %q", rendered)
	}
	if got := lipgloss.Height(rendered); got != model.height {
		t.Fatalf("kanban height = %d, want %d", got, model.height)
	}
	if restored := NewDashboardModel(service, Config{}); !restored.kanbanView {
		t.Fatal("new dashboard did not restore persisted kanban view")
	}

	updated, _ = kanban.Update(tea.KeyPressMsg(tea.Key{Code: 'v', Text: "v"}))
	if updated.(DashboardModel).kanbanView {
		t.Fatal("second v did not restore table view")
	}
}

func TestKanbanNavigationMovesWithinAndAcrossColumns(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.kanbanView = true
	model.sessions = []Session{
		{ID: "question", AgentStatus: AgentStatusQuestion},
		{ID: "working-1", AgentStatus: AgentStatusWorking},
		{ID: "working-2", AgentStatus: AgentStatusWorking},
		{ID: "testing-1", Tag: SessionTagTesting},
		{ID: "testing-2", Tag: SessionTagTesting},
	}
	model.selected = 2

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	next := updated.(DashboardModel)
	if got := next.currentSession().ID; got != "testing-2" {
		t.Fatalf("right selected %q, want same row in next non-empty column", got)
	}
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	next = updated.(DashboardModel)
	if got := next.currentSession().ID; got != "working-2" {
		t.Fatalf("left selected %q, want same row in previous non-empty column", got)
	}
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	next = updated.(DashboardModel)
	if got := next.currentSession().ID; got != "working-1" {
		t.Fatalf("up selected %q, want previous card in the same column", got)
	}
}

func TestSelectedTableRowUsesAmberBackgroundAcrossSegments(t *testing.T) {
	foreground, background := tableRowColors(true)
	if foreground != "229" || background != "#36362f" {
		t.Fatalf("selected table colors = %q, %q; want pale yellow on amber", foreground, background)
	}
	plain := renderTableCell(tablePlainCell("project", foreground), 12, background, true)
	chip := renderTableCell(tableChipCell(" clean", "82", "#244232"), 12, background, true)
	separator := renderTableSegment("  ", "", background, true)
	if lipgloss.Width(plain) != 12 || lipgloss.Width(chip) != 12 || lipgloss.Width(separator) != 2 {
		t.Fatalf("selected row segments lost width: plain=%d chip=%d separator=%d", lipgloss.Width(plain), lipgloss.Width(chip), lipgloss.Width(separator))
	}
}

func TestSessionTableSemanticChipsUseExpectedIconsAndColors(t *testing.T) {
	cases := []struct {
		session Session
		icon    string
		color   string
	}{
		{Session{AgentStatus: AgentStatusQuestion}, " QUESTION", "81"},
		{Session{AgentStatus: AgentStatusWorking}, "⠋ WORKING", "214"},
		{Session{AgentStatus: AgentStatusAttention}, " READY", "82"},
		{Session{AgentStatus: AgentStatusIdle}, " READY", "82"},
		{Session{AgentStatus: AgentStatusWorking, Tag: SessionTagWaitingReview}, " REVIEW", "170"},
		{Session{AgentStatus: AgentStatusWorking, Tag: SessionTagTesting}, " TESTING", "214"},
	}
	for _, tc := range cases {
		label, color, background := sessionTableStateChip(tc.session)
		if label != tc.icon || color != tc.color || background == "" {
			t.Fatalf("sessionTableStateChip(%#v) = %q, %q, %q", tc.session, label, color, background)
		}
	}
	if got := gitStatusChip(&GitStatus{Clean: false, Ahead: 1}); got != " changes ↑" {
		t.Fatalf("gitStatusChip() = %q, want changes chip", got)
	}
	if got := prStateColor("OPEN"); got != "81" {
		t.Fatalf("prStateColor(OPEN) = %q, want cyan", got)
	}
}

func TestTodoSummaryChipShowsTaskProgress(t *testing.T) {
	for _, tc := range []struct {
		todo  *TodoSummary
		label string
		color string
	}{
		{nil, "—", "244"},
		{&TodoSummary{}, "—", "244"},
		{&TodoSummary{Total: 3, Pending: 2, InProgress: 1}, "◐ 0/3", "214"},
		{&TodoSummary{Total: 3, Pending: 2, Completed: 1}, "○ 1/3", "246"},
		{&TodoSummary{Total: 3, Completed: 3}, "✓ 3/3", "82"},
	} {
		label, color, _ := todoSummaryChip(tc.todo)
		if label != tc.label || color != tc.color {
			t.Fatalf("todoSummaryChip(%#v) = %q, %q; want %q, %q", tc.todo, label, color, tc.label, tc.color)
		}
	}
}

func TestSpaceExpandsSelectedSessionTodoTasksInline(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.width, model.height = 140, 24
	model.sessions = []Session{{
		ID: "todo", Root: "/repo/todo", AgentStatus: AgentStatusWorking,
		Todo: &TodoSummary{Total: 3, Pending: 1, InProgress: 1, Completed: 1, Tasks: []TodoTask{
			{Subject: "Pending task", Status: "pending"},
			{Subject: "Active task", Status: "in_progress"},
			{Subject: "Completed task", Status: "completed"},
		}},
	}}

	if rendered := model.View().Content; strings.Contains(rendered, "Pending task") || !strings.Contains(rendered, "space") {
		t.Fatalf("collapsed dashboard = %q, want todo key but no task details", rendered)
	}
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	next := updated.(DashboardModel)
	if !next.todoExpanded {
		t.Fatal("space did not expand todo details")
	}
	for _, want := range []string{"○ Pending task", "◐ Active task", "✓ Completed task"} {
		if rendered := next.View().Content; !strings.Contains(rendered, want) {
			t.Fatalf("expanded dashboard missing %q: %q", want, rendered)
		}
	}
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	if updated.(DashboardModel).todoExpanded {
		t.Fatal("second space did not collapse todo details")
	}
}

func TestExpandedTodoTasksStayWithinDashboardHeight(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.width, model.height, model.todoExpanded = 100, 16, true
	tasks := make([]TodoTask, 20)
	for i := range tasks {
		tasks[i] = TodoTask{Subject: fmt.Sprintf("Task %d", i+1), Status: "pending"}
	}
	model.sessions = []Session{{ID: "todo", Root: "/repo/todo", Todo: &TodoSummary{Total: len(tasks), Pending: len(tasks), Tasks: tasks}}}

	rendered := model.View().Content
	if got := lipgloss.Height(rendered); got != model.height {
		t.Fatalf("expanded dashboard height = %d, want %d", got, model.height)
	}
	if !strings.Contains(rendered, "more") {
		t.Fatalf("expanded dashboard did not summarize clipped tasks: %q", rendered)
	}
}

func TestWorkingStateChipAnimatesAtStableWidth(t *testing.T) {
	first := workingStateChip(0)
	second := workingStateChip(3)
	if first == second {
		t.Fatalf("working spinner did not advance: %q", first)
	}
	if lipgloss.Width(first) != lipgloss.Width(second) || !strings.HasSuffix(first, " WORKING") || !strings.HasSuffix(second, " WORKING") {
		t.Fatalf("working spinner changed width or label: %q, %q", first, second)
	}
}

func TestDashboardOrdersActiveBeforeParkedSessions(t *testing.T) {
	ordered := orderSessionsForDashboard([]Session{
		{ID: "testing", AgentStatus: AgentStatusQuestion, Tag: SessionTagTesting},
		{ID: "review", AgentStatus: AgentStatusQuestion, Tag: SessionTagWaitingReview},
		{ID: "ready", AgentStatus: AgentStatusIdle},
		{ID: "working", AgentStatus: AgentStatusWorking},
	})
	if got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID}; got[0] != "working" || got[1] != "ready" || got[2] != "review" || got[3] != "testing" {
		t.Fatalf("session order = %#v, want active sessions before review and testing", got)
	}
}

func TestRenderSessionsPanelUsesAllContentRows(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.selected = 6
	for i := 0; i < 7; i++ {
		model.sessions = append(model.sessions, Session{ID: fmt.Sprintf("session-%d", i), Root: fmt.Sprintf("/repo/project-%d", i)})
	}

	panel := model.renderSessionsPanel(100, 10)
	if got := strings.Count(panel, "▌"); got != 7 {
		t.Fatalf("rendered rows = %d, want 7", got)
	}
	if !strings.Contains(panel, "project-6") {
		t.Fatalf("selected row is not visible: %q", panel)
	}
}

func TestDenseTableKeepsSelectedRowVisibleAndFitsCompactWidths(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.width, model.height = 80, 18
	for i := 0; i < 20; i++ {
		model.sessions = append(model.sessions, Session{ID: fmt.Sprintf("session-%02d", i), Root: fmt.Sprintf("/repo/project-%02d", i), Branch: "feature/very-long-branch-name"})
	}
	model.selected = len(model.sessions) - 1
	content := model.View().Content
	if !strings.Contains(content, "project-19") {
		t.Fatalf("selected session not visible: %q", content)
	}
	if strings.Contains(content, "project-00") {
		t.Fatalf("table did not bound rows around selected session")
	}
	if got := lipgloss.Height(content); got != model.height {
		t.Fatalf("View() height = %d, want %d", got, model.height)
	}
	lines := strings.Split(content, "\n")
	if !strings.Contains(lines[len(lines)-1], "KEYS") {
		t.Fatalf("footer is not pinned: %q", lines[len(lines)-1])
	}
	for _, width := range []int{70, 80, 95, 120} {
		model.width = width
		for _, line := range strings.Split(model.View().Content, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("line width %d exceeds terminal width %d: %q", lipgloss.Width(line), width, line)
			}
		}
	}
}

func TestDenseTableCountsUsePrioritySemanticsAndShowsWideData(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.width, model.height = 140, 30
	model.usage = UsageCache{Sessions: []UsageSession{{SessionID: "cost", TotalCostUSD: 1.25}}}
	model.sessions = []Session{
		{ID: "question", Root: "/repo/question", AgentStatus: AgentStatusQuestion, Tag: SessionTagWaitingReview},
		{ID: "attention", Root: "/repo/attention", AgentStatus: AgentStatusAttention, Tag: SessionTagWaitingReview},
		{ID: "working", Root: "/repo/working", AgentStatus: AgentStatusWorking},
		{ID: "working-review", Root: "/repo/working-review", Branch: "feature/review", AgentStatus: AgentStatusWorking, Tag: SessionTagWaitingReview},
		{ID: "review", Root: "/repo/review", Tag: SessionTagWaitingReview},
		{ID: "testing", Root: "/repo/testing", Tag: SessionTagTesting},
		{ID: "idle", Root: "/repo/idle", AgentSessionIDs: []string{"cost"}, Todo: &TodoSummary{Total: 3, Pending: 1, InProgress: 1, Completed: 1}, GitStatus: &GitStatus{Clean: false, Ahead: 1}, PR: &PRInfo{Found: true, Number: 42, State: "OPEN"}},
	}
	rendered := model.View().Content
	for _, want := range []string{"2 active", "needs-you 0", "working 1", "ready 1", "review 4", "testing 1", "REVIEW · 4 parked", "TESTING · 1 parked", "STATE", "SESSION", "BRANCH", "GIT", "PR", "TODO", "COST", "changes ↑", "#42 open", "◐ 1/3", "$1.25"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dense table missing %q: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "usage $1.25") {
		t.Fatalf("cost column includes nested usage label: %q", rendered)
	}
	if !strings.Contains(rendered, " REVIEW") {
		t.Fatalf("waiting-review chip missing: %q", rendered)
	}

}

func TestDenseTableSeparatesProjectAndBranch(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	session := Session{Root: "/repo/project", Branch: "feature/example"}
	row := model.renderSessionTableRow(session, false, 140)
	if strings.Count(row, "project") != 1 || strings.Count(row, "feature/example") != 1 {
		t.Fatalf("row = %q, want project and branch rendered once", row)
	}
	if strings.Contains(row, "project · feature/example") {
		t.Fatalf("row = %q, want branch separated from session column", row)
	}
}

func TestDashboardNavigationStartsAnimation(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{ID: "one"}, {ID: "two"}}
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if cmd == nil {
		t.Fatal("navigation returned nil animation command")
	}
	if updated.(DashboardModel).selected != 1 {
		t.Fatalf("selected = %d, want 1", updated.(DashboardModel).selected)
	}
}

func TestDashboardRefreshCompletesWhenNoPRRefreshIsQueued(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.status = "refreshing"
	updated, _ := model.Update(dashboardMsg{sessions: []Session{{ID: "session"}}})
	if got := updated.(DashboardModel).status; got != "ready" {
		t.Fatalf("status = %q, want ready", got)
	}
}

func TestDashboardRefreshEmptySessionsCompletes(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.status = "refreshing"
	updated, _ := model.Update(dashboardMsg{sessions: []Session{}})
	if got := updated.(DashboardModel).status; got != "ready" {
		t.Fatalf("status = %q, want ready", got)
	}
}

func TestRenderStatusLineOnlyShowsErrors(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{PR: &PRInfo{Found: true, Number: 42}}}
	if rendered := model.renderStatusLine(120); rendered != "" {
		t.Fatalf("renderStatusLine() = %q, want no duplicate selected-session summary", rendered)
	}

	model.err = errors.New("refresh failed")
	if rendered := model.renderStatusLine(120); !strings.Contains(rendered, "refresh failed") {
		t.Fatalf("renderStatusLine() = %q, want error", rendered)
	}
}

func TestRenderPRInspectorShowsSelectedPRDetailsAndCommands(t *testing.T) {
	unresolved := 2
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{PR: &PRInfo{
		Found:              true,
		Number:             1180,
		Title:              "Finalize RP timestamps",
		State:              "OPEN",
		ChecksState:        "pass",
		ReviewDecision:     "approved",
		UnresolvedComments: &unresolved,
		RefreshedAt:        time.Now().Add(-3 * time.Minute),
	}}}

	rendered := model.renderPRInspector(140)
	for _, want := range []string{"PR #1180", "Finalize RP timestamps", "refreshed 3m ago", "READY", "✓ CI PASS", "✓ APPROVED", "● 2 UNRESOLVED", "open", "refresh"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("PR inspector missing %q: %q", want, rendered)
		}
	}
	for _, width := range []int{40, 80, 140} {
		for _, line := range strings.Split(model.renderPRInspector(width), "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("PR inspector line width %d exceeds %d: %q", lipgloss.Width(line), width, line)
			}
		}
	}
}

func TestRenderPRInspectorOnlyShowsActualPRInDashboard(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	if rendered := model.renderPRInspector(120); rendered != "" {
		t.Fatalf("renderPRInspector() = %q without a selected PR", rendered)
	}
	model.sessions = []Session{{PR: &PRInfo{Found: false}}}
	if rendered := model.renderPRInspector(120); rendered != "" {
		t.Fatalf("renderPRInspector() = %q for missing PR", rendered)
	}
	model.sessions[0].PR.Found = true
	model.mode = modeCreate
	if rendered := model.renderPRInspector(120); rendered != "" {
		t.Fatalf("renderPRInspector() = %q outside dashboard mode", rendered)
	}
}

func TestRenderPRInspectorShowsBlockingStates(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{PR: &PRInfo{Found: true, Number: 42, IsDraft: true, ChecksState: "fail", ReviewDecision: "changes_requested"}}}

	rendered := model.renderPRInspector(140)
	for _, want := range []string{"DRAFT", "✕ CI FAIL", "✕ CHANGES REQUESTED", "COMMENTS UNAVAILABLE"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("PR inspector missing %q: %q", want, rendered)
		}
	}
}

func TestFormatRelativeAge(t *testing.T) {
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Time{}, ""},
		{now.Add(-30 * time.Second), "now"},
		{now.Add(-3 * time.Minute), "3m ago"},
		{now.Add(-2 * time.Hour), "2h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
	}
	for _, tc := range cases {
		if got := formatRelativeAge(tc.at, now); got != tc.want {
			t.Fatalf("formatRelativeAge(%s) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestSessionParkedAge(t *testing.T) {
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	taggedAt := now.Add(-49 * time.Hour)
	if got := sessionParkedAge(Session{Tag: SessionTagWaitingReview, TagUpdatedAt: &taggedAt}, now); got != "2d" {
		t.Fatalf("sessionParkedAge() = %q, want %q", got, "2d")
	}
	if got := sessionParkedAge(Session{Tag: SessionTagTesting, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-time.Minute)}, now); got != "3h" {
		t.Fatalf("legacy sessionParkedAge() = %q, want %q", got, "3h")
	}
	if got := sessionParkedAge(Session{CreatedAt: now.Add(-3 * time.Hour)}, now); got != "" {
		t.Fatalf("sessionParkedAge() for active session = %q, want empty", got)
	}
}

func TestFooterBindingsOmitContextualPRKeys(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})

	for _, binding := range model.footerBindings() {
		help := binding.Help()
		switch help.Key {
		case "o", "p":
			t.Fatalf("footerBindings() includes contextual PR key %q", help.Key)
		}
	}
}

func TestAnimationTickAdvancesDeckRailAndKeepsRailAlive(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.motionTickerActive = true

	updated, cmd := model.Update(animationTickMsg{})
	next := updated.(DashboardModel)
	if next.motionFrame != 1 {
		t.Fatalf("motionFrame = %d, want 1", next.motionFrame)
	}
	if !next.motionTickerActive || cmd == nil {
		t.Fatalf("animation tick did not stay active for rail: active=%v cmd=%v", next.motionTickerActive, cmd)
	}

	next.motionTickerActive = true
	updated, cmd = next.Update(animationTickMsg{})
	next = updated.(DashboardModel)
	if !next.motionTickerActive || cmd == nil {
		t.Fatalf("animation tick stopped ambient rail after spring settled: active=%v cmd=%v", next.motionTickerActive, cmd)
	}
}

func TestSelectionMovementStartsAnimationTick(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{
		{ID: "session-1", Root: "/repo/one", Workdir: "/repo/one"},
		{ID: "session-2", Root: "/repo/two", Workdir: "/repo/two"},
	}

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	next := updated.(DashboardModel)
	if next.selected != 1 {
		t.Fatalf("selected = %d, want 1", next.selected)
	}
	if !next.motionTickerActive || cmd == nil {
		t.Fatalf("selection movement did not start animation tick: active=%v cmd=%v", next.motionTickerActive, cmd)
	}
}

func TestDashboardLoadStartsAmbientAnimationWithNoSessions(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})

	updated, cmd := model.Update(dashboardMsg{sessions: nil})
	next := updated.(DashboardModel)
	if !next.motionTickerActive || cmd == nil {
		t.Fatalf("empty dashboard did not start ambient animation: active=%v cmd=%v", next.motionTickerActive, cmd)
	}
}

func TestPRRefreshResultWinsOverReconciledSessionState(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{ID: "session", Branch: "current", GitStatus: &GitStatus{Branch: "current"}}}
	updated, _ := model.Update(prRefreshMsg{sessionID: "session", session: Session{ID: "session", Branch: "current", PR: &PRInfo{Found: true, Number: 42}}})
	session := updated.(DashboardModel).sessions[0]
	if session.PR == nil || session.PR.Number != 42 {
		t.Fatalf("PR = %#v, want newly refreshed PR", session.PR)
	}
}

func TestStaleAgentSnapshotPreservesPRAndBranchThroughUpdate(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{ID: "session", Branch: "current", PR: &PRInfo{Found: true, Number: 42}, GitStatus: &GitStatus{Branch: "current"}}}
	updated, _ := model.Update(agentStatusRefreshMsg{sessions: []Session{{ID: "session", Branch: "stale", AgentStatus: AgentStatusWorking}}})
	session := updated.(DashboardModel).sessions[0]
	if session.Branch != "current" || session.PR == nil || session.PR.Number != 42 || session.AgentStatus != AgentStatusWorking {
		t.Fatalf("session = %#v, want fresh status with current branch and PR", session)
	}
}

func TestStalePRRefreshResultCannotOverwriteObservedBranch(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.status = "refreshing PRs"
	model.prAutoRefreshing = 1
	model.prRefreshInFlight = map[string]struct{}{"session": {}}
	model.sessions = []Session{{ID: "session", Branch: "new", GitStatus: &GitStatus{Branch: "new"}}}
	updated, _ := model.Update(prRefreshMsg{auto: true, sessionID: "session", session: Session{ID: "session", Branch: "old", PR: &PRInfo{Found: true, Number: 1}}})
	next := updated.(DashboardModel)
	if next.sessions[0].Branch != "new" || next.sessions[0].PR != nil {
		t.Fatalf("session = %#v, want observed branch and cleared PR", next.sessions[0])
	}
	if next.prAutoRefreshing != 0 || next.prRefreshIsInFlight("session") || next.status == "refreshing PRs" {
		t.Fatalf("refresh state = count %d, in-flight %v, status %q", next.prAutoRefreshing, next.prRefreshIsInFlight("session"), next.status)
	}
}

func TestGitStatusRefreshErrorIsSurfaced(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	updated, _ := model.Update(gitStatusRefreshMsg{err: errors.New("git unavailable")})
	next := updated.(DashboardModel)
	if next.err == nil || next.status != "git refresh failed" {
		t.Fatalf("error/status = %v/%q, want surfaced git refresh failure", next.err, next.status)
	}
}

func TestPeriodicGitStatusRefreshReplacesBranchAndPR(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{{ID: "session", Branch: "old", PR: &PRInfo{Found: true, Number: 1}, GitStatus: &GitStatus{Branch: "old"}}}
	updated, _ := model.Update(gitStatusRefreshMsg{sessions: []Session{{ID: "session", Branch: "new", GitStatus: &GitStatus{Branch: "new"}}}})
	session := updated.(DashboardModel).sessions[0]
	if session.Branch != "new" || session.PR != nil || session.GitStatus == nil || session.GitStatus.Branch != "new" {
		t.Fatalf("session = %#v, want new branch and cleared PR", session)
	}
}

func TestAgentStatusRefreshPreservesVisibleSessionOrder(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{
		{ID: "oldest", AgentStatus: AgentStatusWorking},
		{ID: "newest", AgentStatus: AgentStatusIdle},
	}
	model.selected = 1

	updated, _ := model.Update(agentStatusRefreshMsg{sessions: []Session{
		{ID: "newest", AgentStatus: AgentStatusWorking},
		{ID: "oldest", AgentStatus: AgentStatusIdle},
	}})
	next := updated.(DashboardModel)

	if got := []string{next.sessions[0].ID, next.sessions[1].ID}; got[0] != "newest" || got[1] != "oldest" {
		t.Fatalf("session order = %#v, want working before idle", got)
	}
	if next.selected != 0 || next.sessions[next.selected].ID != "newest" {
		t.Fatalf("selected = %d (%s), want newest preserved", next.selected, next.sessions[next.selected].ID)
	}
	if next.sessions[0].AgentStatus != AgentStatusWorking || next.sessions[1].AgentStatus != AgentStatusIdle {
		t.Fatalf("sessions were not updated in place: %#v", next.sessions)
	}
}

func TestAgentStatusRefreshPreservesGitStatus(t *testing.T) {
	model := NewDashboardModel(&Service{}, Config{})
	model.sessions = []Session{
		{ID: "session-1", AgentStatus: AgentStatusWorking, GitStatus: &GitStatus{Clean: false, Changed: 1}},
	}

	updated, _ := model.Update(agentStatusRefreshMsg{sessions: []Session{
		{ID: "session-1", AgentStatus: AgentStatusIdle},
	}})
	next := updated.(DashboardModel)

	if next.sessions[0].GitStatus == nil || next.sessions[0].GitStatus.Changed != 1 {
		t.Fatalf("GitStatus = %#v, want preserved dirty status", next.sessions[0].GitStatus)
	}
	if next.sessions[0].AgentStatus != AgentStatusIdle {
		t.Fatalf("AgentStatus = %q, want idle", next.sessions[0].AgentStatus)
	}
}

func TestSessionTagPickerPersistsTestingAndMovesSessionDown(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	sessions := []Session{
		{ID: "selected", Root: "/repo/selected", Workdir: "/repo/selected"},
		{ID: "active", Root: "/repo/active", Workdir: "/repo/active"},
	}
	for _, session := range sessions {
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
	}
	model := NewDashboardModel(NewService(Paths{StateDir: stateDir}, store, &stubRunner{}), Config{})
	model.sessions = sessions

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	next := updated.(DashboardModel)
	if next.mode != modeTagPicker {
		t.Fatalf("mode = %v, want tag picker", next.mode)
	}
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	next = updated.(DashboardModel)
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	next = updated.(DashboardModel)
	updated, cmd := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	next = updated.(DashboardModel)
	if cmd == nil {
		t.Fatal("tag picker enter returned nil command")
	}
	msg, ok := cmd().(tagUpdateMsg)
	if !ok {
		t.Fatalf("tag update command message = %T, want tagUpdateMsg", cmd())
	}
	updated, _ = next.Update(msg)
	next = updated.(DashboardModel)

	if next.mode != modeDashboard {
		t.Fatalf("mode = %v, want dashboard", next.mode)
	}
	if next.sessions[0].ID != "active" || next.sessions[1].ID != "selected" {
		t.Fatalf("session order = %#v, want active before testing", []string{next.sessions[0].ID, next.sessions[1].ID})
	}
	if next.selected != 1 || next.sessions[next.selected].Tag != SessionTagTesting {
		t.Fatalf("selected/tag = %d/%q, want selected testing", next.selected, next.sessions[next.selected].Tag)
	}
	loaded, err := store.LoadSession("selected")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tag != SessionTagTesting {
		t.Fatalf("persisted tag = %q, want %q", loaded.Tag, SessionTagTesting)
	}
	if loaded.TagUpdatedAt == nil {
		t.Fatal("persisted tag timestamp is nil")
	}
}

func TestSessionNoteEditorPersistsNote(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{ID: "noted", Root: "/repo/noted", Workdir: "/repo/noted", Tag: SessionTagWaitingReview}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	model := NewDashboardModel(NewService(Paths{StateDir: stateDir}, store, &stubRunner{}), Config{})
	model.sessions = []Session{session}

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	next := updated.(DashboardModel)
	if next.mode != modeNoteEditor {
		t.Fatalf("mode = %v, want note editor", next.mode)
	}
	next.noteInput.SetValue("  Waiting for review feedback  ")
	updated, cmd := next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	next = updated.(DashboardModel)
	if cmd == nil {
		t.Fatal("note editor enter returned nil command")
	}
	msg, ok := cmd().(noteUpdateMsg)
	if !ok {
		t.Fatalf("note update command message = %T, want noteUpdateMsg", cmd())
	}
	updated, _ = next.Update(msg)
	next = updated.(DashboardModel)
	if next.mode != modeDashboard || next.sessions[0].Note != "Waiting for review feedback" {
		t.Fatalf("mode/note = %v/%q", next.mode, next.sessions[0].Note)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Note != "Waiting for review feedback" {
		t.Fatalf("persisted note = %q", loaded.Note)
	}
}

func TestNeedsAutoPRRefreshMigratesLegacyCommentCacheOnce(t *testing.T) {
	now := testTime()
	session := Session{
		Branch: "feature/pr",
		PR:     &PRInfo{Found: true, RefreshedAt: now},
	}
	if !needsAutoPRRefresh(session, now) {
		t.Fatal("needsAutoPRRefresh() = false, want legacy cache refresh")
	}

	session.PR.UnresolvedCommentsChecked = true
	if needsAutoPRRefresh(session, now) {
		t.Fatal("needsAutoPRRefresh() = true after comment refresh attempt")
	}
}

func TestDashboardLoadAutoRefreshesMissingAndStalePRCache(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	now := time.Now().UTC()
	sessions := []Session{
		{ID: "missing-pr", Root: "/repo", Workdir: "/repo", Branch: "feature/missing"},
		{ID: "stale-pr", Root: "/repo", Workdir: "/repo", Branch: "feature/stale", PR: &PRInfo{RefreshedAt: now.Add(-prAutoRefreshMaxAge - time.Minute)}},
		{ID: "fresh-pr", Root: "/repo", Workdir: "/repo", Branch: "feature/fresh", PR: &PRInfo{RefreshedAt: now}},
		{ID: "no-branch", Root: "/repo", Workdir: "/repo"},
	}
	for _, session := range sessions {
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
	}
	ghCalls := 0
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch name {
			case "git":
				return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
			case "gh":
				if argsContain(args, "api") {
					return []byte(`[{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}]`), nil
				}
				ghCalls++
				return []byte(`[{"number":11,"url":"https://github.com/oysandvik94/oak-tree/pull/11","title":"Auto","state":"OPEN"}]`), nil
			default:
				return nil, nil
			}
		},
	}
	model := NewDashboardModel(NewService(Paths{StateDir: stateDir}, store, runner), Config{})

	updated, cmd := model.Update(dashboardMsg{sessions: sessions})
	next := updated.(DashboardModel)
	if next.status != "refreshing PRs" {
		t.Fatalf("status = %q, want refreshing PRs", next.status)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("dashboard load command message = %T, want tea.BatchMsg", cmd())
	}

	var prMsgs []prRefreshMsg
	for _, batchCmd := range batch {
		if msg, ok := batchCmd().(prRefreshMsg); ok {
			prMsgs = append(prMsgs, msg)
		}
	}
	if len(prMsgs) != 2 {
		t.Fatalf("auto PR refresh messages = %d, want 2", len(prMsgs))
	}
	for _, msg := range prMsgs {
		if !msg.auto {
			t.Fatalf("prRefreshMsg.auto = false for %#v", msg)
		}
		if msg.sessionID != "missing-pr" && msg.sessionID != "stale-pr" {
			t.Fatalf("auto refreshed session %q, want missing-pr or stale-pr", msg.sessionID)
		}
	}
	if ghCalls != 2 {
		t.Fatalf("gh calls = %d, want 2", ghCalls)
	}

	for _, msg := range prMsgs {
		updated, _ = next.Update(msg)
		next = updated.(DashboardModel)
	}
	if next.status != "ready" {
		t.Fatalf("status after auto PR refresh = %q, want ready", next.status)
	}
}

func TestPeriodicSessionRefreshAutoRefreshesStalePRCache(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	now := time.Now().UTC()
	sessions := []Session{
		{ID: "stale-pr", Root: "/repo", Workdir: "/repo", Branch: "feature/stale", PR: &PRInfo{RefreshedAt: now.Add(-prAutoRefreshMaxAge - time.Minute)}},
	}
	for _, session := range sessions {
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
	}
	ghCalls := 0
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch name {
			case "git":
				return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
			case "gh":
				if argsContain(args, "api") {
					return []byte(`[{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}]`), nil
				}
				ghCalls++
				return []byte(`[{"number":12,"url":"https://github.com/oysandvik94/oak-tree/pull/12","title":"Auto","state":"OPEN"}]`), nil
			default:
				return nil, nil
			}
		},
	}
	model := NewDashboardModel(NewService(Paths{StateDir: stateDir}, store, runner), Config{})
	model.sessions = sessions

	updated, cmd := model.Update(gitStatusRefreshMsg{sessions: sessions})
	next := updated.(DashboardModel)
	if next.status != "refreshing PRs" {
		t.Fatalf("status = %q, want refreshing PRs", next.status)
	}
	msg, ok := cmd().(prRefreshMsg)
	if !ok {
		t.Fatalf("periodic refresh command message = %T, want prRefreshMsg", cmd())
	}
	if !msg.auto || msg.sessionID != "stale-pr" {
		t.Fatalf("prRefreshMsg = %#v, want auto stale-pr refresh", msg)
	}
	if ghCalls != 1 {
		t.Fatalf("gh calls = %d, want 1", ghCalls)
	}
}

func TestUsageTickChecksCacheAndSchedulesNextTick(t *testing.T) {
	stateDir := t.TempDir()
	model := NewDashboardModel(NewService(Paths{StateDir: stateDir}, NewStore(stateDir), &stubRunner{}), Config{})

	_, cmd := model.Update(usageTickMsg{})
	if cmd == nil {
		t.Fatal("usage tick returned nil command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("usage tick command message = %T, want tea.BatchMsg", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("usage tick batch length = %d, want cache check and next tick", len(batch))
	}
	if msg, ok := batch[0]().(usageCacheMsg); !ok || msg.err != nil {
		t.Fatalf("first usage tick command = %#v, want successful usageCacheMsg", msg)
	}
}

func TestTruncateMiddle(t *testing.T) {
	got := truncateMiddle("abcdef", 5)
	if got != "ab…ef" {
		t.Fatalf("truncateMiddle() = %q, want %q", got, "ab…ef")
	}
}
