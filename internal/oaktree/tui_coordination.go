package oaktree

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type coordinationUpdateMsg struct {
	group CoordinationGroup
	err   error
}

type coordinationDialog struct {
	groupID      string
	organizerID  string
	cursor       int
	detailOffset int
	editingID    string
	name         string
	chosen       map[string]bool
	input        textinput.Model
	busy         bool
}

func (m DashboardModel) coordinationGroup(id string) *CoordinationGroup {
	for _, group := range m.coordinationGroups {
		if group.ID == id && group.ClosedAt == nil {
			return &group
		}
	}
	return nil
}

func (m DashboardModel) sessionByID(id string) *Session {
	for _, session := range m.sessions {
		if session.ID == id {
			return &session
		}
	}
	return nil
}

func (m DashboardModel) groupForSession(id string) *CoordinationGroup {
	for _, group := range m.coordinationGroups {
		if group.ClosedAt == nil && group.Contains(id) && m.sessionByID(group.OrganizerID) != nil {
			return &group
		}
	}
	return nil
}

func (m DashboardModel) foldedParticipant(session Session) *CoordinationGroup {
	group := m.groupForSession(session.ID)
	if group != nil && group.OrganizerID != session.ID && !m.expandedGroups[group.ID] {
		return group
	}
	return nil
}

func (m DashboardModel) coordinatedQuestion(session Session) bool {
	group := m.groupForSession(session.ID)
	return group != nil && group.OrganizerID != session.ID && session.AgentStatus == AgentStatusQuestion
}

func (m DashboardModel) boardProjection() ([]Session, []int) {
	var sessions []Session
	var indexes []int
	for i, session := range m.sessions {
		if group := m.groupForSession(session.ID); group != nil && group.OrganizerID != session.ID {
			if session.AgentStatus == AgentStatusQuestion {
				// Questions keep the same column and counts when the group expands.
				session.Tag = SessionTagNone
			} else if !m.expandedGroups[group.ID] && session.Tag != SessionTagTesting {
				continue
			}
		}
		sessions = append(sessions, session)
		indexes = append(indexes, i)
	}
	return sessions, indexes
}

func (m DashboardModel) normalizeCoordinationSelection() DashboardModel {
	if !m.kanbanView {
		return m
	}
	if selected := m.currentSession(); selected != nil {
		if group := m.foldedParticipant(*selected); group != nil && selected.AgentStatus != AgentStatusQuestion && selected.Tag != SessionTagTesting {
			m = m.withSelected(group.OrganizerID)
		}
	}
	return m
}

func (m DashboardModel) coordinationSelection(columnDelta, rowDelta int) int {
	sessions, indexes := m.boardProjection()
	for i, index := range indexes {
		if index == m.selected {
			return indexes[kanbanSelection(sessions, i, columnDelta, rowDelta)]
		}
	}
	return m.selected
}

func (m DashboardModel) toggleCoordinationExpansion() DashboardModel {
	selected := m.currentSession()
	if selected == nil {
		return m
	}
	if group := m.groupForSession(selected.ID); group != nil {
		if m.expandedGroups == nil {
			m.expandedGroups = make(map[string]bool)
		}
		m.expandedGroups[group.ID] = !m.expandedGroups[group.ID]
	}
	return m.normalizeCoordinationSelection()
}

func (m DashboardModel) coordinationUpdateCmd(request CoordinationRequest) tea.Cmd {
	return func() tea.Msg {
		group, err := m.svc.ApplyCoordination(request)
		return coordinationUpdateMsg{group: group, err: err}
	}
}

func (m DashboardModel) applyCoordinationUpdate(request CoordinationRequest) (tea.Model, tea.Cmd) {
	if m.groupDialog.busy {
		return m, nil
	}
	m.groupDialog.busy = true
	m.err = nil
	m.status = "saving coordination"
	return m, m.coordinationUpdateCmd(request)
}

func (m DashboardModel) withCoordinationGroups(groups []CoordinationGroup) DashboardModel {
	// A periodic snapshot may have started before a dashboard mutation completed.
	merged := append([]CoordinationGroup(nil), groups...)
	for _, current := range m.coordinationGroups {
		found := false
		for i := range merged {
			if merged[i].ID == current.ID {
				if current.UpdatedAt.After(merged[i].UpdatedAt) {
					merged[i] = current
				}
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, current)
		}
	}
	m.coordinationGroups = merged
	return m.normalizeCoordinationSelection()
}

