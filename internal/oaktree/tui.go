package oaktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	clipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/lipgloss"
)

type dashboardMsg struct {
	sessions []Session
	err      error
}

type animationTickMsg struct{}

type createResultMsg struct {
	session Session
	err     error
}

type branchListMsg struct {
	root     string
	branches []string
	err      error
}

type closeResultMsg struct {
	sessionID string
	err       error
}

type attachResultMsg struct {
	err error
}

type prRefreshMsg struct {
	sessionID string
	session   Session
	err       error
	auto      bool
}

type openPRMsg struct {
	err error
}

type tagUpdateMsg struct {
	session Session
	err     error
}

type noteUpdateMsg struct {
	session Session
	err     error
}

type usageCacheMsg struct {
	state UsageCacheState
	err   error
}

type usageRefreshMsg struct {
	cache  UsageCache
	err    error
	silent bool
}

type usageTickMsg struct{}

type agentStatusRefreshMsg struct {
	sessions []Session
	err      error
}

type gitStatusTickMsg struct{}

type gitStatusRefreshMsg struct {
	sessions []Session
	err      error
}

const agentStatusRefreshInterval = time.Second
const gitStatusRefreshInterval = 4 * time.Second
const usageRefreshInterval = UsageCacheMaxAge
const prAutoRefreshMaxAge = 10 * time.Minute
const animationFrameRate = 24
const animationTickInterval = time.Second / animationFrameRate

type mode int

const (
	modeDashboard mode = iota
	modeCreate
	modeConfirmClose
	modeTagPicker
	modeNoteEditor
)

type tagOption struct {
	Tag   SessionTag
	Label string
	Color string
}

var sessionTagOptions = []tagOption{
	{Tag: SessionTagNone, Label: "active", Color: "81"},
	{Tag: SessionTagWaitingReview, Label: "waiting review", Color: "244"},
	{Tag: SessionTagBlocked, Label: "blocked", Color: "203"},
}

type createStage int

const (
	createStagePicker createStage = iota
	createStageManual
	createStageBranch
)

type createForm struct {
	stage            createStage
	rootSearch       textinput.Model
	rootManual       textinput.Model
	branch           textinput.Model
	branchMode       BranchMode
	rootList         list.Model
	rootCandidates   []rootCandidate
	filtered         []rootCandidate
	branchCandidates []string
	filteredBranches []string
	branchCursor     int
	branchesLoading  bool
	selectedRootPath string
}

func newCreateInput(prompt, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = prompt
	input.Placeholder = placeholder
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = clipgloss.NewStyle().Foreground(clipgloss.Color("81")).Bold(true)
	styles.Focused.Text = clipgloss.NewStyle().Foreground(clipgloss.Color("229")).Bold(true)
	styles.Focused.Placeholder = clipgloss.NewStyle().Foreground(clipgloss.Color("244"))
	styles.Blurred.Prompt = clipgloss.NewStyle().Foreground(clipgloss.Color("245"))
	styles.Cursor.Color = clipgloss.Color("81")
	input.SetStyles(styles)
	return input
}

func newCreateForm(candidates []rootCandidate, width, height int) createForm {
	form := createForm{
		rootCandidates: append([]rootCandidate(nil), candidates...),
		rootSearch:     newCreateInput("search> ", "type to filter roots"),
		rootManual:     newCreateInput("root> ", "enter a root path"),
		branch:         newCreateInput("branch> ", "optional branch"),
		branchMode:     BranchModeCreateNew,
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	form.rootList = list.New(rootCandidateItems(form.rootCandidates), delegate, width, max(4, height/3))
	form.rootList.SetShowTitle(false)
	form.rootList.SetShowFilter(false)
	form.rootList.SetShowStatusBar(false)
	form.rootList.SetShowPagination(false)
	form.rootList.SetShowHelp(false)
	form.rootList.SetFilteringEnabled(false)
	form.syncLayout(width, height)
	form.applyFilter()
	if len(form.rootCandidates) == 0 {
		form.stage = createStageManual
	} else {
		form.stage = createStagePicker
	}
	return form
}

func (f *createForm) reset(candidates []rootCandidate, width, height int) {
	*f = newCreateForm(candidates, width, height)
}

func (f *createForm) syncLayout(width, height int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	fieldWidth := max(24, width-6)
	listHeight := max(4, height/3)
	f.rootSearch.SetWidth(fieldWidth)
	f.rootManual.SetWidth(fieldWidth)
	f.branch.SetWidth(fieldWidth)
	f.rootList.SetSize(fieldWidth, listHeight)
}

func (f *createForm) blurAll() {
	f.rootSearch.Blur()
	f.rootManual.Blur()
	f.branch.Blur()
}

func (f *createForm) focusActive() tea.Cmd {
	f.blurAll()
	switch f.stage {
	case createStageManual:
		f.rootManual.CursorEnd()
		return f.rootManual.Focus()
	case createStageBranch:
		f.branch.CursorEnd()
		return f.branch.Focus()
	default:
		f.rootSearch.CursorEnd()
		return f.rootSearch.Focus()
	}
}

func (f *createForm) hasCandidates() bool {
	return len(f.rootCandidates) > 0
}

func (f *createForm) currentPickerRoot() string {
	item := f.rootList.SelectedItem()
	if item == nil {
		return ""
	}
	candidate, ok := item.(rootCandidate)
	if !ok {
		return ""
	}
	return candidate.Path
}

func (f *createForm) selectedRoot() string {
	switch f.stage {
	case createStagePicker:
		if path := f.currentPickerRoot(); path != "" {
			return path
		}
	case createStageManual:
		return normalizeCreateRootPath(f.rootManual.Value())
	case createStageBranch:
		return f.selectedRootPath
	}
	return ""
}

func (f *createForm) applyFilter() {
	prev := f.currentPickerRoot()
	f.filtered = filterRootCandidates(f.rootSearch.Value(), f.rootCandidates)
	f.rootList.SetItems(rootCandidateItems(f.filtered))
	if len(f.filtered) == 0 {
		return
	}
	if prev != "" {
		for i, candidate := range f.filtered {
			if candidate.Path == prev {
				f.rootList.Select(i)
				return
			}
		}
	}
	f.rootList.ResetSelected()
}

func (f *createForm) moveRootCursor(key string) {
	switch key {
	case "up":
		f.rootList.CursorUp()
	case "down":
		f.rootList.CursorDown()
	case "pgup":
		f.rootList.PrevPage()
	case "pgdown":
		f.rootList.NextPage()
	}
}

func (f *createForm) toggleBranchMode() {
	if f.branchMode == BranchModeOpenExisting {
		f.branchMode = BranchModeCreateNew
		return
	}
	f.branchMode = BranchModeOpenExisting
	f.applyBranchFilter()
}

func (f *createForm) applyBranchFilter() {
	ranks := list.DefaultFilter(strings.TrimSpace(f.branch.Value()), f.branchCandidates)
	f.filteredBranches = make([]string, 0, len(ranks))
	for _, rank := range ranks {
		f.filteredBranches = append(f.filteredBranches, f.branchCandidates[rank.Index])
	}
	f.branchCursor = 0
}

func (f *createForm) selectedBranch() string {
	if f.branchMode == BranchModeOpenExisting && f.branchCursor < len(f.filteredBranches) {
		return f.filteredBranches[f.branchCursor]
	}
	return strings.TrimSpace(f.branch.Value())
}

func (f *createForm) moveBranchCursor(key string) {
	if len(f.filteredBranches) == 0 {
		return
	}
	if key == "up" && f.branchCursor > 0 {
		f.branchCursor--
	}
	if key == "down" && f.branchCursor+1 < len(f.filteredBranches) {
		f.branchCursor++
	}
}

func (f createForm) branchModeLabel() string {
	if f.branchMode == BranchModeOpenExisting {
		return "open existing"
	}
	return "create new"
}

func (f createForm) branchModeColor() string {
	if f.branchMode == BranchModeOpenExisting {
		return "214"
	}
	return "81"
}

func (f *createForm) enterRoot() (string, error) {
	switch f.stage {
	case createStagePicker:
		path := f.currentPickerRoot()
		if path == "" {
			query := strings.TrimSpace(f.rootSearch.Value())
			if query == "" {
				return "", fmt.Errorf("no root selected")
			}
			return "", fmt.Errorf("no roots match %q", query)
		}
		return path, nil
	case createStageManual:
		path := normalizeCreateRootPath(f.rootManual.Value())
		if path == "" {
			return "", fmt.Errorf("root is required")
		}
		return path, nil
	default:
		return "", fmt.Errorf("root already selected")
	}
}

func (f *createForm) stageHelp() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	switch f.stage {
	case createStagePicker:
		return dim.Render("enter selects root   tab manual path   esc cancel")
	case createStageManual:
		if f.hasCandidates() {
			return dim.Render("enter continues   tab fuzzy picker   esc cancel")
		}
		return dim.Render("enter continues   esc cancel")
	case createStageBranch:
		return dim.Render("enter launches session   ctrl+o mode   ↑/↓ select   tab change root   esc cancel")
	default:
		return ""
	}
}

func (f *createForm) render() string {
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("81")).
		Padding(0, 2)
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("✦ New session")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	var b strings.Builder
	b.WriteString(title)
	b.WriteString(" ")
	b.WriteString(dim.Render(f.stageLabel()))
	b.WriteByte('\n')
	switch f.stage {
	case createStagePicker:
		b.WriteString(dim.Render("choose a repository root from configured search dirs"))
		b.WriteByte('\n')
		b.WriteString(f.rootSearch.View())
		b.WriteByte('\n')
		b.WriteString(f.renderRootCandidates())
	case createStageManual:
		b.WriteString(dim.Render("manual root entry"))
		b.WriteByte('\n')
		b.WriteString(f.rootManual.View())
	case createStageBranch:
		b.WriteString(dim.Render("selected root: "))
		b.WriteString(compactPath(f.selectedRootPath, 72))
		b.WriteByte('\n')
		b.WriteString(dim.Render("mode: "))
		b.WriteString(renderTinyPill(strings.ToUpper(f.branchModeLabel()), f.branchModeColor()))
		b.WriteByte('\n')
		b.WriteString(f.branch.View())
		if f.branchMode == BranchModeOpenExisting {
			b.WriteByte('\n')
			b.WriteString(f.renderBranchCandidates())
		}
	}
	b.WriteByte('\n')
	b.WriteString(f.stageHelp())
	return panel.Render(b.String())
}

