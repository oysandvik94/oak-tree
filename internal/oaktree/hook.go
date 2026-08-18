package oaktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type AgentEvent struct {
	OakSessionID string       `json:"oak_session_id"`
	Event        string       `json:"event"`
	Cwd          string       `json:"cwd,omitempty"`
	SessionID    string       `json:"session_id,omitempty"`
	SessionFile  string       `json:"session_file,omitempty"`
	Todo         *TodoSummary `json:"todo,omitempty"`
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

func (s *Service) HandleAgentEvent(ctx context.Context, event AgentEvent) error {
	if strings.TrimSpace(event.OakSessionID) == "" {
		return errors.New("agent event missing oak_session_id")
	}
	session, err := s.Store.FindSessionByID(event.OakSessionID)
	if err != nil {
		return err
	}
	if session.RightPaneID == "" {
		return errors.New("oak-tree session is still starting")
	}
	if event.Event == "todo" {
		if err := validateTodoSummary(event.Todo); err != nil {
			return err
		}
	}
	shouldNotifyQuestion := event.Event == "question" && session.AgentStatus != AgentStatusQuestion
	shouldNotifySettled := event.Event == "agent_settled" && session.AgentStatus == AgentStatusWorking
	now := time.Now().UTC()
	err = s.Store.UpdateSession(session.ID, func(stored *Session) error {
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
		case "agent_start", "question_answered":
			stored.AgentStatus = AgentStatusWorking
		case "question":
			stored.AgentStatus = AgentStatusQuestion
		case "agent_settled":
			stored.AgentStatus = AgentStatusAttention
		case "todo":
			summary := *event.Todo
			stored.Todo = &summary
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
	if shouldNotifyQuestion {
		_ = s.Exec.Run(ctx, "notify-send", "Pi question waiting", agentQuestionNotificationBody(*session))
	}
	if shouldNotifySettled {
		_ = s.Exec.Run(ctx, "notify-send", "Pi finished working", agentQuestionNotificationBody(*session))
	}
	return nil
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