func (m DashboardModel) beginCoordination() (tea.Model, tea.Cmd) {
	selected := m.currentSession()
	if selected == nil {
		return m, nil
	}
	m.groupDialog = coordinationDialog{organizerID: selected.ID, chosen: make(map[string]bool)}
	m.err = nil
	if group := m.groupForSession(selected.ID); group != nil {
		m.groupDialog.groupID = group.ID
		if selected.ID != group.OrganizerID {
			m.groupDialog.cursor = group.assignmentIndex(selected.ID) + 1
		}
		m.mode = modeCoordination
		return m, nil
	}
	return m.coordinationInput(modeCoordinationName, "group> ", "", 80)
}

func (m DashboardModel) coordinationInput(next mode, prompt, value string, limit int) (tea.Model, tea.Cmd) {
	m.mode = next
	if group := m.coordinationGroup(m.groupDialog.groupID); group != nil {
		m.groupDialog.editingID = m.dialogMember(*group)
	}
	m.groupDialog.input = newCreateInput(prompt, "")
	m.groupDialog.input.CharLimit = limit
	m.groupDialog.input.SetWidth(max(10, min(m.width-10, 90)-len(prompt)))
	m.groupDialog.input.SetValue(value)
	m.groupDialog.input.CursorEnd()
	return m, m.groupDialog.input.Focus()
}

func (m DashboardModel) coordinationCandidates() []Session {
	var candidates []Session
	for _, session := range m.sessions {
		if m.groupForSession(session.ID) != nil || (m.groupDialog.groupID == "" && session.ID == m.groupDialog.organizerID) {
			continue
		}
		candidates = append(candidates, session)
	}
	return candidates
}

func (m DashboardModel) dialogMembers(group CoordinationGroup) []string {
	ids := []string{group.OrganizerID}
	for _, assignment := range group.Assignments {
		ids = append(ids, assignment.SessionID)
	}
	return ids
}

func (m DashboardModel) dialogMember(group CoordinationGroup) string {
	ids := m.dialogMembers(group)
	return ids[max(0, min(m.groupDialog.cursor, len(ids)-1))]
}