func (f *createForm) renderBranchCandidates() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if f.branchesLoading {
		return dim.Render("loading branches…")
	}
	if len(f.filteredBranches) == 0 {
		return dim.Render("No matching branches; type a branch name manually.")
	}
	start := max(0, f.branchCursor-2)
	end := min(len(f.filteredBranches), start+5)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		prefix := "  "
		style := dim
		if i == f.branchCursor {
			prefix = "▸ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
		}
		lines = append(lines, style.Render(prefix+f.filteredBranches[i]))
	}
	return strings.Join(lines, "\n")
}

func (f *createForm) stageLabel() string {
	switch f.stage {
	case createStagePicker:
		return renderTinyPill("01 ROOT", "81")
	case createStageManual:
		return renderTinyPill("01 MANUAL", "214")
	case createStageBranch:
		return renderTinyPill("02 BRANCH", "170")
	default:
		return ""
	}
}

func (f *createForm) renderRootCandidates() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	if len(f.filtered) == 0 {
		query := strings.TrimSpace(f.rootSearch.Value())
		if query == "" {
			return dim.Render("No configured root directories found.")
		}
		return dim.Render(fmt.Sprintf("No roots match %q.", query))
	}
	cursor := f.rootList.Index()
	if cursor < 0 || cursor >= len(f.filtered) {
		cursor = 0
	}
	visible := f.visibleRootCandidates(cursor)
	lines := make([]string, 0, visible.end-visible.start+2)
	if visible.start > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("… %d earlier", visible.start)))
	}
	for i := visible.start; i < visible.end; i++ {
		candidate := f.filtered[i]
		marker := "  "
		name := titleStyle.Render(candidate.Title())
		path := pathStyle.Render(candidate.Description())
		if i == cursor {
			marker = accent.Render("▸ ")
			name = selectedStyle.Render(candidate.Title())
			path = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(candidate.Description())
		}
		lines = append(lines, marker+name+" "+path)
	}
	if remaining := len(f.filtered) - visible.end; remaining > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("… %d more", remaining)))
	}
	return strings.Join(lines, "\n")
}

type rootCandidateWindow struct {
	start int
	end   int
}

func (f *createForm) visibleRootCandidates(cursor int) rootCandidateWindow {
	limit := max(4, f.rootList.Height()-1)
	if limit > len(f.filtered) {
		limit = len(f.filtered)
	}
	start := 0
	if cursor >= limit {
		start = cursor - limit + 1
	}
	if start > len(f.filtered)-limit {
		start = max(0, len(f.filtered)-limit)
	}
	return rootCandidateWindow{start: start, end: min(len(f.filtered), start+limit)}
}

func normalizeCreateRootPath(value string) string {
	normalized, err := normalizeConfigPath(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return normalized
}

func rootCandidateItems(candidates []rootCandidate) []list.Item {
	items := make([]list.Item, len(candidates))
	for i := range candidates {
		items[i] = candidates[i]
	}
	return items
}

type DashboardModel struct {
	svc                *Service
	sessions           []Session
	selected           int
	help               help.Model
	usage              UsageCache
	usageLoaded        bool
	usageStale         bool
	usageErr           error
	status             string
	err                error
	width              int
	height             int
	mode               mode
	form               createForm
	rootCandidates     []rootCandidate
	confirmClose       bool
	tagCursor          int
	tagUpdating        bool
	noteInput          textinput.Model
	noteUpdating       bool
	creating           bool
	attaching          bool
	closing            bool
	prRefreshing       bool
	prRefreshInFlight  map[string]struct{}
	prAutoRefreshing   int
	openingPR          bool
	usageRefreshing    bool
	motionFrame        int
	motionTickerActive bool
	todoExpanded       bool
	kanbanView         bool
	statusSeenAt       map[string]time.Time
}

func NewDashboardModel(svc *Service, cfg Config) DashboardModel {
	rootCandidates := discoverRootCandidates(cfg.RootSearchDirs, cfg.Roots)
	helpModel := help.New()
	helpModel.Ellipsis = "…"
	helpModel.ShortSeparator = " "
	helpModel.Styles.ShortKey = clipgloss.NewStyle().
		Foreground(clipgloss.Color("254")).
		Padding(0, 1).
		Bold(true)
	helpModel.Styles.ShortDesc = clipgloss.NewStyle().Foreground(clipgloss.Color("246"))
	helpModel.Styles.ShortSeparator = clipgloss.NewStyle().Foreground(clipgloss.Color("238"))
	model := DashboardModel{
		svc:            svc,
		mode:           modeDashboard,
		form:           newCreateForm(rootCandidates, 80, 24),
		noteInput:      newCreateInput("note> ", "why is this parked?"),
		rootCandidates: rootCandidates,
		help:           helpModel,
		status:         "ready",
	}
	if svc != nil && svc.Store != nil {
		if preferences, err := svc.Store.LoadDashboardPreferences(); err == nil {
			model.kanbanView = preferences.KanbanView
			model.statusSeenAt = preferences.StatusSeenAt
		}
	}
	return model
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), m.usageCacheCmd(), m.usageTickCmd(), m.agentStatusRefreshCmd(), m.gitStatusTickCmd())
}

func (m DashboardModel) agentStatusRefreshCmd() tea.Cmd {
	return tea.Tick(agentStatusRefreshInterval, func(time.Time) tea.Msg {
		sessions, err := m.svc.ListSessionsWithAgentStatus(context.Background())
		return agentStatusRefreshMsg{sessions: sessions, err: err}
	})
}

func (m DashboardModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.svc.ListSessions(context.Background())
		return dashboardMsg{sessions: sessions, err: err}
	}
}

func (m DashboardModel) gitStatusTickCmd() tea.Cmd {
	return tea.Tick(gitStatusRefreshInterval, func(time.Time) tea.Msg {
		return gitStatusTickMsg{}
	})
}

func (m DashboardModel) gitStatusRefreshCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.svc.ListSessions(context.Background())
		return gitStatusRefreshMsg{sessions: sessions, err: err}
	}
}

func (m DashboardModel) animationTickCmd() tea.Cmd {
	return tea.Tick(animationTickInterval, func(time.Time) tea.Msg {
		return animationTickMsg{}
	})
}

func (m DashboardModel) branchListCmd(root string) tea.Cmd {
	return func() tea.Msg {
		branches, err := ExistingBranches(context.Background(), m.svc.Exec, root)
		return branchListMsg{root: root, branches: branches, err: err}
	}
}

func (m DashboardModel) createCmd(root, branch string, branchMode BranchMode) tea.Cmd {
	return func() tea.Msg {
		session, err := m.svc.CreateSession(context.Background(), CreateSessionInput{Root: root, Branch: branch, BranchMode: branchMode})
		return createResultMsg{session: session, err: err}
	}
}

func (m DashboardModel) closeCmd(session Session, fallbackTmuxSession string) tea.Cmd {
	return func() tea.Msg {
		err := m.svc.CloseSessionWithFallback(context.Background(), session.ID, fallbackTmuxSession)
		return closeResultMsg{sessionID: session.ID, err: err}
	}
}

func (m DashboardModel) attachCmd(session Session) tea.Cmd {
	return func() tea.Msg {
		if session.AgentStatus == AgentStatusAttention {
			if _, err := m.svc.AcknowledgeAgentAttention(context.Background(), session.ID); err != nil {
				return attachResultMsg{err: err}
			}
		}
		err := m.svc.AttachSession(context.Background(), session)
		return attachResultMsg{err: err}
	}
}

func (m DashboardModel) prRefreshCmd(session Session, auto bool) tea.Cmd {
	return func() tea.Msg {
		updated, err := m.svc.RefreshSessionPR(context.Background(), session.ID)
		return prRefreshMsg{sessionID: session.ID, session: updated, err: err, auto: auto}
	}
}

func (m DashboardModel) openPRCmd(session Session) tea.Cmd {
	return func() tea.Msg {
		err := m.svc.OpenSessionPR(context.Background(), session)
		return openPRMsg{err: err}
	}
}

func (m DashboardModel) tagUpdateCmd(session Session, tag SessionTag) tea.Cmd {
	return func() tea.Msg {
		updated, err := m.svc.SetSessionTag(context.Background(), session.ID, tag)
		return tagUpdateMsg{session: updated, err: err}
	}
}

func (m DashboardModel) noteUpdateCmd(session Session, note string) tea.Cmd {
	return func() tea.Msg {
		updated, err := m.svc.SetSessionNote(context.Background(), session.ID, note)
		return noteUpdateMsg{session: updated, err: err}
	}
}

func (m DashboardModel) usageCacheCmd() tea.Cmd {
	return func() tea.Msg {
		state, err := m.svc.LoadCachedUsage(UsageCacheMaxAge)
		return usageCacheMsg{state: state, err: err}
	}
}

func (m DashboardModel) usageRefreshCmd(silent bool) tea.Cmd {
	return func() tea.Msg {
		cache, err := m.svc.RefreshUsage(context.Background())
		return usageRefreshMsg{cache: cache, err: err, silent: silent}
	}
}

func (m DashboardModel) usageTickCmd() tea.Cmd {
	return tea.Tick(usageRefreshInterval, func(time.Time) tea.Msg {
		return usageTickMsg{}
	})
}

func (m DashboardModel) closeFallbackTmuxSession() string {
	if m.selected+1 < len(m.sessions) {
		return m.sessions[m.selected+1].TmuxSessionName
	}
	if m.selected > 0 {
		return m.sessions[m.selected-1].TmuxSessionName
	}
	return ""
}

