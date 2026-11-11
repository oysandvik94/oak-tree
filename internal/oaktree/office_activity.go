package oaktree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxOfficeActivityEntries = 200
	maxOfficeActivitySeen    = 2000
)

type officeActivityEntry struct {
	AgentID    string `json:"agentId"`
	Author     string `json:"author"`
	Source     string `json:"source"`
	Text       string `json:"text"`
	Time       string `json:"time"`
	QuestionID string `json:"questionId,omitempty"`
	Key        string `json:"key"`
}

type officeActivityHistory struct {
	Entries []officeActivityEntry `json:"entries"`
	Seen    []string              `json:"seen"`
}

func officeActivityPath(store *Store) string {
	return filepath.Join(store.StateDir, "office-activity.json")
}

func readOfficeActivityHistory(store *Store) (officeActivityHistory, error) {
	var history officeActivityHistory
	data, err := readPrivateFile(officeActivityPath(store))
	if errors.Is(err, os.ErrNotExist) {
		return history, nil
	}
	if err != nil {
		return history, err
	}
	if err := json.Unmarshal(data, &history); err != nil {
		return history, err
	}
	return history, nil
}

func writeOfficeActivityHistory(store *Store, history officeActivityHistory) error {
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	return atomicWrite(officeActivityPath(store), data, 0o600)
}

func officeActivityForAgent(agent officeAgentState, at time.Time) officeActivityEntry {
	author := agent.Name
	if author == "" {
		author = agent.Project
	}
	role := agent.Role
	if role == "" {
		role = "Agent"
	}
	author += " · " + role
	entry := officeActivityEntry{AgentID: agent.ID, Author: author, Time: at.UTC().Format(time.RFC3339Nano)}
	labels := map[string]string{"working": "Working", "question": "Question", "ready": "Ready", "review": "Review", "blocked": "Blocked"}
	status := labels[agent.SessionState]
	if len(agent.Questions) > 0 {
		question := agent.Questions[0]
		entry.Source = "Question"
		entry.QuestionID = question.ID
		var prompts []struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(question.Questions, &prompts) == nil && len(prompts) > 0 {
			entry.Text = prompts[0].Prompt
		}
		if entry.Text == "" {
			entry.Text = question.Title
		}
		if entry.Text == "" {
			entry.Text = "Your answer is needed"
		}
	} else {
		entry.Source = map[string]string{"task": "CURRENT TASK", "session": "SESSION UPDATE", "status": "SESSION STATUS"}[agent.ActivitySource]
		if entry.Source == "" {
			entry.Source = "Activity"
		}
		entry.Text = agent.Activity
		if agent.ActivitySource == "status" || entry.Text == "" {
			entry.Text = status
		}
	}
	entry.Key = officeActivityKey(entry.AgentID, entry.Source, entry.QuestionID, entry.Text)
	if text := []rune(entry.Text); len(text) > 4000 {
		entry.Text = string(text[:4000]) + "…"
	}
	return entry
}

func officeActivityKey(parts ...string) string {
	key, _ := json.Marshal(parts)
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

func officeCampfireActivity(post CampfireMessage, name string) officeActivityEntry {
	id, author := post.SourceSessionID, name
	if post.AgentRole != "" {
		id += ":" + post.AgentRole
		author += " · " + post.AgentRole
	}
	at := post.At.UTC().Format(time.RFC3339Nano)
	return officeActivityEntry{AgentID: id, Author: author, Source: "CAMPFIRE UPDATE", Text: post.Message, Time: at,
		Key: officeActivityKey("campfire", id, at, post.Kind, post.Message)}
}

func updateOfficeActivityHistory(store *Store, snapshot *officeSnapshot) ([]officeActivityEntry, error) {
	if err := store.ensureDirs(); err != nil {
		return nil, err
	}
	var result []officeActivityEntry
	err := withFileLock(filepath.Join(store.StateDir, ".office-activity.lock"), func() error {
		history, err := readOfficeActivityHistory(store)
		if err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(history.Seen))
		for _, key := range history.Seen {
			seen[key] = struct{}{}
		}
		at := snapshot.UpdatedAt
		if at.IsZero() {
			at = time.Now()
		}
		changed := false
		incoming := append([]officeActivityEntry(nil), snapshot.campfire...)
		for _, agent := range snapshot.Agents {
			if agent.ActivitySource == "session" && len(agent.Questions) == 0 {
				continue
			}
			incoming = append(incoming, officeActivityForAgent(agent, at))
		}
		for _, entry := range incoming {
			if strings.TrimSpace(entry.Text) == "" {
				continue
			}
			if _, exists := seen[entry.Key]; exists {
				continue
			}
			seen[entry.Key] = struct{}{}
			history.Seen = append(history.Seen, entry.Key)
			history.Entries = append(history.Entries, entry)
			changed = true
		}
		if len(history.Seen) > maxOfficeActivitySeen {
			history.Seen = history.Seen[len(history.Seen)-maxOfficeActivitySeen:]
			changed = true
		}
		if changed {
			sort.SliceStable(history.Entries, func(i, j int) bool {
				a, _ := time.Parse(time.RFC3339Nano, history.Entries[i].Time)
				b, _ := time.Parse(time.RFC3339Nano, history.Entries[j].Time)
				return a.Before(b)
			})
		}
		if len(history.Entries) > maxOfficeActivityEntries {
			history.Entries = history.Entries[len(history.Entries)-maxOfficeActivityEntries:]
			changed = true
		}
		if changed {
			if err := writeOfficeActivityHistory(store, history); err != nil {
				return err
			}
		}
		result = append([]officeActivityEntry{}, history.Entries...)
		return nil
	})
	return result, err
}

func clearOfficeActivityHistory(store *Store) error {
	if err := store.ensureDirs(); err != nil {
		return err
	}
	return withFileLock(filepath.Join(store.StateDir, ".office-activity.lock"), func() error {
		history, err := readOfficeActivityHistory(store)
		if err != nil {
			return err
		}
		history.Entries = []officeActivityEntry{}
		return writeOfficeActivityHistory(store, history)
	})
}