func (m DashboardModel) updateCoordination(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.groupDialog.busy {
		return m, nil
	}
	if msg.String() == "esc" {
		if m.mode == modeCoordination || m.groupDialog.groupID == "" {
			m.mode = modeDashboard
		} else {
			m.mode = modeCoordination
		}
		m.err = nil
		m.groupDialog.input.Blur()
		return m.normalizeCoordinationSelection(), nil
	}
	group := m.coordinationGroup(m.groupDialog.groupID)
	if m.groupDialog.groupID != "" && group == nil {
		m.mode = modeDashboard
		m.status = "group closed"
		return m, nil
	}
	if m.mode == modeCoordinationName || m.mode == modeCoordinationTask || m.mode == modeCoordinationResult {
		if msg.String() != "enter" {
			var cmd tea.Cmd
			m.groupDialog.input, cmd = m.groupDialog.input.Update(msg)
			return m, cmd
		}
		value := strings.TrimSpace(m.groupDialog.input.Value())
		if value == "" {
			m.err = fmt.Errorf("enter a value")
			return m, nil
		}
		if m.mode == modeCoordinationName {
			if group != nil {
				return m.applyCoordinationUpdate(CoordinationRequest{Action: "update", GroupID: group.ID, Name: value})
			}
			m.groupDialog.name = value
			m.groupDialog.cursor = 0
			m.mode = modeCoordinationParticipants
			m.groupDialog.input.Blur()
			return m, nil
		}
		request := CoordinationRequest{GroupID: group.ID, SessionID: m.groupDialog.editingID}
		if m.mode == modeCoordinationTask {
			request.Action, request.Task = "assign", value
		} else {
			request.Action, request.Status, request.Result = "report", AssignmentReturned, value
		}
		return m.applyCoordinationUpdate(request)
	}
	if m.mode == modeCoordinationClose {
		if msg.String() == "y" {
			return m.applyCoordinationUpdate(CoordinationRequest{Action: "close", GroupID: group.ID})
		}
		return m, nil
	}
	if m.mode == modeCoordinationParticipants {
		candidates := m.coordinationCandidates()
		m.groupDialog.cursor = max(0, min(m.groupDialog.cursor, len(candidates)-1))
		switch msg.String() {
		case "down", "j":
			m.groupDialog.cursor = min(m.groupDialog.cursor+1, max(0, len(candidates)-1))
		case "up", "k":
			m.groupDialog.cursor = max(0, m.groupDialog.cursor-1)
		case "space":
			if len(candidates) > 0 && group == nil {
				id := candidates[m.groupDialog.cursor].ID
				m.groupDialog.chosen[id] = !m.groupDialog.chosen[id]
			}
		case "enter":
			if group != nil {
				if len(candidates) > 0 {
					return m.applyCoordinationUpdate(CoordinationRequest{Action: "assign", GroupID: group.ID, SessionID: candidates[m.groupDialog.cursor].ID, Task: group.Name})
				}
				return m, nil
			}
			selected := m.sessionByID(m.groupDialog.organizerID)
			if selected == nil {
				m.err = fmt.Errorf("organizing session is no longer available")
				return m, nil
			}
			request := CoordinationRequest{Action: "create", Name: m.groupDialog.name, OrganizerID: selected.ID}
			for _, candidate := range candidates {
				if m.groupDialog.chosen[candidate.ID] {
					request.Assignments = append(request.Assignments, CoordinationAssignment{SessionID: candidate.ID, Task: request.Name})
				}
			}
			return m.applyCoordinationUpdate(request)
		}
		return m, nil
	}
	if group == nil {
		return m, nil
	}
	m.groupDialog.cursor = max(0, min(m.groupDialog.cursor, len(group.Assignments)))
	member := m.dialogMember(*group)
	request := CoordinationRequest{GroupID: group.ID, SessionID: member}
	switch msg.String() {
	case "j", "down":
		m.groupDialog.cursor = min(m.groupDialog.cursor+1, len(group.Assignments))
		m.groupDialog.detailOffset = 0
	case "k", "up":
		m.groupDialog.cursor = max(0, m.groupDialog.cursor-1)
		m.groupDialog.detailOffset = 0
	case "pgdown":
		details := m.coordinationDetailLines(*group, max(16, min(m.width-8, 100)))
		m.groupDialog.detailOffset = min(max(0, len(details)-1), m.groupDialog.detailOffset+3)
	case "pgup":
		m.groupDialog.detailOffset = max(0, m.groupDialog.detailOffset-3)
	case "enter":
		if session := m.sessionByID(member); session != nil && !m.attaching {
			m.attaching = true
			m.status = "switching"
			return m, m.attachCmd(*session)
		}
	case "tab":
		m.groupDialog.cursor = 0
		m.groupDialog.detailOffset = 0
	case "n":
		return m.coordinationInput(modeCoordinationName, "name> ", group.Name, 80)
	case "+", "=":
		m.groupDialog.cursor = 0
		m.mode = modeCoordinationParticipants
	case "o":
		if member != group.OrganizerID {
			request.Action, request.OrganizerID = "update", member
			return m.applyCoordinationUpdate(request)
		}
	case "-":
		if member != group.OrganizerID {
			request.Action = "remove"
			return m.applyCoordinationUpdate(request)
		}
	case "e":
		if index := group.assignmentIndex(member); index >= 0 {
			return m.coordinationInput(modeCoordinationTask, "task> ", group.Assignments[index].Task, 500)
		}
	case "r":
		if index := group.assignmentIndex(member); index >= 0 {
			return m.coordinationInput(modeCoordinationResult, "result> ", group.Assignments[index].Result, 1000)
		}
	case "w":
		if member != group.OrganizerID {
			request.Action, request.Status = "report", AssignmentWorking
			return m.applyCoordinationUpdate(request)
		}
	case "a":
		request.Action = "accept"
		return m.applyCoordinationUpdate(request)
	case "c":
		m.mode = modeCoordinationClose
	}
	return m, nil
}