func (m DashboardModel) currentSession() *Session {
	if len(m.sessions) == 0 {
		return nil
	}
	if m.selected < 0 || m.selected >= len(m.sessions) {
		return nil
	}
	session := m.sessions[m.selected]
	return &session
}

func (m DashboardModel) withSelected(id string) DashboardModel {
	for i := range m.sessions {
		if m.sessions[i].ID == id {
			m.selected = i
			return m
		}
	}
	if len(m.sessions) > 0 && m.selected >= len(m.sessions) {
		m.selected = len(m.sessions) - 1
	}
	return m
}

func (m DashboardModel) withSessionsPreservingSelection(sessions []Session) DashboardModel {
	return m.withSessions(sessions, false)
}

func (m DashboardModel) withSessionsPreservingVisibleOrder(sessions []Session) DashboardModel {
	return m.withSessions(sessions, true)
}

func (m DashboardModel) withSessions(sessions []Session, preserveVisibleOrder bool) DashboardModel {
	prevID := ""
	if sel := m.currentSession(); sel != nil {
		prevID = sel.ID
	}
	if preserveVisibleOrder && len(m.sessions) > 0 {
		byID := make(map[string]Session, len(sessions))
		for _, session := range sessions {
			byID[session.ID] = session
		}
		merged := make([]Session, 0, len(sessions))
		seen := make(map[string]struct{}, len(sessions))
		for _, current := range m.sessions {
			session, ok := byID[current.ID]
			if !ok {
				continue
			}
			merged = append(merged, sessionWithPreservedTransient(current, session))
			seen[session.ID] = struct{}{}
		}
		for _, session := range sessions {
			if _, ok := seen[session.ID]; ok {
				continue
			}
			merged = append(merged, session)
		}
		m.sessions = merged
	} else {
		m.sessions = sessions
	}
	m.sessions = orderSessionsForDashboard(m.sessions)
	if prevID != "" {
		for i := range m.sessions {
			if m.sessions[i].ID == prevID {
				m.selected = i
				return m
			}
		}
	}
	if m.selected >= len(m.sessions) {
		m.selected = len(m.sessions) - 1
	}
	if len(m.sessions) == 0 {
		m.selected = 0
	}
	return m
}

func sessionWithPreservedTransient(previous, next Session) Session {
	if next.GitStatus == nil {
		next.GitStatus = previous.GitStatus
	}
	return next
}

func sessionStatusUpdatedAt(session Session) time.Time {
	latest := session.AgentStatusUpdatedAt
	if latest == nil || session.TagUpdatedAt != nil && session.TagUpdatedAt.After(*latest) {
		latest = session.TagUpdatedAt
	}
	if latest == nil {
		return time.Time{}
	}
	return *latest
}

func (m DashboardModel) hasUnseenStatus(session Session) bool {
	seen, ok := m.statusSeenAt[session.ID]
	return ok && sessionStatusUpdatedAt(session).After(seen)
}

func (m DashboardModel) syncStatusSeenAt() DashboardModel {
	if m.statusSeenAt == nil {
		m.statusSeenAt = map[string]time.Time{}
	}
	changed := false
	for _, session := range m.sessions {
		if _, ok := m.statusSeenAt[session.ID]; !ok {
			m.statusSeenAt[session.ID] = sessionStatusUpdatedAt(session)
			changed = true
		}
	}
	if selected := m.currentSession(); selected != nil {
		updatedAt := sessionStatusUpdatedAt(*selected)
		if seen := m.statusSeenAt[selected.ID]; updatedAt.After(seen) {
			m.statusSeenAt[selected.ID] = updatedAt
			changed = true
		}
	}
	if changed && m.svc != nil && m.svc.Store != nil {
		if err := m.svc.Store.SaveDashboardPreferences(DashboardPreferences{KanbanView: m.kanbanView, StatusSeenAt: m.statusSeenAt}); err != nil {
			m.err = err
			m.status = "view state save failed"
		}
	}
	return m
}

func (m DashboardModel) preserveAgentSnapshotTransient(sessions []Session) []Session {
	previous := make(map[string]Session, len(m.sessions))
	for _, session := range m.sessions {
		previous[session.ID] = session
	}
	for i := range sessions {
		if current, ok := previous[sessions[i].ID]; ok {
			sessions[i].Branch = current.Branch
			sessions[i].PR = current.PR
			sessions[i].GitStatus = current.GitStatus
		}
	}
	return sessions
}

func orderSessionsForDashboard(sessions []Session) []Session {
	ordered := append([]Session(nil), sessions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return sessionDashboardPriority(ordered[i]) < sessionDashboardPriority(ordered[j])
	})
	return ordered
}

func sessionDashboardPriority(session Session) int {
	return sessionDashboardBucket(session)
}

func sessionTagOptionIndex(tag SessionTag) int {
	for i, option := range sessionTagOptions {
		if option.Tag == tag {
			return i
		}
	}
	return 0
}

func sessionTagLabel(tag SessionTag) string {
	return sessionTagOptions[sessionTagOptionIndex(tag)].Label
}

func sessionTagColor(tag SessionTag) string {
	return sessionTagOptions[sessionTagOptionIndex(tag)].Color
}

func (m DashboardModel) prRefreshIsInFlight(sessionID string) bool {
	if m.prRefreshInFlight == nil {
		return false
	}
	_, ok := m.prRefreshInFlight[sessionID]
	return ok
}

func (m DashboardModel) markPRRefreshInFlight(sessionID string) DashboardModel {
	if sessionID == "" {
		return m
	}
	if m.prRefreshInFlight == nil {
		m.prRefreshInFlight = map[string]struct{}{}
	}
	m.prRefreshInFlight[sessionID] = struct{}{}
	return m
}

func (m DashboardModel) clearPRRefreshInFlight(sessionID string) DashboardModel {
	if m.prRefreshInFlight != nil {
		delete(m.prRefreshInFlight, sessionID)
	}
	return m
}

func (m DashboardModel) autoPRRefreshCmds(now time.Time) ([]tea.Cmd, DashboardModel) {
	var cmds []tea.Cmd
	for _, session := range m.sessions {
		if !needsAutoPRRefresh(session, now) || m.prRefreshIsInFlight(session.ID) {
			continue
		}
		m = m.markPRRefreshInFlight(session.ID)
		m.prAutoRefreshing++
		cmds = append(cmds, m.prRefreshCmd(session, true))
	}
	return cmds, m
}

func needsAutoPRRefresh(session Session, now time.Time) bool {
	if strings.TrimSpace(session.Branch) == "" {
		return false
	}
	if session.PR == nil || session.PR.RefreshedAt.IsZero() {
		return true
	}
	if session.PR.Found && !session.PR.UnresolvedCommentsChecked {
		return true
	}
	return now.Sub(session.PR.RefreshedAt) >= prAutoRefreshMaxAge
}

func (m DashboardModel) beginCreate() (DashboardModel, tea.Cmd) {
	m.form.reset(m.rootCandidates, m.width, m.height)
	m.mode = modeCreate
	m.status = "create session"
	m.err = nil
	return m.withAnimationCmd(m.form.focusActive())
}

func (m DashboardModel) cancelCreate() DashboardModel {
	m.mode = modeDashboard
	m.status = "cancelled"
	m.err = nil
	return m
}

func (m DashboardModel) enterCreateRoot() (DashboardModel, tea.Cmd) {
	root, err := m.form.enterRoot()
	if err != nil {
		m.err = err
		return m, nil
	}
	m.err = nil
	m.form.selectedRootPath = root
	m.form.stage = createStageBranch
	m.status = "branch optional"
	return m.withAnimationCmd(m.form.focusActive())
}

