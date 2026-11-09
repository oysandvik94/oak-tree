package oaktree

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

//go:embed officeweb
var officeAssets embed.FS

type officeRecord struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type officeAgentState struct {
	ID             string              `json:"id"`
	SessionID      string              `json:"sessionId"`
	Project        string              `json:"project"`
	Branch         string              `json:"branch"`
	Role           string              `json:"role"`
	Status         string              `json:"status"`
	SessionState   string              `json:"sessionState"`
	Activity       string              `json:"activity"`
	ActivitySource string              `json:"activitySource"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	Questions      []officeQuestion    `json:"questions"`
	Group          *officeGroup        `json:"group,omitempty"`
	ToolFailures   *officeToolFailures `json:"toolFailures,omitempty"`
}

type officeGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type officeSnapshot struct {
	Agents    []officeAgentState `json:"agents"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

func officeStatus(status AgentStatus) string {
	switch status {
	case AgentStatusWorking:
		return "working"
	case AgentStatusQuestion:
		return "question"
	case AgentStatusIdle, AgentStatusAttention:
		return "ready"
	default:
		return "unknown"
	}
}

func officeSnapshotFrom(sessions []Session, posts []CampfireMessage, groups []CoordinationGroup, now time.Time) officeSnapshot {
	result := officeSnapshot{Agents: []officeAgentState{}, UpdatedAt: now.UTC()}
	sessionsByID := make(map[string]Session, len(sessions))
	for _, session := range sessions {
		sessionsByID[session.ID] = session
	}
	type membership struct {
		group  officeGroup
		parked string
	}
	members := make(map[string]membership)
	for _, group := range groups {
		organizer, present := sessionsByID[group.OrganizerID]
		if group.ClosedAt != nil || !present {
			continue
		}
		parked := ""
		switch sessionDashboardBucket(organizer) {
		case 3:
			parked = "review"
		case 4:
			parked = "blocked"
		}
		entry := membership{group: officeGroup{ID: group.ID, Name: group.Name}, parked: parked}
		members[group.OrganizerID] = entry
		for _, assignment := range group.Assignments {
			members[assignment.SessionID] = entry
		}
	}
	for _, session := range sessions {
		latest := CampfireMessage{}
		for _, post := range posts {
			if post.SourceSessionID == session.ID && post.At.After(latest.At) {
				latest = post
			}
		}
		for _, post := range session.Campfire {
			if post.At.After(latest.At) {
				latest = post
			}
		}
		state := [...]string{"question", "working", "ready", "review", "blocked"}[sessionDashboardBucket(session)]
		var group *officeGroup
		if member, ok := members[session.ID]; ok {
			group = &member.group
			if member.parked != "" {
				state = member.parked
			}
		}
		add := func(role string, status AgentStatus, todo *TodoSummary) {
			name := officeStatus(status)
			activity, source := name, "status"
			if latest.Message != "" && !latest.At.After(now) && now.Sub(latest.At) <= 10*time.Minute {
				activity, source = latest.Message, "session"
			}
			if todo != nil {
				for _, task := range todo.Tasks {
					if task.Status == "in_progress" && strings.TrimSpace(task.Subject) != "" {
						activity, source = task.Subject, "task"
						break
					}
				}
			}
			id := session.ID
			if role != "" {
				id += ":" + role
			}
			updated := session.UpdatedAt
			if session.AgentStatusUpdatedAt != nil {
				updated = *session.AgentStatusUpdatedAt
			}
			project := filepath.Base(filepath.Clean(session.Root))
			if project == "." || project == string(filepath.Separator) {
				project = "session"
			}
			result.Agents = append(result.Agents, officeAgentState{ID: id, SessionID: session.ID, Project: project, Branch: session.Branch, Role: role, Status: name, SessionState: state, Activity: activity, ActivitySource: source, UpdatedAt: updated.UTC(), Group: group})
		}
		if len(session.Agents) == 0 {
			add("", session.AgentStatus, session.Todo)
			continue
		}
		roles := make([]string, 0, len(session.Agents))
		for role := range session.Agents {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			agent := session.Agents[role]
			add(role, agent.Status, agent.Todo)
		}
	}
	return result
}

func officeHandler(store *Store, port int, token string, onActivity func()) http.Handler {
	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	origin := "http://" + host
	assets := map[string]string{}
	_ = fs.WalkDir(officeAssets, "officeweb", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			rel := strings.TrimPrefix(name, "officeweb/")
			if rel == "index.html" || rel == "office.js" || rel == "questions.js" || rel == "office.css" || strings.HasPrefix(rel, "assets/") {
				assets["/"+rel] = name
			}
		}
		return err
	})
	assets["/"] = assets["/index.html"]
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if r.Host != host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/health" || r.URL.Path == "/api/state" || r.URL.Path == "/api/answer" || r.URL.Path == "/api/focus" {
			isAction := r.URL.Path == "/api/answer" || r.URL.Path == "/api/focus"
			if (isAction && r.Method != http.MethodPost) || (!isAction && r.Method != http.MethodGet) {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if isAction && r.Header.Get("Origin") != origin {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if onActivity != nil {
				onActivity()
			}
			w.Header().Set("Content-Type", "application/json")
			if isAction {
				media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || media != "application/json" {
					w.WriteHeader(http.StatusUnsupportedMediaType)
					officeJSONError(w, "application/json required")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 65536)
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()
				if r.URL.Path == "/api/focus" {
					var request struct {
						AgentID string `json:"agentId"`
					}
					if err := decoder.Decode(&request); err != nil || request.AgentID == "" {
						w.WriteHeader(http.StatusUnprocessableEntity)
						officeJSONError(w, "invalid request")
						return
					}
					var extra any
					if decoder.Decode(&extra) != io.EOF {
						w.WriteHeader(http.StatusUnprocessableEntity)
						officeJSONError(w, "invalid request")
						return
					}
					status, message := officeFocusSession(r.Context(), store, NewOSRunner(""), request.AgentID)
					w.WriteHeader(status)
					if status == http.StatusOK {
						_, _ = io.WriteString(w, `{"ok":true}`)
					} else {
						officeJSONError(w, message)
					}
					return
				}
				var request officeAnswerRequest
				if err := decoder.Decode(&request); err != nil {
					w.WriteHeader(http.StatusUnprocessableEntity)
					officeJSONError(w, "invalid request")
					return
				}
				var extra any
				if err := decoder.Decode(&extra); err != io.EOF {
					w.WriteHeader(http.StatusUnprocessableEntity)
					officeJSONError(w, "invalid request")
					return
				}
				status, message := officeSubmitAnswer(store, request)
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = io.WriteString(w, `{"ok":true}`)
				} else {
					officeJSONError(w, message)
				}
				return
			}
			if r.URL.Path == "/api/health" {
				_, _ = io.WriteString(w, `{"ok":true}`)
				return
			}
			sessions, err := store.ListSessions()
			if err != nil {
				http.Error(w, "Could not load office state", http.StatusInternalServerError)
				return
			}
			posts, err := store.LoadCampfire()
			if err != nil {
				http.Error(w, "Could not load office state", http.StatusInternalServerError)
				return
			}
			groups, err := store.LoadCoordinationGroups()
			if err != nil {
				http.Error(w, "Could not load office state", http.StatusInternalServerError)
				return
			}
			snapshot := officeSnapshotFrom(sessions, posts, groups, time.Now())
			officeAttachQuestions(&snapshot, sessions, officeBridges(store.StateDir))
			_ = json.NewEncoder(w).Encode(snapshot)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name, ok := assets[r.URL.Path]
		if !ok || name == "" {
			http.NotFound(w, r)
			return
		}
		content, err := officeAssets.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write(content)
	})
}

