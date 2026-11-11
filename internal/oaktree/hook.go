package oaktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
)

const (
	maxCampfireMessages = 200
	campfireMinInterval = 45 * time.Second
)

type AgentEvent struct {
	OakSessionID    string       `json:"oak_session_id"`
	AgentRole       string       `json:"agent_role,omitempty"`
	TmuxPaneID      string       `json:"tmux_pane_id,omitempty"`
	Event           string       `json:"event"`
	Cwd             string       `json:"cwd,omitempty"`
	SessionID       string       `json:"session_id,omitempty"`
	SessionFile     string       `json:"session_file,omitempty"`
	SessionName     string       `json:"session_name,omitempty"`
	Todo            *TodoSummary `json:"todo,omitempty"`
	ActivityKind    string       `json:"activity_kind,omitempty"`
	ActivityMessage string       `json:"activity_message,omitempty"`
}

func ParseAgentEvent(r io.Reader) (AgentEvent, error) {
	var event AgentEvent
	if err := json.NewDecoder(r).Decode(&event); err != nil {
		return event, err
	}
	if strings.TrimSpace(event.Event) == "" {
		return event, errors.New("agent event missing event")
	}
	return event, nil
}

func (s *Service) ResolveOakSessionID(ctx context.Context, paneID string) (string, error) {
	if strings.TrimSpace(paneID) == "" {
		return "", errors.New("agent event missing oak_session_id and tmux_pane_id")
	}
	output, err := s.Exec.Output(ctx, "tmux", PaneSessionIdCommandArgs(paneID)...)
	if err != nil {
		return "", fmt.Errorf("resolve tmux session for pane %s: %w", paneID, err)
	}
	tmuxSessionID := strings.TrimSpace(string(output))
	if tmuxSessionID == "" {
		return "", errors.New("tmux did not return a session id")
	}
	sessions, err := s.Store.ListSessions()
	if err != nil {
		return "", err
	}
	for _, session := range sessions {
		if session.TmuxSessionID == tmuxSessionID {
			return session.ID, nil
		}
	}
	return "", errors.New("current tmux session is not managed by oak-tree")
}

func (s *Service) HandleAgentEvent(ctx context.Context, event AgentEvent) error {
	if strings.TrimSpace(event.OakSessionID) == "" {
		var err error
		event.OakSessionID, err = s.ResolveOakSessionID(ctx, event.TmuxPaneID)
		if err != nil {
			return err
		}
	}
	session, err := s.Store.FindSessionByID(event.OakSessionID)
	if err != nil {
		return err
	}
	if session.RightPaneID == "" {
		return errors.New("oak-tree session is still starting")
	}
	if event.Event == "session_name_auto" || event.Event == "session_name_manual" {
		if len(session.Agents) > 0 && event.AgentRole != "architect" {
			return errors.New("session naming requires the architect agent")
		}
		name := strings.TrimSpace(strings.Join(strings.FieldsFunc(event.SessionName, unicode.IsControl), " "))
		if len([]rune(name)) > 80 {
			name = string([]rune(name)[:80])
		}
		if name == "oak-"+session.ID+"-architect" || name == "oak-"+session.ID+"-junior" {
			return nil
		}
		return s.Store.UpdateSession(session.ID, func(stored *Session) error {
			if stored.Name != "" && stored.DisplayName() == "" {
				stored.Name, stored.NameManual = "", false
			}
			if event.Event == "session_name_manual" {
				stored.Name, stored.NameManual = name, true
			} else if stored.Name == "" && !stored.NameManual && name != "" {
				stored.Name = name
			}
			return nil
		})
	}
	if event.AgentRole != "" {
		return s.handlePairedAgentEvent(ctx, session, event)
	}
	if len(session.Agents) > 0 {
		return errors.New("paired agent event missing role")
	}
	if event.Event == "todo" {
		if err := validateTodoSummary(event.Todo); err != nil {
			return err
		}
	}
	if event.Event == "campfire" {
		if err := validateCampfireMessage(event.ActivityKind, event.ActivityMessage); err != nil {
			return err
		}
		event.ActivityMessage = strings.TrimSpace(event.ActivityMessage)
	}
	shouldNotifyQuestion := event.Event == "question" && session.AgentStatus != AgentStatusQuestion
	shouldNotifySettled := event.Event == "agent_settled" && session.AgentStatus == AgentStatusWorking
	piPaneID := session.RightPaneID
	if event.TmuxPaneID != "" {
		piPaneID = event.TmuxPaneID
	}
	shouldPublishPanes := event.Event == "session_start" || piPaneID != session.RightPaneID
	now := time.Now().UTC()
	err = s.Store.UpdateSession(session.ID, func(stored *Session) error {
		if event.TmuxPaneID != "" {
			stored.RightPaneID = event.TmuxPaneID
		}
		if event.SessionID != "" {
			stored.AgentSessionIDs = dedupeStrings(append(stored.AgentSessionIDs, event.SessionID))
		}
		if event.SessionFile != "" {
			stored.AgentSessionFile = event.SessionFile
		}
		if event.Cwd != "" {
			stored.LastHookCwd = event.Cwd
		}
		stored.LastHookAt = &now
		switch event.Event {
		case "session_start", "session_shutdown":
			if event.Event == "session_shutdown" {
				stored.AgentStatus = AgentStatusIdle
			}
		case "agent_start":
			stored.AgentStatus = AgentStatusWorking
			unparkSession(stored)
		case "question_answered":
			stored.AgentStatus = AgentStatusWorking
		case "question":
			stored.AgentStatus = AgentStatusQuestion
		case "agent_settled":
			stored.AgentStatus = AgentStatusAttention
		case "todo":
			summary := *event.Todo
			stored.Todo = &summary
			return nil
		case "campfire":
			return nil
		default:
			return fmt.Errorf("unknown agent event %q", event.Event)
		}
		stored.AgentStatusUpdatedAt = &now
		return nil
	})
	if err != nil {
		return err
	}
	if shouldPublishPanes {
		// Best effort: the runner records failures in command diagnostics.
		_ = SetOakSessionOptions(ctx, s.Exec, session.TmuxSessionID, session.ID, session.LeftPaneID, piPaneID)
	}
	if event.Event == "campfire" {
		return s.appendCampfire(*session, event.AgentRole, event.ActivityKind, event.ActivityMessage)
	}
	if shouldNotifyQuestion {
		_ = s.Exec.Run(ctx, "notify-send", "Pi question waiting", agentQuestionNotificationBody(*session))
	}
	if shouldNotifySettled {
		_ = s.Exec.Run(ctx, "notify-send", "Pi finished working", agentQuestionNotificationBody(*session))
	}
	return nil
}