func (m DashboardModel) tabCreateStage() (DashboardModel, tea.Cmd) {
	switch m.form.stage {
	case createStagePicker:
		m.form.stage = createStageManual
	case createStageManual:
		if root := strings.TrimSpace(m.form.rootManual.Value()); root != "" {
			m.form.selectedRootPath = normalizeCreateRootPath(root)
			m.form.stage = createStageBranch
			break
		}
		if !m.form.hasCandidates() {
			return m, nil
		}
		m.form.stage = createStagePicker
	case createStageBranch:
		if m.form.hasCandidates() {
			m.form.stage = createStagePicker
		} else {
			m.form.stage = createStageManual
		}
	}
	m.err = nil
	return m.withAnimationCmd(m.form.focusActive())
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.mode == modeCreate {
			m.form.syncLayout(msg.Width, msg.Height)
		} else if m.mode == modeNoteEditor {
			m.noteInput.SetWidth(max(12, min(msg.Width, 100)-4))
		}
		return m, nil
	case dashboardMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "refresh failed"
			return m, nil
		}
		m.err = nil
		m = m.withSessionsPreservingSelection(msg.sessions).syncStatusSeenAt()
		if len(m.sessions) == 0 {
			if m.status == "refreshing" {
				m.status = "ready"
			}
			return m.withAnimationCmd()
		}
		var cmds []tea.Cmd
		var prCmds []tea.Cmd
		prCmds, m = m.autoPRRefreshCmds(time.Now())
		if len(prCmds) > 0 {
			cmds = append(cmds, prCmds...)
			m.status = "refreshing PRs"
		} else if m.status == "refreshing" {
			m.status = "ready"
		}
		return m.withAnimationCmd(cmds...)
	case agentStatusRefreshMsg:
		if msg.err == nil {
			m = m.withSessionsPreservingVisibleOrder(m.preserveAgentSnapshotTransient(msg.sessions)).syncStatusSeenAt()
		}
		cmds := []tea.Cmd{m.agentStatusRefreshCmd()}
		return m.withAnimationCmd(cmds...)
	case gitStatusTickMsg:
		return m, tea.Batch(m.gitStatusRefreshCmd(), m.gitStatusTickCmd())
	case gitStatusRefreshMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = "git refresh failed"
			return m, nil
		}
		m = m.withSessionsPreservingVisibleOrder(msg.sessions)
		prCmds, updated := m.autoPRRefreshCmds(time.Now())
		m = updated
		if len(prCmds) > 0 {
			m.status = "refreshing PRs"
			return m, tea.Batch(prCmds...)
		}
		return m, nil
	case branchListMsg:
		if m.mode != modeCreate || m.form.selectedRootPath != msg.root {
			return m, nil
		}
		m.form.branchesLoading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.form.branchCandidates = msg.branches
		m.form.applyBranchFilter()
		return m.withAnimationCmd()
	case createResultMsg:
		m.creating = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "create failed"
			m.mode = modeDashboard
			return m, nil
		}
		m.err = nil
		m.mode = modeDashboard
		m.status = "session created"
		return m, m.refreshCmd()
	case closeResultMsg:
		m.closing = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "close failed"
			return m, nil
		}
		m.err = nil
		m.status = "session closed"
		return m, m.refreshCmd()
	case attachResultMsg:
		m.attaching = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "attach failed"
			return m, nil
		}
		m.err = nil
		return m, tea.Quit
	case prRefreshMsg:
		m = m.clearPRRefreshInFlight(msg.sessionID)
		if msg.auto && m.prAutoRefreshing > 0 {
			m.prAutoRefreshing--
		}
		if !msg.auto {
			m.prRefreshing = false
		}
		if msg.err != nil {
			if msg.auto {
				if m.prAutoRefreshing == 0 {
					m.status = "PR auto refresh failed"
				}
			} else {
				m.err = msg.err
				m.status = "PR refresh failed"
			}
			return m, nil
		}
		if !msg.auto {
			m.err = nil
		}
		stale := false
		for i := range m.sessions {
			if m.sessions[i].ID != msg.session.ID {
				continue
			}
			currentBranch := ""
			if m.sessions[i].GitStatus != nil {
				currentBranch = m.sessions[i].GitStatus.Branch
			}
			if currentBranch != "" && msg.session.Branch != "" && currentBranch != msg.session.Branch {
				stale = true
				break
			}
			m.sessions[i] = sessionWithPreservedTransient(m.sessions[i], msg.session)
			break
		}
		if stale {
			if m.prAutoRefreshing == 0 || !msg.auto {
				m.status = "ready"
			}
			return m, nil
		}
		if msg.auto {
			if m.prAutoRefreshing == 0 && m.status == "refreshing PRs" {
				m.status = "ready"
			}
		} else {
			m.status = prStatusMessage(msg.session.PR)
		}
		return m, nil
	case openPRMsg:
		m.openingPR = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "open PR failed"
			return m, nil
		}
		m.err = nil
		m.status = "opened PR"
		return m, nil
	case tagUpdateMsg:
		m.tagUpdating = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "status update failed"
			return m, nil
		}
		m.err = nil
		m.mode = modeDashboard
		for i := range m.sessions {
			if m.sessions[i].ID == msg.session.ID {
				m.sessions[i] = sessionWithPreservedTransient(m.sessions[i], msg.session)
				break
			}
		}
		m.sessions = orderSessionsForDashboard(m.sessions)
		m = m.withSelected(msg.session.ID)
		m.status = "marked " + sessionTagLabel(msg.session.Tag)
		return m.syncStatusSeenAt(), nil
	case noteUpdateMsg:
		m.noteUpdating = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "note update failed"
			return m, m.noteInput.Focus()
		}
		m.err = nil
		m.mode = modeDashboard
		for i := range m.sessions {
			if m.sessions[i].ID == msg.session.ID {
				m.sessions[i] = sessionWithPreservedTransient(m.sessions[i], msg.session)
				break
			}
		}
		m.status = "note saved"
		return m, nil
	case usageCacheMsg:
		if msg.err != nil {
			m.usageErr = msg.err
			return m, nil
		}
		if msg.state.Found {
			m.usage = msg.state.Cache
			m.usageLoaded = true
			m.usageStale = msg.state.Stale
		}
		if (!msg.state.Found || msg.state.Stale) && !m.usageRefreshing {
			m.usageRefreshing = true
			return m, m.usageRefreshCmd(true)
		}
		return m, nil
	case usageRefreshMsg:
		m.usageRefreshing = false
		if msg.err != nil {
			m.usageErr = msg.err
			if !msg.silent {
				m.status = "usage refresh failed"
			}
			return m, nil
		}
		m.usage = msg.cache
		m.usageLoaded = true
		m.usageStale = false
		m.usageErr = nil
		if !msg.silent {
			m.status = "usage refreshed"
		}
		return m, nil
	case usageTickMsg:
		return m, tea.Batch(m.usageCacheCmd(), m.usageTickCmd())
	case animationTickMsg:
		m.motionTickerActive = false
		m.motionFrame++
		return m.withAnimationCmd()
	case tea.KeyMsg:
		switch m.mode {
		case modeCreate:
			return m.updateCreate(msg)
		case modeTagPicker:
			return m.updateTagPicker(msg)
		case modeNoteEditor:
			return m.updateNoteEditor(msg)
		default:
			return m.updateDashboard(msg)
		}
	}
	return m, nil
}

func (m DashboardModel) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.err = nil
		m.status = "refreshing"
		return m, m.refreshCmd()
	case "u":
		if !m.usageRefreshing {
			m.usageRefreshing = true
			m.usageErr = nil
			m.status = "refreshing usage"
			return m, m.usageRefreshCmd(false)
		}
		return m, nil
	case "p":
		if sel := m.currentSession(); sel != nil && !m.prRefreshing && !m.prRefreshIsInFlight(sel.ID) {
			m.prRefreshing = true
			m = m.markPRRefreshInFlight(sel.ID)
			m.err = nil
			m.status = "refreshing PR"
			return m, m.prRefreshCmd(*sel, false)
		}
		return m, nil
	case "o":
		if sel := m.currentSession(); sel != nil && !m.openingPR {
			m.openingPR = true
			m.err = nil
			m.status = "opening PR"
			return m, m.openPRCmd(*sel)
		}
		return m, nil
	case "t":
		if sel := m.currentSession(); sel != nil && m.mode == modeDashboard {
			m.mode = modeTagPicker
			m.tagCursor = sessionTagOptionIndex(sel.Tag)
			m.err = nil
			m.status = "session status"
			return m.withAnimationCmd()
		}
		return m, nil
	case "e":
		if sel := m.currentSession(); sel != nil {
			m.mode = modeNoteEditor
			m.noteInput.SetValue(sel.Note)
			m.noteInput.SetWidth(max(12, min(m.width, 100)-4))
			m.noteInput.CursorEnd()
			m.err = nil
			m.status = "edit note"
			return m, m.noteInput.Focus()
		}
		return m, nil
	case "v":
		m.kanbanView = !m.kanbanView
		m.todoExpanded = false
		if m.svc != nil && m.svc.Store != nil {
			if err := m.svc.Store.SaveDashboardPreferences(DashboardPreferences{KanbanView: m.kanbanView, StatusSeenAt: m.statusSeenAt}); err != nil {
				m.err = err
				m.status = "view preference failed"
			}
		}
		return m.withAnimationCmd()
	case "space":
		if !m.kanbanView {
			if sel := m.currentSession(); sel != nil && sel.Todo != nil && len(sel.Todo.Tasks) > 0 {
				m.todoExpanded = !m.todoExpanded
				return m.withAnimationCmd()
			}
		}
		return m, nil
	case "h", "left":
		if m.kanbanView {
			m.selected = kanbanSelection(m.sessions, m.selected, -1, 0)
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		return m, nil
	case "l", "right":
		if m.kanbanView {
			m.selected = kanbanSelection(m.sessions, m.selected, 1, 0)
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		return m, nil
	case "j", "down":
		if len(m.sessions) == 0 {
			return m, nil
		}
		if m.kanbanView {
			m.selected = kanbanSelection(m.sessions, m.selected, 0, 1)
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		if m.selected < len(m.sessions)-1 {
			m.selected++
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		return m, nil
	case "k", "up":
		if len(m.sessions) == 0 {
			return m, nil
		}
		if m.kanbanView {
			m.selected = kanbanSelection(m.sessions, m.selected, 0, -1)
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		if m.selected > 0 {
			m.selected--
			return m.syncStatusSeenAt().withAnimationCmd()
		}
		return m, nil
	case "n":
		return m.beginCreate()
	case "enter":
		if sel := m.currentSession(); sel != nil && !m.attaching {
			m.attaching = true
			m.status = "switching"
			return m, m.attachCmd(*sel)
		}
		return m, nil
	case "x":
		if sel := m.currentSession(); sel != nil {
			m.mode = modeConfirmClose
			m.status = "confirm close with y"
			return m.withAnimationCmd()
		}
		return m, nil
	case "y":
		if m.mode == modeConfirmClose {
			if sel := m.currentSession(); sel != nil && !m.closing {
				m.closing = true
				m.mode = modeDashboard
				m.status = "closing"
				return m, m.closeCmd(*sel, m.closeFallbackTmuxSession())
			}
		}
	}
	return m, nil
}

func (m DashboardModel) updateTagPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeDashboard
		m.status = "cancelled"
		m.err = nil
		return m, nil
	case "j", "down":
		if m.tagCursor < len(sessionTagOptions)-1 {
			m.tagCursor++
			return m.withAnimationCmd()
		}
		return m, nil
	case "k", "up":
		if m.tagCursor > 0 {
			m.tagCursor--
			return m.withAnimationCmd()
		}
		return m, nil
	case "enter":
		if sel := m.currentSession(); sel != nil && !m.tagUpdating {
			m.tagUpdating = true
			m.err = nil
			tag := sessionTagOptions[m.tagCursor].Tag
			m.status = "updating status"
			return m, m.tagUpdateCmd(*sel, tag)
		}
		return m, nil
	}
	return m, nil
}

func (m DashboardModel) updateNoteEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeDashboard
		m.noteInput.Blur()
		m.status = "cancelled"
		m.err = nil
		return m, nil
	case "enter":
		if sel := m.currentSession(); sel != nil && !m.noteUpdating {
			m.noteUpdating = true
			m.noteInput.Blur()
			m.status = "saving note"
			return m, m.noteUpdateCmd(*sel, m.noteInput.Value())
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.noteInput, cmd = m.noteInput.Update(msg)
	m.err = nil
	return m, cmd
}

func (m DashboardModel) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.cancelCreate(), nil
	case "tab", "shift+tab":
		return m.tabCreateStage()
	case "ctrl+o":
		if m.form.stage == createStageBranch {
			m.form.toggleBranchMode()
			m.err = nil
			if m.form.branchMode == BranchModeOpenExisting && len(m.form.branchCandidates) == 0 {
				m.form.branchesLoading = true
				return m.withAnimationCmd(m.branchListCmd(m.form.selectedRootPath))
			}
			return m.withAnimationCmd()
		}
		return m, nil
	case "enter":
		switch m.form.stage {
		case createStagePicker:
			return m.enterCreateRoot()
		case createStageManual:
			return m.enterCreateRoot()
		case createStageBranch:
			root := m.form.selectedRootPath
			if root == "" {
				m.err = fmt.Errorf("root is required")
				return m, nil
			}
			if m.form.branchMode == BranchModeOpenExisting && m.form.branchesLoading {
				return m, nil
			}
			branch := m.form.selectedBranch()
			if m.form.branchMode == BranchModeOpenExisting && branch == "" {
				m.err = fmt.Errorf("select or enter a branch")
				return m, nil
			}
			m.creating = true
			m.mode = modeDashboard
			if m.form.branchMode == BranchModeOpenExisting {
				m.status = "opening branch"
			} else {
				m.status = "creating"
			}
			return m, m.createCmd(root, branch, m.form.branchMode)
		}
		return m, nil
	}

	switch m.form.stage {
	case createStagePicker:
		switch msg.String() {
		case "up", "down", "pgup", "pgdown":
			m.form.moveRootCursor(msg.String())
			return m.withAnimationCmd()
		}
		old := m.form.rootSearch.Value()
		var cmd tea.Cmd
		m.form.rootSearch, cmd = m.form.rootSearch.Update(msg)
		if m.form.rootSearch.Value() != old {
			m.err = nil
			m.form.applyFilter()
			return m.withAnimationCmd(cmd)
		}
		return m, cmd
	case createStageManual:
		var cmd tea.Cmd
		m.form.rootManual, cmd = m.form.rootManual.Update(msg)
		m.err = nil
		return m, cmd
	case createStageBranch:
		if m.form.branchMode == BranchModeOpenExisting && (msg.String() == "up" || msg.String() == "down") {
			m.form.moveBranchCursor(msg.String())
			return m.withAnimationCmd()
		}
		old := m.form.branch.Value()
		var cmd tea.Cmd
		m.form.branch, cmd = m.form.branch.Update(msg)
		if m.form.branchMode == BranchModeOpenExisting && m.form.branch.Value() != old {
			m.form.applyBranchFilter()
		}
		m.err = nil
		return m, cmd
	}
	return m, nil
}

