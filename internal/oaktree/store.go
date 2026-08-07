package oaktree

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	StateDir string
	mu       sync.Mutex
}

type DashboardPreferences struct {
	KanbanView bool `json:"kanban_view"`
}

func NewStore(stateDir string) *Store {
	return &Store{StateDir: stateDir}
}

func (s *Store) ensureDirs() error {
	for _, dir := range []string{
		s.StateDir,
		filepath.Join(s.StateDir, "sessions"),
		filepath.Join(s.StateDir, "worktrees"),
		filepath.Join(s.StateDir, "hooks"),
		filepath.Join(s.StateDir, "cache"),
	} {
		if err := ensurePrivateDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func readPrivateFile(path string) ([]byte, error) {
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func atomicWrite(path string, data []byte, mode fs.FileMode) error {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Store) SaveSession(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSessionLocked(session)
}

func (s *Store) saveSessionLocked(session Session) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	path := SessionFilePath(s.StateDir, session.ID)
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0o600)
}

func (s *Store) LoadSession(id string) (Session, error) {
	path := SessionFilePath(s.StateDir, id)
	data, err := readPrivateFile(path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Store) ListSessions() ([]Session, error) {
	if err := s.ensureDirs(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.StateDir, "sessions"))
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		session, err := s.LoadSession(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (s *Store) UpdateSession(id string, fn func(*Session) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.LoadSession(id)
	if err != nil {
		return err
	}
	if err := fn(&session); err != nil {
		return err
	}
	session.UpdatedAt = time.Now().UTC()
	return s.saveSessionLocked(session)
}

func (s *Store) FindSessionByWorkdir(workdir string) (*Session, error) {
	sessions, err := s.ListSessions()
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if SamePath(sessions[i].Workdir, workdir) {
			copy := sessions[i]
			return &copy, nil
		}
	}
	return nil, errors.New("session not found")
}

func (s *Store) FindSessionByID(id string) (*Session, error) {
	session, err := s.LoadSession(id)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) LoadDashboardPreferences() (DashboardPreferences, error) {
	data, err := readPrivateFile(DashboardPreferencesFilePath(s.StateDir))
	if err != nil {
		return DashboardPreferences{}, err
	}
	var preferences DashboardPreferences
	if err := json.Unmarshal(data, &preferences); err != nil {
		return DashboardPreferences{}, err
	}
	return preferences, nil
}

func (s *Store) SaveDashboardPreferences(preferences DashboardPreferences) error {
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(DashboardPreferencesFilePath(s.StateDir), data, 0o600)
}

func (s *Store) LoadUsageCache() (UsageCache, error) {
	data, err := readPrivateFile(UsageCacheFilePath(s.StateDir))
	if err != nil {
		return UsageCache{}, err
	}
	var cache UsageCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return UsageCache{}, err
	}
	return cache, nil
}

func (s *Store) SaveUsageCache(cache UsageCache) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(UsageCacheFilePath(s.StateDir), data, 0o600)
}