func (s *Service) handlePairedAgentEvent(ctx context.Context, session *Session, event AgentEvent) error {
	if event.AgentRole != "architect" && event.AgentRole != "junior" {
		return errors.New("unknown paired agent role")
	}
	if event.Event == "todo" {
		if err := validateTodoSummary(event.Todo); err != nil {
			return err
		}
	}
	if event.Event == "campfire" {
		if err := validateCampfireMessage(event.ActivityKind, event.ActivityMessage); err != nil {
			return err
		}
		event.ActivityMessage = strings.TrimSpace(event.ActivityMessage)
	}
	now := time.Now().UTC()
	var previous, current AgentStatus
	var selectedPane string
	err := s.Store.UpdateSession(session.ID, func(stored *Session) error {
		agent, ok := stored.Agents[event.AgentRole]
		if !ok {
			return errors.New("paired agent is not registered")
		}
		previous = stored.AgentStatus
		if event.Event == "pane_selected" {
			if event.TmuxPaneID == "" || event.TmuxPaneID != agent.PaneID {
				return errors.New("selected pane does not match paired agent")
			}
			stored.RightPaneID = event.TmuxPaneID
			stored.AgentSessionFile = agent.SessionFile
			selectedPane = event.TmuxPaneID
			current = stored.AgentStatus
			return nil
		}
		if event.TmuxPaneID != "" {
			agent.PaneID = event.TmuxPaneID
		}
		if event.SessionID != "" {
			agent.SessionID = event.SessionID
			stored.AgentSessionIDs = dedupeStrings(append(stored.AgentSessionIDs, event.SessionID))
		}
		if event.SessionFile != "" {
			agent.SessionFile = event.SessionFile
			if agent.PaneID == stored.RightPaneID {
				stored.AgentSessionFile = event.SessionFile
			}
		}
		stored.LastHookAt = &now
		if event.Cwd != "" {
			stored.LastHookCwd = event.Cwd
		}
		switch event.Event {
		case "session_start":
		case "session_shutdown":
			agent.Status = AgentStatusIdle
		case "agent_start", "question_answered":
			agent.Status = AgentStatusWorking
			if event.Event == "agent_start" {
				unparkSession(stored)
			}
		case "question":
			agent.Status = AgentStatusQuestion
		case "agent_settled":
			agent.Status = AgentStatusAttention
		case "todo":
			agent.Todo = event.Todo
		case "campfire":
		default:
			return fmt.Errorf("unknown agent event %q", event.Event)
		}
		stored.Agents[event.AgentRole] = agent
		stored.AgentStatus = pairedAgentStatus(stored.Agents)
		stored.Todo = pairedAgentTodos(stored.Agents)
		if event.Event != "todo" && event.Event != "campfire" {
			stored.AgentStatusUpdatedAt = &now
		}
		current = stored.AgentStatus
		return nil
	})
	if err != nil {
		return err
	}
	if selectedPane != "" {
		_ = SetOakSessionOptions(ctx, s.Exec, session.TmuxSessionID, session.ID, session.LeftPaneID, selectedPane)
	}
	if event.Event == "campfire" {
		return s.appendCampfire(*session, event.AgentRole, event.ActivityKind, event.ActivityMessage)
	}
	if current == AgentStatusQuestion && previous != AgentStatusQuestion {
		_ = s.Exec.Run(ctx, "notify-send", "Pi question waiting", agentQuestionNotificationBody(*session))
	} else if current == AgentStatusAttention && previous == AgentStatusWorking {
		_ = s.Exec.Run(ctx, "notify-send", "Pi finished working", agentQuestionNotificationBody(*session))
	}
	return nil
}