func (m DashboardModel) View() tea.View {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	outerWidth := m.width
	if outerWidth <= 0 {
		outerWidth = 100
	}
	outerHeight := m.height
	if outerHeight <= 0 {
		outerHeight = 30
	}
	contentWidth := max(40, outerWidth-2)
	header := m.renderHeader(contentWidth)
	formView := ""
	if m.mode == modeCreate {
		formView = m.form.render()
	}
	tagView := ""
	if m.mode == modeTagPicker {
		tagView = m.renderTagPicker(contentWidth)
	}
	noteView := ""
	if m.mode == modeNoteEditor {
		noteView = m.renderNoteEditor(contentWidth)
	}
	status := m.renderStatusLine(contentWidth)
	prInspector := m.renderPRInspector(contentWidth)
	footer := m.renderFooter(contentWidth)
	confirmView := ""
	if m.mode == modeConfirmClose {
		confirmView = dim.Render("Press `y` to close the selected session or `esc` to cancel.")
	}
	reservedHeight := lipgloss.Height(header) + lipgloss.Height(footer)
	nonBodyParts := 2
	if formView != "" {
		reservedHeight += lipgloss.Height(formView)
		nonBodyParts++
	}
	if tagView != "" {
		reservedHeight += lipgloss.Height(tagView)
		nonBodyParts++
	}
	if noteView != "" {
		reservedHeight += lipgloss.Height(noteView)
		nonBodyParts++
	}
	if confirmView != "" {
		reservedHeight += lipgloss.Height(confirmView)
		nonBodyParts++
	}
	if status != "" {
		reservedHeight += lipgloss.Height(status)
		nonBodyParts++
	}
	if prInspector != "" {
		reservedHeight += lipgloss.Height(prInspector)
		nonBodyParts++
	}
	reservedHeight += nonBodyParts + 2
	bodyHeight := max(6, outerHeight-reservedHeight)
	body := m.renderBody(contentWidth, bodyHeight)
	var parts []string
	parts = append(parts, header)
	if formView != "" {
		parts = append(parts, formView)
	}
	if tagView != "" {
		parts = append(parts, tagView)
	}
	if noteView != "" {
		parts = append(parts, noteView)
	}
	parts = append(parts, body)
	if confirmView != "" {
		parts = append(parts, confirmView)
	}
	if status != "" {
		parts = append(parts, status)
	}
	if prInspector != "" {
		parts = append(parts, prInspector)
	}
	if footer != "" {
		parts = append(parts, footer)
	}
	v := tea.NewView(strings.Join(parts, "\n\n"))
	v.AltScreen = true
	return v
}

func (m DashboardModel) renderHeader(width int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	brand := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("oak-tree")
	tagline := dim.Render("tmux + coding-agent command deck")
	selected := "none selected"
	if session := m.currentSession(); session != nil {
		selected = sessionDisplayTitle(*session)
	}
	modeText := "DASHBOARD"
	modeColor := "80"
	if m.kanbanView {
		modeText = "KANBAN"
	}
	if m.mode == modeCreate {
		modeText = "CREATE"
		modeColor = "170"
	} else if m.mode == modeConfirmClose {
		modeText = "CONFIRM"
		modeColor = "203"
	} else if m.mode == modeTagPicker {
		modeText = "STATUS"
		modeColor = "214"
	} else if m.mode == modeNoteEditor {
		modeText = "NOTE"
		modeColor = "170"
	}
	row := lipgloss.JoinHorizontal(lipgloss.Center,
		brand,
		" ",
		tagline,
		"  ",
		renderTinyPill(modeText, modeColor),
	)
	stats := joinHeaderChips(width, []string{
		renderMetric("sessions", fmt.Sprintf("%d", len(m.sessions)), "81"),
		renderMetric("selected", truncateMiddle(selected, max(14, width/3)), "170"),
		renderMetric("state", truncateMiddle(m.status, max(8, width/4)), statusColor(m.status, m.err)),
	}, 1)
	rule := m.deckRail(width, "238")
	return strings.Join([]string{row, stats, rule}, "\n")
}

func (m DashboardModel) renderBody(width, height int) string {
	if m.kanbanView {
		return m.renderKanbanPanel(width, height)
	}
	return m.renderSessionsPanel(width, height)
}

func (m DashboardModel) renderTagPicker(width int) string {
	session := m.currentSession()
	if session == nil {
		return ""
	}
	panelWidth := max(32, min(width, 72))
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render("Session status")
	title += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(truncateMiddle(sessionDisplayTitle(*session), max(12, panelWidth-20)))
	lines := []string{title}
	for i, option := range sessionTagOptions {
		cursor := "  "
		if i == m.tagCursor {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("› ")
		}
		label := renderTinyPill(strings.ToUpper(option.Label), option.Color)
		if option.Tag == SessionTagNone {
			label = renderTinyPill("ACTIVE", option.Color)
		}
		lines = append(lines, cursor+label)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(0, 1).
		Width(panelWidth).
		Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) renderNoteEditor(width int) string {
	session := m.currentSession()
	if session == nil {
		return ""
	}
	panelWidth := max(32, min(width, 100))
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render("Session note")
	title += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(truncateMiddle(sessionDisplayTitle(*session), max(12, panelWidth-20)))
	m.noteInput.SetWidth(max(12, panelWidth-4))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(0, 1).
		Width(panelWidth).
		Render(title + "\n" + m.noteInput.View())
}

func (m DashboardModel) renderSessionsPanel(width, height int) string {
	panelWidth := max(24, width)
	innerWidth := max(20, panelWidth-4)
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1).Width(panelWidth).Height(max(5, height))
	counts := [5]int{}
	for _, session := range m.sessions {
		counts[sessionDashboardBucket(session)]++
	}
	parked := counts[3] + counts[4]
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render("◈ Sessions") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(fmt.Sprintf("%d active", len(m.sessions)-parked))
	if len(m.sessions) == 0 {
		return panelStyle.Render(title + "\n\n" + m.renderEmptySessions(innerWidth))
	}
	summary := joinStatusItems(innerWidth, []string{renderMetric("needs-you", fmt.Sprintf("%d", counts[0]), "81"), renderMetric("working", fmt.Sprintf("%d", counts[1]), "214"), renderMetric("ready", fmt.Sprintf("%d", counts[2]), "82"), renderMetric("review", fmt.Sprintf("%d", counts[3]), "170"), renderMetric("blocked", fmt.Sprintf("%d", counts[4]), "203")})
	head := m.renderSessionTableHeader(innerWidth)
	maxRows := max(1, height-3)
	if counts[3] > 0 {
		maxRows = max(1, maxRows-1)
	}
	if counts[4] > 0 {
		maxRows = max(1, maxRows-1)
	}
	todoRows := 0
	if selected := m.currentSession(); m.todoExpanded && selected != nil && selected.Todo != nil && len(selected.Todo.Tasks) > 0 && maxRows > 1 {
		todoRows = min(len(selected.Todo.Tasks), maxRows-1)
		maxRows -= todoRows
	}
	start := 0
	if m.selected >= maxRows {
		start = m.selected - maxRows + 1
	}
	if start > len(m.sessions)-maxRows {
		start = max(0, len(m.sessions)-maxRows)
	}
	end := min(len(m.sessions), start+maxRows)
	rows := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		if m.sessions[i].Tag == SessionTagWaitingReview && (i == start || m.sessions[i-1].Tag != SessionTagWaitingReview) {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true).Render(fmt.Sprintf("REVIEW · %d parked", counts[3])))
		}
		if m.sessions[i].Tag == SessionTagBlocked && (i == start || m.sessions[i-1].Tag != SessionTagBlocked) {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render(fmt.Sprintf("BLOCKED · %d parked", counts[4])))
		}
		rows = append(rows, m.renderSessionTableRow(m.sessions[i], i == m.selected, innerWidth))
		if i == m.selected && todoRows > 0 {
			rows = append(rows, renderTodoTaskRows(m.sessions[i].Todo, innerWidth, todoRows)...)
		}
	}
	return panelStyle.Render(strings.Join([]string{title, summary, head, strings.Join(rows, "\n")}, "\n"))
}