func officeRecordPath(stateDir string) string { return filepath.Join(stateDir, "office.json") }

func readOfficeRecord(stateDir string) (officeRecord, error) {
	var record officeRecord
	data, err := readPrivateFile(officeRecordPath(stateDir))
	if err != nil {
		return record, err
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	decoded, err := hex.DecodeString(record.Token)
	if err != nil || len(decoded) != 32 || record.Port < 1 || record.Port > 65535 || record.PID < 1 {
		return officeRecord{}, errors.New("invalid office runtime record")
	}
	return record, nil
}

func officeHealthy(record officeRecord) bool {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/health", record.Port), nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+record.Token)
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func withOfficeStartupLock(stateDir string, fn func() error) error {
	lock, err := os.OpenFile(filepath.Join(stateDir, ".office.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = lock.Chmod(0o600); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("office startup is busy")
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}

func officeServerCommand(executable, stateDir string, log *os.File) *exec.Cmd {
	child := exec.Command(executable, "office", "--serve", "--state-dir", stateDir)
	child.Stdout = log
	child.Stderr = log
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return child
}

// EnsureOffice starts at most one detached office process per state directory.
func EnsureOffice(stateDir string) (string, error) {
	if err := ensurePrivateDir(stateDir); err != nil {
		return "", err
	}
	var record officeRecord
	err := withOfficeStartupLock(stateDir, func() error {
		existing, err := readOfficeRecord(stateDir)
		if err == nil && officeHealthy(existing) {
			record = existing
			return nil
		}
		if err == nil {
			if processErr := syscall.Kill(existing.PID, 0); !errors.Is(processErr, syscall.ESRCH) {
				return fmt.Errorf("office runtime record cannot be verified; inspect %s before removing it", officeRecordPath(stateDir))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("office runtime record cannot be read; inspect %s: %w", officeRecordPath(stateDir), err)
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		logDir := filepath.Join(stateDir, "logs")
		if err := ensurePrivateDir(logDir); err != nil {
			return err
		}
		logPath := filepath.Join(logDir, "office.log")
		log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("open office log: %w", err)
		}
		defer log.Close()
		if err = log.Chmod(0o600); err != nil {
			return fmt.Errorf("secure office log: %w", err)
		}
		child := officeServerCommand(exe, stateDir, log)
		if err = child.Start(); err != nil {
			return fmt.Errorf("office server could not start; see %s: %w", logPath, err)
		}
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			candidate, err := readOfficeRecord(stateDir)
			if err == nil && candidate.PID == child.Process.Pid && officeHealthy(candidate) {
				record = candidate
				return child.Process.Release()
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
		return fmt.Errorf("office server did not start; see %s", logPath)
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d/#token=%s", record.Port, record.Token), nil
}

func OpenOffice(ctx context.Context, stateDir string) error {
	url, err := EnsureOffice(stateDir)
	if err != nil {
		return err
	}
	openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(openCtx, "xdg-open", url).Run(); err != nil {
		return fmt.Errorf("open office browser: %w", err)
	}
	return nil
}

func ServeOffice(ctx context.Context, store *Store) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	var bytes [32]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		return err
	}
	record := officeRecord{Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(bytes[:]), PID: os.Getpid()}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err = atomicWrite(officeRecordPath(store.StateDir), data, 0o600); err != nil {
		return err
	}
	defer func() {
		current, err := readOfficeRecord(store.StateDir)
		if err == nil && current.Token == record.Token {
			_ = os.Remove(officeRecordPath(store.StateDir))
		}
	}()
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	server := &http.Server{Handler: officeHandler(store, record.Port, record.Token, func() { lastActivity.Store(time.Now().UnixNano()) }), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		case <-ticker.C:
			if time.Since(time.Unix(0, lastActivity.Load())) >= 30*time.Minute {
				shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return server.Shutdown(shutdown)
			}
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		}
	}
}
