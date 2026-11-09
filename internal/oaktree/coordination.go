package oaktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type AssignmentStatus string

const (
	AssignmentPending  AssignmentStatus = "pending"
	AssignmentWorking  AssignmentStatus = "working"
	AssignmentReturned AssignmentStatus = "returned"
	AssignmentAccepted AssignmentStatus = "accepted"
)

type CoordinationAssignment struct {
	SessionID string           `json:"session_id"`
	Task      string           `json:"task"`
	Status    AssignmentStatus `json:"status"`
	Result    string           `json:"result,omitempty"`
}

type CoordinationGroup struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	OrganizerID string                   `json:"organizer_id"`
	Assignments []CoordinationAssignment `json:"assignments"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	ClosedAt    *time.Time               `json:"closed_at,omitempty"`
}

func (g CoordinationGroup) Contains(sessionID string) bool {
	if g.OrganizerID == sessionID {
		return true
	}
	return g.assignmentIndex(sessionID) >= 0
}

func (g CoordinationGroup) assignmentIndex(sessionID string) int {
	for i := range g.Assignments {
		if g.Assignments[i].SessionID == sessionID {
			return i
		}
	}
	return -1
}

func (s *Store) LoadCoordinationGroups() ([]CoordinationGroup, error) {
	contents, err := readPrivateFile(filepath.Join(s.StateDir, "coordination.json"))
	if os.IsNotExist(err) {
		return []CoordinationGroup{}, nil
	}
	if err != nil {
		return nil, err
	}
	var groups []CoordinationGroup
	if err := json.Unmarshal(contents, &groups); err != nil {
		return nil, fmt.Errorf("read coordination groups: %w", err)
	}
	return groups, nil
}

func (s *Store) updateCoordinationGroups(update func(*[]CoordinationGroup) error) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	return withFileLock(filepath.Join(s.StateDir, ".coordination.lock"), func() error {
		groups, err := s.LoadCoordinationGroups()
		if err != nil {
			return err
		}
		if err := update(&groups); err != nil {
			return err
		}
		contents, err := json.MarshalIndent(groups, "", "  ")
		if err != nil {
			return err
		}
		return atomicWrite(filepath.Join(s.StateDir, "coordination.json"), contents, 0o600)
	})
}

// CoordinationRequest is shared by the dashboard, CLI, and managed Pi tool.
// An empty ActorID denotes a user action; agents supply their resolved oak identity.
type CoordinationRequest struct {
	Action      string                   `json:"action"`
	ActorID     string                   `json:"-"`
	GroupID     string                   `json:"group_id,omitempty"`
	Name        string                   `json:"name,omitempty"`
	OrganizerID string                   `json:"organizer_id,omitempty"`
	SessionID   string                   `json:"session_id,omitempty"`
	Assignments []CoordinationAssignment `json:"assignments,omitempty"`
	Task        string                   `json:"task,omitempty"`
	Status      AssignmentStatus         `json:"status,omitempty"`
	Result      string                   `json:"result,omitempty"`
}

func coordinationText(value, label string, limit int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if utf8.RuneCountInString(value) > limit || strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("%s must be a single line of at most %d characters", label, limit)
	}
	return value, nil
}

func (s *Service) ApplyCoordination(request CoordinationRequest) (CoordinationGroup, error) {
	var updated CoordinationGroup
	err := s.Store.updateCoordinationGroups(func(groups *[]CoordinationGroup) error {
		now := time.Now().UTC()
		if request.Action == "create" {
			id, err := newSessionID()
			if err != nil {
				return err
			}
			organizer := request.OrganizerID
			if organizer == "" {
				organizer = request.ActorID
			}
			if request.ActorID != "" && request.ActorID != organizer {
				return fmt.Errorf("create the group from its organizing session")
			}
			updated = CoordinationGroup{ID: id, Name: request.Name, OrganizerID: organizer, CreatedAt: now, UpdatedAt: now}
			for _, assignment := range request.Assignments {
				updated.Assignments = append(updated.Assignments, CoordinationAssignment{SessionID: assignment.SessionID, Task: assignment.Task, Status: AssignmentPending})
			}
			if err := s.validateCoordinationGroup(updated, *groups); err != nil {
				return err
			}
			updated.Name = strings.TrimSpace(updated.Name)
			for i := range updated.Assignments {
				updated.Assignments[i].Task = strings.TrimSpace(updated.Assignments[i].Task)
			}
			*groups = append(*groups, updated)
			return nil
		}
		index := -1
		for i := range *groups {
			if (*groups)[i].ID == request.GroupID && (*groups)[i].ClosedAt == nil {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("active coordination group not found")
		}
		updated = (*groups)[index]
		updated.Assignments = append([]CoordinationAssignment(nil), updated.Assignments...)
		if request.ActorID != "" && request.ActorID != updated.OrganizerID {
			if request.Action != "report" || request.SessionID != request.ActorID {
				return fmt.Errorf("only the organizer can manage this group")
			}
		}
		assignmentIndex := updated.assignmentIndex(request.SessionID)
		switch request.Action {
		case "update":
			if request.Name != "" {
				updated.Name = strings.TrimSpace(request.Name)
			}
			if request.OrganizerID != "" && request.OrganizerID != updated.OrganizerID {
				newOrganizerIndex := updated.assignmentIndex(request.OrganizerID)
				if newOrganizerIndex < 0 {
					return fmt.Errorf("the new organizer must be a participant")
				}
				updated.Assignments[newOrganizerIndex] = CoordinationAssignment{SessionID: updated.OrganizerID, Task: updated.Name, Status: AssignmentPending}
				updated.OrganizerID = request.OrganizerID
			}
		case "assign":
			if assignmentIndex < 0 {
				updated.Assignments = append(updated.Assignments, CoordinationAssignment{SessionID: request.SessionID, Task: strings.TrimSpace(request.Task), Status: AssignmentPending})
			} else {
				updated.Assignments[assignmentIndex].Task = strings.TrimSpace(request.Task)
				updated.Assignments[assignmentIndex].Status = AssignmentPending
				updated.Assignments[assignmentIndex].Result = ""
			}
		case "report":
			if assignmentIndex < 0 {
				return fmt.Errorf("participant not found")
			}
			if request.Status != AssignmentPending && request.Status != AssignmentWorking && request.Status != AssignmentReturned {
				return fmt.Errorf("report status must be pending, working, or returned")
			}
			result, err := coordinationText(request.Result, "result", 1000, request.Status == AssignmentReturned)
			if err != nil {
				return err
			}
			updated.Assignments[assignmentIndex].Status = request.Status
			updated.Assignments[assignmentIndex].Result = result
		case "accept":
			if assignmentIndex < 0 || updated.Assignments[assignmentIndex].Status != AssignmentReturned {
				return fmt.Errorf("only a returned assignment can be accepted")
			}
			updated.Assignments[assignmentIndex].Status = AssignmentAccepted
		case "remove":
			if assignmentIndex < 0 {
				return fmt.Errorf("participant not found")
			}
			updated.Assignments = append(updated.Assignments[:assignmentIndex], updated.Assignments[assignmentIndex+1:]...)
			if len(updated.Assignments) == 0 {
				updated.ClosedAt = &now
			}
		case "close":
			updated.ClosedAt = &now
		default:
			return fmt.Errorf("unknown coordination action")
		}
		updated.UpdatedAt = now
		if updated.ClosedAt == nil {
			if err := s.validateCoordinationGroup(updated, *groups); err != nil {
				return err
			}
		}
		(*groups)[index] = updated
		return nil
	})
	return updated, err
}

func (s *Service) validateCoordinationGroup(group CoordinationGroup, groups []CoordinationGroup) error {
	if _, err := coordinationText(group.Name, "group name", 80, true); err != nil {
		return err
	}
	if len(group.Assignments) == 0 {
		return fmt.Errorf("select at least one participant")
	}
	seen := map[string]bool{group.OrganizerID: true}
	for _, assignment := range group.Assignments {
		if seen[assignment.SessionID] {
			return fmt.Errorf("each session can have only one role in a group")
		}
		seen[assignment.SessionID] = true
		if _, err := coordinationText(assignment.Task, "assignment", 500, true); err != nil {
			return err
		}
	}
	for id := range seen {
		if id == "" || SafeComponent(id) != id {
			return fmt.Errorf("invalid session id")
		}
		if _, err := s.Store.LoadSession(id); err != nil {
			return fmt.Errorf("session is no longer available")
		}
		for _, existing := range groups {
			if existing.ID != group.ID && existing.ClosedAt == nil && existing.Contains(id) {
				return fmt.Errorf("a session already belongs to another active group")
			}
		}
	}
	return nil
}

func (s *Store) removeSessionFromCoordination(id string) error {
	return s.updateCoordinationGroups(func(groups *[]CoordinationGroup) error {
		now := time.Now().UTC()
		for i := range *groups {
			group := &(*groups)[i]
			if group.ClosedAt != nil || !group.Contains(id) {
				continue
			}
			if group.OrganizerID == id {
				group.ClosedAt = &now
			} else if index := group.assignmentIndex(id); index >= 0 {
				group.Assignments = append(group.Assignments[:index], group.Assignments[index+1:]...)
				if len(group.Assignments) == 0 {
					group.ClosedAt = &now
				}
			}
			group.UpdatedAt = now
		}
		return nil
	})
}