func (m DashboardModel) renderKanbanPanel(width, height int) string {
	panelWidth := max(24, width)
	innerWidth := max(20, panelWidth-4)
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1).Width(panelWidth).Height(max(5, height))
	counts := [5]int{}
	buckets := [5][]int{}
	for i, session := range m.sessions {
		bucket := sessionDashboardBucket(session)
		counts[bucket]++
		buckets[bucket] = append(buckets[bucket], i)
	}
	parked := counts[3] + counts[4]
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render("◈ Sessions") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(fmt.Sprintf("%d active", len(m.sessions)-parked))
	if len(m.sessions) == 0 {
		return panelStyle.Render(title + "\n\n" + m.renderEmptySessions(innerWidth))
	}
	summary := joinStatusItems(innerWidth, []string{renderMetric("needs-you", fmt.Sprintf("%d", counts[0]), "81"), renderMetric("working", fmt.Sprintf("%d", counts[1]), "214"), renderMetric("ready", fmt.Sprintf("%d", counts[2]), "82"), renderMetric("review", fmt.Sprintf("%d", counts[3]), "170"), renderMetric("blocked", fmt.Sprintf("%d", counts[4]), "203")})
	labels := [5]string{"QUESTION", "WORKING", "READY", "REVIEW", "BLOCKED"}
	colors := [5]string{"81", "214", "82", "170", "203"}
	columnWidth := max(6, (innerWidth-4)/5)
	columnHeight := max(1, height-4)
	columns := make([]string, 0, 9)
	for i := range labels {
		if i > 0 {
			columns = append(columns, " ")
		}
		columns = append(columns, m.renderKanbanColumn(labels[i], colors[i], buckets[i], columnWidth, columnHeight))
	}
	return panelStyle.Render(strings.Join([]string{title, summary, lipgloss.JoinHorizontal(lipgloss.Top, columns...)}, "\n"))
}

func (m DashboardModel) renderKanbanColumn(label, color string, sessionIndexes []int, width, height int) string {
	contentWidth := max(1, width-2)
	header := truncateMiddle(fmt.Sprintf("%s %d", label, len(sessionIndexes)), contentWidth)
	lines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(header)}
	if len(sessionIndexes) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("—"))
	} else if height >= 4 {
		maxCards := max(1, (height-1)/4)
		start := 0
		for i, sessionIndex := range sessionIndexes {
			if sessionIndex == m.selected && i >= maxCards {
				start = i - maxCards + 1
				break
			}
		}
		end := min(len(sessionIndexes), start+maxCards)
		for i, sessionIndex := range sessionIndexes[start:end] {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, m.renderKanbanCard(m.sessions[sessionIndex], sessionIndex == m.selected, contentWidth, color)...)
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color)).Width(contentWidth).Height(height).Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) renderKanbanCard(session Session, selected bool, width int, accent string) []string {
	foreground, background := tableRowColors(selected)
	prefix := "  "
	if selected {
		prefix = "▌ "
	}
	titleWidth := max(1, width-2)
	noteIcon := ""
	titleTextWidth := titleWidth
	if strings.TrimSpace(session.Note) != "" {
		noteIcon = "✎"
		titleTextWidth = max(1, titleWidth-2)
	}
	titleText := sessionProjectName(session)
	if m.hasUnseenStatus(session) {
		titleText = "● " + titleText
	}
	title := truncateMiddle(titleText, titleTextWidth)
	branch := truncateMiddle(strings.TrimSpace(session.Branch), titleWidth)
	if branch == "" {
		branch = "—"
	}
	meta := []string{}
	if age := sessionParkedAge(session, time.Now()); age != "" {
		meta = append(meta, "age "+age)
	}
	if session.GitStatus != nil {
		meta = append(meta, gitStatusChip(session.GitStatus))
	}
	if session.PR != nil && session.PR.Found {
		meta = append(meta, fmt.Sprintf("#%d", session.PR.Number))
	}
	if todo, _, _ := todoSummaryChip(session.Todo); todo != "—" {
		meta = append(meta, todo)
	}
	if usage, ok := m.usage.ForSessionIDs(sessionUsageIDs(session)); ok {
		meta = append(meta, formatUSD(usage.TotalCostUSD))
	}
	metaLine := joinStatusItems(titleWidth, meta)
	if metaLine == "" {
		metaLine = "—"
	}
	padding := strings.Repeat(" ", max(0, titleTextWidth-lipgloss.Width(title)))
	first := renderTableSegment(prefix, accent, background, selected)
	if noteIcon != "" {
		first += renderTableSegment(noteIcon, accent, background, true) + renderTableSegment(" ", foreground, background, selected)
	}
	first += renderTableSegment(title+padding, foreground, background, selected)
	return []string{
		first,
		renderTableSegment("  "+padTableCell(branch, titleWidth), "246", background, false),
		renderTableSegment("  "+padTableCell(metaLine, titleWidth), "246", background, false),
	}
}

func kanbanSelection(sessions []Session, selected, columnDelta, rowDelta int) int {
	if selected < 0 || selected >= len(sessions) {
		return selected
	}
	buckets := [5][]int{}
	for i, session := range sessions {
		bucket := sessionDashboardBucket(session)
		buckets[bucket] = append(buckets[bucket], i)
	}
	currentBucket := sessionDashboardBucket(sessions[selected])
	currentRow := 0
	for i, sessionIndex := range buckets[currentBucket] {
		if sessionIndex == selected {
			currentRow = i
			break
		}
	}
	if columnDelta != 0 {
		for bucket := currentBucket + columnDelta; bucket >= 0 && bucket < len(buckets); bucket += columnDelta {
			if len(buckets[bucket]) > 0 {
				return buckets[bucket][min(currentRow, len(buckets[bucket])-1)]
			}
		}
		return selected
	}
	targetRow := max(0, min(currentRow+rowDelta, len(buckets[currentBucket])-1))
	return buckets[currentBucket][targetRow]
}

func sessionDashboardBucket(session Session) int {
	if session.Tag == SessionTagWaitingReview {
		return 3
	}
	if session.Tag == SessionTagBlocked {
		return 4
	}
	switch session.AgentStatus {
	case AgentStatusQuestion:
		return 0
	case AgentStatusWorking:
		return 1
	default:
		return 2
	}
}

func sessionTableLine(state, session, branch, git, pr, todo, cost string, width int) string {
	if width < 70 {
		stateWidth := min(12, max(1, width/3))
		sessionWidth := max(1, width-stateWidth-2)
		return strings.Join([]string{padTableCell(state, stateWidth), truncateMiddle(session, sessionWidth)}, "  ")
	}
	stateWidth, sessionWidth, gitWidth, prWidth, costWidth := 12, 20, 10, 12, 8
	if width >= 110 {
		sessionWidth, gitWidth, prWidth, costWidth := 24, 12, 18, 10
		branchWidth := width - stateWidth - sessionWidth - gitWidth - prWidth - 10 - costWidth - 12
		return strings.Join([]string{padTableCell(state, stateWidth), padTableCell(session, sessionWidth), padTableCell(branch, branchWidth), padTableCell(git, gitWidth), padTableCell(pr, prWidth), padTableCell(todo, 10), padTableCell(cost, costWidth)}, "  ")
	}
	branchWidth := width - stateWidth - sessionWidth - gitWidth - prWidth - costWidth - 10
	cells := []string{padTableCell(state, stateWidth), padTableCell(session, sessionWidth)}
	if branchWidth >= 8 {
		cells = append(cells, padTableCell(branch, branchWidth))
	}
	cells = append(cells, padTableCell(git, gitWidth), padTableCell(pr, prWidth), padTableCell(cost, costWidth))
	return strings.Join(cells, "  ")
}