func (m DashboardModel) renderCoordinationView(width, height int) tea.View {
	innerWidth := max(16, min(width-8, 100))
	lines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true).Render("Coordination group")}
	group := m.coordinationGroup(m.groupDialog.groupID)
	if group != nil {
		lines = append(lines, truncateMiddle(group.Name, innerWidth), m.coordinationProgress(*group))
	} else if organizer := m.sessionByID(m.groupDialog.organizerID); organizer != nil {
		lines = append(lines, "Organizer: "+truncateMiddle(sessionDisplayTitle(*organizer), innerWidth-11))
	}
	lines = append(lines, "")
	switch m.mode {
	case modeCoordinationName, modeCoordinationTask, modeCoordinationResult:
		if m.mode == modeCoordinationName {
			lines = append(lines, "Name the shared change.")
		} else if m.mode == modeCoordinationTask {
			lines = append(lines, "Replace assignment (resets its progress and result).")
		} else {
			lines = append(lines, "Return a result for organizer acceptance.")
		}
		lines = append(lines, m.groupDialog.input.View(), "", "enter save · esc back")
	case modeCoordinationParticipants:
		lines = append(lines, "Select participants. Existing groups are excluded.")
		candidates := m.coordinationCandidates()
		if len(candidates) == 0 {
			lines = append(lines, "No available sessions.")
		}
		visible := max(1, height-17)
		start := max(0, m.groupDialog.cursor-visible+1)
		for i := start; i < min(len(candidates), start+visible); i++ {
			mark := "[ ] "
			if m.groupDialog.chosen[candidates[i].ID] {
				mark = "[x] "
			}
			line := mark + coordinationSessionLabel(candidates[i])
			lines = append(lines, renderCoordinationRow(line, i == m.groupDialog.cursor, innerWidth))
		}
		lines = append(lines, "", "j/k move · space select · enter save · esc back")
	case modeCoordinationClose:
		lines = append(lines, "Close this group and unfold its participants?", "Sessions and branches remain untouched.", "", "y close group · esc cancel")
	default:
		if group != nil {
			ids := m.dialogMembers(*group)
			visible := max(1, min(5, (height-20)/2))
			start := max(0, m.groupDialog.cursor-visible+1)
			for i := start; i < min(len(ids), start+visible); i++ {
				session := m.sessionByID(ids[i])
				label, state := ids[i]+" (session closed)", "unavailable"
				if session != nil {
					label = coordinationSessionLabel(*session)
					state, _, _ = agentStateChipAtFrame(session.AgentStatus, m.motionFrame)
				}
				progress := "ORGANIZER"
				if index := group.assignmentIndex(ids[i]); index >= 0 {
					progress = string(group.Assignments[index].Status)
				}
				lines = append(lines, renderCoordinationRow(label, i == m.groupDialog.cursor, innerWidth), "  "+state+" · "+progress)
			}
			details := m.coordinationDetailLines(*group, innerWidth)
			if len(details) > 0 {
				available := max(1, height-len(lines)-12)
				offset := min(m.groupDialog.detailOffset, max(0, len(details)-1))
				end := min(len(details), offset+available)
				lines = append(lines, "")
				lines = append(lines, details[offset:end]...)
				if offset > 0 || end < len(details) {
					lines = append(lines, fmt.Sprintf("pgup/pgdown detail · lines %d–%d/%d", offset+1, end, len(details)))
				}
			}
			lines = append(lines, "", "j/k move · enter attach · tab organizer · esc board", "+ add · - remove · o swap organizer · n rename", "e assignment · w working · r return · a accept · c close")
		}
	}
	if m.groupDialog.busy {
		lines = append(lines, "Saving…")
	}
	if m.err != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(truncateMiddle(m.err.Error(), innerWidth)))
	}
	for i := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(innerWidth).Render(lines[i])
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("221")).Padding(1, 2).Width(innerWidth).Render(strings.Join(lines, "\n"))
	view := tea.NewView(lipgloss.Place(max(1, width), max(1, height), lipgloss.Center, lipgloss.Center, panel))
	view.AltScreen = true
	return view
}