func pairedAgentStatus(agents map[string]PairedAgent) AgentStatus {
	for _, status := range []AgentStatus{AgentStatusQuestion, AgentStatusWorking, AgentStatusAttention, AgentStatusIdle} {
		for _, agent := range agents {
			if agent.Status == status {
				return status
			}
		}
	}
	return AgentStatusUnknown
}

func pairedAgentTodos(agents map[string]PairedAgent) *TodoSummary {
	var combined *TodoSummary
	for _, role := range []string{"architect", "junior"} {
		if todo := agents[role].Todo; todo != nil {
			if combined == nil {
				combined = &TodoSummary{}
			}
			combined.Total += todo.Total
			combined.Pending += todo.Pending
			combined.InProgress += todo.InProgress
			combined.Completed += todo.Completed
			combined.Tasks = append(combined.Tasks, todo.Tasks...)
		}
	}
	return combined
}

func unparkSession(session *Session) {
	if session.Tag != SessionTagWaitingReview && session.Tag != SessionTagBlocked {
		return
	}
	session.Tag = SessionTagNone
	session.TagUpdatedAt = nil
}

func (s *Service) appendCampfire(session Session, role, kind, message string) error {
	return s.Store.UpdateCampfire(func(messages *[]CampfireMessage) error {
		now := time.Now().UTC()
		for i := len(*messages) - 1; i >= 0; i-- {
			last := (*messages)[i]
			if last.SourceSessionID != session.ID {
				continue
			}
			if last.Kind == kind && last.Message == message {
				return nil
			}
			if now.Sub(last.At) < campfireMinInterval && !campfireRateLimitExempt(kind) {
				return nil
			}
			break
		}
		*messages = append(*messages, CampfireMessage{At: now, Kind: kind, Message: message, SourceSessionID: session.ID, AgentRole: role, Project: sessionProjectName(session)})
		return nil
	})
}

func validateCampfireMessage(kind, message string) error {
	switch kind {
	case "plan", "discovery", "snag", "recovery", "tests", "milestone", "handoff", "aside":
	default:
		return fmt.Errorf("invalid Campfire kind %q", kind)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("invalid Campfire message")
	}
	for _, char := range message {
		if unicode.IsControl(char) {
			return errors.New("invalid Campfire message")
		}
	}
	if len([]rune(message)) > 140 {
		return errors.New("Campfire message exceeds 140 characters")
	}
	return nil
}

func campfireRateLimitExempt(kind string) bool {
	return kind == "snag" || kind == "milestone" || kind == "handoff"
}

func validateTodoSummary(summary *TodoSummary) error {
	if summary == nil {
		return errors.New("todo event missing summary")
	}
	if summary.Total < 0 || summary.Pending < 0 || summary.InProgress < 0 || summary.Completed < 0 || summary.Total != summary.Pending+summary.InProgress+summary.Completed {
		return errors.New("invalid todo summary")
	}
	if summary.Tasks == nil {
		return nil
	}
	counts := map[string]int{}
	for _, task := range summary.Tasks {
		if strings.TrimSpace(task.Subject) == "" || strings.ContainsAny(task.Subject, "\r\n") {
			return errors.New("invalid todo task subject")
		}
		switch task.Status {
		case "pending", "in_progress", "completed":
			counts[task.Status]++
		default:
			return fmt.Errorf("invalid todo task status %q", task.Status)
		}
	}
	if len(summary.Tasks) != summary.Total || counts["pending"] != summary.Pending || counts["in_progress"] != summary.InProgress || counts["completed"] != summary.Completed {
		return errors.New("todo tasks do not match summary")
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