func padTableCell(value string, width int) string {
	value = truncateMiddle(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

func (m DashboardModel) renderSessionTableHeader(width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Bold(true).Render(sessionTableLine("STATE", "SESSION", "BRANCH", "GIT", "PR", "TODO", "COST", width))
}

func (m DashboardModel) renderSessionTableRow(session Session, selected bool, width int) string {
	state, stateColor, stateBackground := sessionTableStateChipAtFrame(session, m.motionFrame)
	git := "—"
	gitColor, gitBackground := "244", ""
	if session.GitStatus != nil {
		git = gitStatusChip(session.GitStatus)
		gitColor, gitBackground = gitStatusColor(session.GitStatus), tableChipBackground(gitStatusColor(session.GitStatus))
	}
	pr := "—"
	prColor, prBackground := "244", ""
	if session.PR != nil && session.PR.Found {
		pr = " #" + fmt.Sprintf("%d", session.PR.Number) + " " + strings.ToLower(session.PR.State)
		prColor = prStateColor(session.PR.State)
		prBackground = tableChipBackground(prColor)
	}
	todo, todoColor, todoBackground := todoSummaryChip(session.Todo)
	cost := "—"
	costColor := "244"
	if usage, ok := m.usage.ForSessionIDs(sessionUsageIDs(session)); ok {
		cost = formatUSD(usage.TotalCostUSD)
		costColor = "81"
	}
	rowForeground, rowBackground := tableRowColors(selected)
	sessionTitle := sessionProjectName(session)
	if m.hasUnseenStatus(session) {
		sessionTitle = "● " + sessionTitle
	}
	line := sessionTableChipLine(
		tableChipCell(state, stateColor, stateBackground),
		tablePlainCell(sessionTitle, rowForeground),
		tablePlainCell(strings.TrimSpace(session.Branch), rowForeground),
		tableChipCell(git, gitColor, gitBackground),
		tableChipCell(pr, prColor, prBackground),
		tableChipCell(todo, todoColor, todoBackground),
		tablePlainCell(cost, costColor),
		max(1, width-2), rowBackground, selected,
	)
	return renderTableSegment("▌ ", rowForeground, rowBackground, selected) + line
}

func renderTodoTaskRows(todo *TodoSummary, width, limit int) []string {
	if todo == nil || limit <= 0 {
		return nil
	}
	tasks := todo.Tasks
	shown := min(len(tasks), limit)
	if shown < len(tasks) {
		shown = max(0, shown-1)
	}
	rows := make([]string, 0, limit)
	for _, task := range tasks[:shown] {
		icon, color := "○", "246"
		switch task.Status {
		case "in_progress":
			icon, color = "◐", "214"
		case "completed":
			icon, color = "✓", "82"
		}
		prefix := renderTableSegment("│ "+strings.Repeat(" ", 14), "229", "#2d2f38", false)
		marker := renderTableSegment(icon+" ", color, "#2d2f38", true)
		subjectWidth := max(1, width-18)
		subject := truncateMiddle(task.Subject, subjectWidth)
		rows = append(rows, prefix+marker+renderTableSegment(subject+strings.Repeat(" ", max(0, subjectWidth-lipgloss.Width(subject))), "252", "#2d2f38", false))
	}
	if shown < len(tasks) {
		label := fmt.Sprintf("… %d more", len(tasks)-shown)
		rows = append(rows, renderTableSegment("│ "+strings.Repeat(" ", 16)+padTableCell(label, max(1, width-18)), "246", "#2d2f38", false))
	}
	return rows
}

func sessionTableStateChip(session Session) (string, string, string) {
	return sessionTableStateChipAtFrame(session, 0)
}

func sessionTableStateChipAtFrame(session Session, frame int) (string, string, string) {
	switch sessionDashboardBucket(session) {
	case 0:
		return " QUESTION", "81", "#263d4a"
	case 1:
		return workingStateChip(frame), "214", "#4a3818"
	case 2:
		return " READY", "82", "#244b2b"
	case 3:
		return " REVIEW", "170", "#4b2847"
	default:
		return " BLOCKED", "203", "#4a2525"
	}
}

func todoSummaryChip(todo *TodoSummary) (string, string, string) {
	if todo == nil || todo.Total == 0 {
		return "—", "244", ""
	}
	progress := fmt.Sprintf("%d/%d", todo.Completed, todo.Total)
	switch {
	case todo.InProgress > 0:
		return "◐ " + progress, "214", tableChipBackground("214")
	case todo.Completed == todo.Total:
		return "✓ " + progress, "82", tableChipBackground("82")
	default:
		return "○ " + progress, "246", tableChipBackground("246")
	}
}

func workingStateChip(frame int) string {
	const spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	frames := []rune(spinner)
	return string(frames[(frame/3)%len(frames)]) + " WORKING"
}

func gitStatusChip(status *GitStatus) string {
	if status == nil || strings.TrimSpace(status.Error) != "" {
		return "? git"
	}
	switch {
	case !status.Clean && status.Ahead > 0:
		return " changes ↑"
	case !status.Clean:
		return " changes"
	case status.Ahead > 0:
		return " unpushed ↑"
	default:
		return " clean"
	}
}

func tableChipBackground(color string) string {
	switch color {
	case "81":
		return "#263d4a"
	case "82":
		return "#244232"
	case "170":
		return "#4b2847"
	case "214":
		return "#4c3d20"
	default:
		return "#343640"
	}
}

func tablePlainCell(value, color string) tableCell {
	return tableCell{text: value, fg: color}
}

func tableChipCell(value, color, background string) tableCell {
	return tableCell{text: value, fg: color, bg: background, chip: background != ""}
}

type tableCell struct {
	text string
	fg   string
	bg   string
	chip bool
}

func sessionTableChipLine(state, session, branch, git, pr, todo, cost tableCell, width int, rowBackground string, selected bool) string {
	separator := renderTableSegment("  ", "", rowBackground, selected)
	if width < 70 {
		stateWidth := min(12, max(1, width/3))
		sessionWidth := max(1, width-stateWidth-2)
		return renderTableCell(state, stateWidth, rowBackground, selected) + separator + renderTableCell(session, sessionWidth, rowBackground, selected)
	}
	stateWidth, sessionWidth, gitWidth, prWidth, costWidth := 12, 20, 10, 12, 8
	if width >= 110 {
		sessionWidth, gitWidth, prWidth, costWidth := 24, 12, 18, 10
		branchWidth := width - stateWidth - sessionWidth - gitWidth - prWidth - 10 - costWidth - 12
		return strings.Join([]string{renderTableCell(state, stateWidth, rowBackground, selected), renderTableCell(session, sessionWidth, rowBackground, selected), renderTableCell(branch, branchWidth, rowBackground, selected), renderTableCell(git, gitWidth, rowBackground, selected), renderTableCell(pr, prWidth, rowBackground, selected), renderTableCell(todo, 10, rowBackground, selected), renderTableCell(cost, costWidth, rowBackground, selected)}, separator)
	}
	branchWidth := width - stateWidth - sessionWidth - gitWidth - prWidth - costWidth - 10
	cells := []string{renderTableCell(state, stateWidth, rowBackground, selected), renderTableCell(session, sessionWidth, rowBackground, selected)}
	if branchWidth >= 8 {
		cells = append(cells, renderTableCell(branch, branchWidth, rowBackground, selected))
	}
	cells = append(cells, renderTableCell(git, gitWidth, rowBackground, selected), renderTableCell(pr, prWidth, rowBackground, selected), renderTableCell(cost, costWidth, rowBackground, selected))
	return strings.Join(cells, separator)
}

func renderTableCell(cell tableCell, width int, rowBackground string, selected bool) string {
	text := truncateMiddle(cell.text, width)
	fg := cell.fg
	if selected && !cell.chip {
		fg = "229"
	}
	if cell.chip {
		chip := lipgloss.NewStyle().Foreground(lipgloss.Color(cell.fg)).Background(lipgloss.Color(cell.bg)).Bold(true).Render(text)
		return chip + renderTableSegment(strings.Repeat(" ", max(0, width-lipgloss.Width(text))), "", rowBackground, false)
	}
	return renderTableSegment(text+strings.Repeat(" ", max(0, width-lipgloss.Width(text))), fg, rowBackground, selected)
}

func tableRowColors(selected bool) (foreground, background string) {
	if selected {
		return "229", "#36362f"
	}
	return "252", ""
}

func renderTableSegment(text, foreground, background string, bold bool) string {
	style := lipgloss.NewStyle().Bold(bold)
	if foreground != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if background != "" {
		style = style.Background(lipgloss.Color(background))
	}
	return style.Render(text)
}

func (m DashboardModel) renderEmptySessions(width int) string {
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Render("No active sessions")
	copy := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Press " + renderKeycap("n") + " to launch a tmux coding workspace.")
	config := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(fmt.Sprintf("%d roots indexed from config", len(m.rootCandidates)))
	if width < 32 {
		copy = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(renderKeycap("n") + " new session")
	}
	return strings.Join([]string{head, copy, config}, "\n")
}

func (m DashboardModel) wantsAnimationTick() bool {
	return true
}

func (m DashboardModel) withAnimationCmd(cmds ...tea.Cmd) (DashboardModel, tea.Cmd) {
	cmds = compactCommands(cmds...)
	if m.wantsAnimationTick() && !m.motionTickerActive {
		m.motionTickerActive = true
		cmds = append(cmds, m.animationTickCmd())
	}
	return m, batchCommands(cmds...)
}

func compactCommands(cmds ...tea.Cmd) []tea.Cmd {
	filtered := cmds[:0]
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

func batchCommands(cmds ...tea.Cmd) tea.Cmd {
	cmds = compactCommands(cmds...)
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (m DashboardModel) deckRail(width int, baseColor string) string {
	if width <= 0 {
		return ""
	}
	tail := 10
	cycle := max(1, width+tail)
	head := m.motionFrame % cycle
	baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(baseColor))
	tailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("60"))
	wakeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	headStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	var b strings.Builder
	for i := 0; i < width; i++ {
		distance := head - i
		if distance < 0 {
			distance += cycle
		}
		ch := "─"
		switch {
		case distance == 0:
			ch = "━"
			b.WriteString(headStyle.Render(ch))
		case distance > 0 && distance <= max(2, tail/3):
			ch = "━"
			b.WriteString(wakeStyle.Render(ch))
		case distance > 0 && distance <= tail:
			b.WriteString(tailStyle.Render(ch))
		default:
			b.WriteString(baseStyle.Render(ch))
		}
	}
	return b.String()
}

func (m DashboardModel) renderPRInspector(width int) string {
	if m.mode != modeDashboard {
		return ""
	}
	session := m.currentSession()
	if session == nil || session.PR == nil || !session.PR.Found {
		return ""
	}
	info := session.PR
	badge := renderTinyPill(fmt.Sprintf("PR #%d", info.Number), "81")
	commandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	commands := renderKeycap("o") + " " + commandStyle.Render("open") + "  " + renderKeycap("p") + " " + commandStyle.Render("refresh")
	maxLeftWidth := max(1, width-lipgloss.Width(commands)-2)
	left := badge
	remaining := maxLeftWidth - lipgloss.Width(left)
	age := formatRelativeAge(info.RefreshedAt, time.Now())
	ageText := ""
	if age != "" {
		candidate := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("refreshed " + age)
		if remaining-lipgloss.Width(candidate)-2 >= 12 {
			ageText = candidate
			remaining -= lipgloss.Width(candidate) + 2
		}
	}
	if title := truncateMiddle(info.Title, max(0, remaining-2)); title != "" {
		left += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Render(title)
	}
	if ageText != "" {
		left += "  " + ageText
	}
	gap := max(2, width-lipgloss.Width(left)-lipgloss.Width(commands))
	titleLine := left + strings.Repeat(" ", gap) + commands

	readiness, readinessColor := prReadinessChip(info)
	review, reviewColor := prReviewChip(info.ReviewDecision)
	comments, commentsColor := prCommentsChip(info.UnresolvedComments)
	checksState := strings.ToLower(strings.TrimSpace(info.ChecksState))
	if checksState == "" {
		checksState = "unknown"
	}
	statusLine := joinStatusItems(width, []string{
		renderTinyPill(readiness, readinessColor),
		renderTinyPill(prChecksIcon(checksState)+" CI "+strings.ToUpper(checksState), prChecksColor(checksState)),
		renderTinyPill(review, reviewColor),
		renderTinyPill(comments, commentsColor),
	})
	return m.deckRail(width, "60") + "\n" + titleLine + "\n" + statusLine
}

func formatRelativeAge(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	age := now.Sub(at)
	if age < time.Minute {
		return "now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age/time.Minute))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(age/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(age/(24*time.Hour)))
}

func sessionParkedAge(session Session, now time.Time) string {
	if session.Tag == SessionTagNone {
		return ""
	}
	at := session.CreatedAt
	if session.TagUpdatedAt != nil {
		at = *session.TagUpdatedAt
	}
	return strings.TrimSuffix(formatRelativeAge(at, now), " ago")
}

func prReadinessChip(info *PRInfo) (string, string) {
	if info.IsDraft {
		return "DRAFT", "214"
	}
	switch strings.ToLower(strings.TrimSpace(info.State)) {
	case "merged":
		return "MERGED", "170"
	case "closed":
		return "CLOSED", "244"
	default:
		return "READY", "82"
	}
}

func prReviewChip(decision string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approved":
		return "✓ APPROVED", "82"
	case "changes_requested":
		return "✕ CHANGES REQUESTED", "203"
	case "review_required":
		return "◌ REVIEW REQUIRED", "214"
	default:
		return "· REVIEW UNKNOWN", "246"
	}
}

func prCommentsChip(unresolved *int) (string, string) {
	if unresolved == nil {
		return "COMMENTS UNAVAILABLE", "246"
	}
	if *unresolved == 0 {
		return "✓ COMMENTS RESOLVED", "82"
	}
	return fmt.Sprintf("● %d UNRESOLVED", *unresolved), "214"
}

func (m DashboardModel) renderFooter(width int) string {
	bindings := m.footerBindings()
	label := renderTinyPill("KEYS", "81")
	parts := []string{label}
	used := lipgloss.Width(label)
	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == "" || help.Desc == "" {
			continue
		}
		item := renderKeycap(help.Key) + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(help.Desc)
		nextWidth := lipgloss.Width(item)
		if used+2+nextWidth > width {
			break
		}
		parts = append(parts, item)
		used += 2 + nextWidth
	}
	line := strings.Join(parts, "  ")
	rule := m.deckRail(width, "60")
	return rule + "\n" + line
}

func (m DashboardModel) renderStatusLine(width int) string {
	if m.err == nil {
		return ""
	}
	errText := truncateMiddle(m.err.Error(), max(18, width))
	line := lipgloss.NewStyle().
		Foreground(lipgloss.Color("203")).
		Bold(true).
		Render("✕ " + errText)
	return m.deckRail(width, "238") + "\n" + line
}

func gitStatusLabel(status *GitStatus) string {
	if status == nil {
		return ""
	}
	if strings.TrimSpace(status.Error) != "" {
		return "git?"
	}
	hasChanges := !status.Clean
	hasUnpushed := status.Ahead > 0
	switch {
	case hasChanges && hasUnpushed:
		return "changes ↑"
	case hasChanges:
		return "changes"
	case hasUnpushed:
		return "unpushed ↑"
	default:
		return "clean"
	}
}

func gitStatusColor(status *GitStatus) string {
	if status == nil {
		return "244"
	}
	if strings.TrimSpace(status.Error) != "" {
		return "244"
	}
	if !status.Clean {
		return "214"
	}
	if status.Ahead > 0 {
		return "81"
	}
	if status.Behind > 0 {
		return "110"
	}
	if status.Clean {
		return "82"
	}
	return "246"
}

func sessionUsageIDs(session Session) []string {
	return session.AgentSessionIDs
}

func joinStatusItems(width int, items []string) string {
	if width <= 0 {
		return ""
	}
	var parts []string
	used := 0
	for _, item := range items {
		if strings.TrimSpace(lipgloss.NewStyle().Render(item)) == "" {
			continue
		}
		nextWidth := lipgloss.Width(item)
		gap := 0
		if len(parts) > 0 {
			gap = 2
		}
		if used+gap+nextWidth > width {
			break
		}
		parts = append(parts, item)
		used += gap + nextWidth
	}
	return strings.Join(parts, "  ")
}

func (m DashboardModel) footerBindings() []key.Binding {
	switch m.mode {
	case modeCreate:
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
			key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "mode")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	case modeConfirmClose:
		return []key.Binding{
			key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "close")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	case modeTagPicker:
		return []key.Binding{
			key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/k", "move")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	case modeNoteEditor:
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	default:
		move := key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "move"))
		if m.kanbanView {
			move = key.NewBinding(key.WithKeys("h", "j", "k", "l", "up", "down", "left", "right"), key.WithHelp("hjkl", "move"))
		}
		bindings := []key.Binding{
			move,
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "attach")),
		}
		if !m.kanbanView {
			if selected := m.currentSession(); selected != nil && selected.Todo != nil && len(selected.Todo.Tasks) > 0 {
				bindings = append(bindings, key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "todo")))
			}
		}
		return append(bindings,
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "status")),
			key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "note")),
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close")),
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		)
	}
}