func (m DashboardModel) coordinationDetailLines(group CoordinationGroup, width int) []string {
	index := group.assignmentIndex(m.dialogMember(group))
	if index < 0 {
		return nil
	}
	assignment := group.Assignments[index]
	text := "Task: " + assignment.Task
	if assignment.Result != "" {
		text += "\nResult: " + assignment.Result
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n")
}

func renderCoordinationRow(label string, selected bool, width int) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	prefix := "  "
	if selected {
		style = style.Background(lipgloss.Color("237")).Bold(true)
		prefix = "▌ "
	}
	return style.Render(prefix + truncateMiddle(label, max(1, width-2)))
}

func coordinationSessionLabel(session Session) string {
	label := sessionDisplayTitle(session)
	if session.PR != nil && session.PR.Found {
		label = fmt.Sprintf("#%d · %s", session.PR.Number, label)
	}
	return label
}

func (m DashboardModel) coordinationProgress(group CoordinationGroup) string {
	returned, accepted, questions := 0, 0, 0
	for _, assignment := range group.Assignments {
		if assignment.Status == AssignmentReturned || assignment.Status == AssignmentAccepted {
			returned++
		}
		if assignment.Status == AssignmentAccepted {
			accepted++
		}
		if session := m.sessionByID(assignment.SessionID); session != nil && session.AgentStatus == AgentStatusQuestion {
			questions++
		}
	}
	return fmt.Sprintf("%d/%d returned · %d accepted · %d question", returned, len(group.Assignments), accepted, questions)
}

func (m DashboardModel) renderKanbanItem(index int, width int, accent string) []string {
	session := m.sessions[index]
	selected := index == m.selected
	if group := m.foldedParticipant(session); group != nil && session.AgentStatus == AgentStatusQuestion {
		identity := session.Branch
		if session.PR != nil && session.PR.Found {
			identity = fmt.Sprintf("#%d · %s", session.PR.Number, identity)
		}
		return []string{
			renderCoordinationRow(group.Name, selected, width),
			lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(truncateMiddle("  ↳ needs you", width)),
			truncateMiddle("  "+identity, width),
		}
	}
	lines := m.renderKanbanCard(session, selected, width, accent)
	if group := m.groupForSession(session.ID); group != nil && group.OrganizerID == session.ID {
		lines = append(m.renderCoordinationHeading(*group, session, selected, width), lines[1:]...)
		role := fmt.Sprintf("Organizer · %d participants", len(group.Assignments))
		if lipgloss.Width(role) > width-2 {
			role = fmt.Sprintf("Organizer · %d", len(group.Assignments))
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
		lines = append(lines, style.Render(ansi.Truncate("  "+role, width, "…")))
		questions, returned := 0, 0
		for _, assignment := range group.Assignments {
			if member := m.sessionByID(assignment.SessionID); member != nil && member.AgentStatus == AgentStatusQuestion {
				questions++
			}
			if assignment.Status == AssignmentReturned || assignment.Status == AssignmentAccepted {
				returned++
			}
		}
		lines = append(lines, style.Render(ansi.Truncate(fmt.Sprintf("  %d/%d returned", returned, len(group.Assignments)), width, "…")))
		if questions > 0 {
			questionLabel := "question"
			if questions > 1 {
				questionLabel = "questions"
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(ansi.Truncate(fmt.Sprintf("  %d %s", questions, questionLabel), width, "…")))
		}
	}
	return lines
}

func (m DashboardModel) renderCoordinationHeading(group CoordinationGroup, session Session, selected bool, width int) []string {
	marker := "▸ "
	if m.expandedGroups[group.ID] {
		marker = "▾ "
	}
	name := group.Name
	if strings.TrimSpace(session.Note) != "" {
		name = "✎ " + name
	}
	if m.hasUnseenStatus(session) {
		name = "● " + name
	}
	textWidth := max(1, width-4)
	wrapped := strings.Split(ansi.Wrap(name, textWidth, " _/"), "\n")
	if len(wrapped) > 2 {
		wrapped = []string{wrapped[0], ansi.Truncate(strings.Join(wrapped[1:], " "), textWidth, "…")}
	}
	lines := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		prefix := "  "
		if i == 0 {
			prefix = marker
		}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(renderCoordinationRow(prefix+line, selected, width)))
	}
	return lines
}