func sessionProjectName(session Session) string {
	root := strings.TrimSpace(session.Root)
	if root == "" {
		root = strings.TrimSpace(session.Workdir)
	}
	project := filepath.Base(filepath.Clean(root))
	if project == "." || project == string(filepath.Separator) || project == "" {
		return compactPath(root, 32)
	}
	return project
}

func sessionDisplayTitle(session Session) string {
	project := sessionProjectName(session)
	branch := strings.TrimSpace(session.Branch)
	if branch == "" {
		return project
	}
	return project + " · " + branch
}

func prStatusMessage(info *PRInfo) string {
	if info == nil {
		return "PR status unknown"
	}
	if !info.Found {
		return "no PR found"
	}
	return fmt.Sprintf("PR #%d %s %s", info.Number, info.ChecksState, info.ReviewDecision)
}

func prChecksIcon(state string) string {
	switch state {
	case "pass":
		return "✓"
	case "fail":
		return "✕"
	case "pending":
		return "◌"
	default:
		return "·"
	}
}

func prChecksColor(state string) string {
	switch state {
	case "pass":
		return "82"
	case "fail":
		return "203"
	case "pending":
		return "214"
	default:
		return "246"
	}
}

func prStateColor(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		return "81"
	case "merged":
		return "170"
	case "closed":
		return "244"
	default:
		return "246"
	}
}

func renderMetric(label, value, color string) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
	return labelStyle.Render(label+" ") + valueStyle.Render(value)
}

func renderTinyPill(label, color string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("232")).
		Background(lipgloss.Color(color)).
		Padding(0, 1).
		Bold(true).
		Render(label)
}

func renderKeycap(label string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("232")).
		Background(lipgloss.Color("81")).
		Padding(0, 1).
		Bold(true).
		Render(label)
}

func statusColor(status string, err error) string {
	if err != nil {
		return "203"
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch {
	case strings.Contains(status, "fail"):
		return "203"
	case strings.Contains(status, "creating"), strings.Contains(status, "refreshing"), strings.Contains(status, "opening"), strings.Contains(status, "switching"), strings.Contains(status, "closing"):
		return "214"
	case status == "ready", status == "":
		return "82"
	default:
		return "81"
	}
}

func truncateLines(text string, width int) string {
	if width <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = truncateMiddle(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func compactPath(path string, width int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "-"
	}
	if width <= 0 {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanHome := filepath.Clean(home)
		cleanPath := filepath.Clean(path)
		if cleanPath == cleanHome {
			path = "~"
		} else if strings.HasPrefix(cleanPath, cleanHome+string(filepath.Separator)) {
			path = "~" + strings.TrimPrefix(cleanPath, cleanHome)
		} else {
			path = cleanPath
		}
	} else {
		path = filepath.Clean(path)
	}
	return truncateMiddle(path, width)
}

func truncateMiddle(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	if width == 2 {
		return string(runes[:1]) + "…"
	}
	left := (width - 1) / 2
	right := width - 1 - left
	if right < 1 {
		right = 1
		left = width - right - 1
	}
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func renderChip(label, fg, bg string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Padding(0, 1).
		Bold(true).
		Render(label)
}

func joinHeaderChips(width int, chips []string, gap int) string {
	if gap < 0 {
		gap = 0
	}
	spacer := strings.Repeat(" ", gap)
	used := 0
	rendered := make([]string, 0, len(chips))
	for _, chip := range chips {
		if chip == "" {
			continue
		}
		next := chip
		if len(rendered) > 0 {
			next = spacer + next
		}
		if used+lipgloss.Width(next) > width {
			break
		}
		rendered = append(rendered, chip)
		used += lipgloss.Width(next)
	}
	return strings.Join(rendered, spacer)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
