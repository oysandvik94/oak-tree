package oaktree

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOfficeChildUsesRequestedStateDir(t *testing.T) {
	dir := t.TempDir()
	log, err := os.CreateTemp(t.TempDir(), "office-log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := officeServerCommand("/example/oak-tree", dir, log)
	want := []string{"/example/oak-tree", "office", "--serve", "--state-dir", dir}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("command args = %q, want %q", cmd.Args, want)
	}
	if cmd.Stderr != log || cmd.Stdout != log || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("child must use detached process and private log")
	}
}

func TestOfficeRecordReadSecuresPermissions(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "office.json")
	content := `{"port":12345,"pid":123,"token":"` + strings.Repeat("a", 64) + `"}`
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOfficeRecord(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %o", info.Mode().Perm())
	}
}

func TestOfficeUnhealthyRecordDoesNotAssumePIDOwnership(t *testing.T) {
	dir := t.TempDir()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	record := officeRecord{Port: port, PID: os.Getpid(), Token: strings.Repeat("a", 64)}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(officeRecordPath(dir), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = EnsureOffice(dir)
	if err == nil || !strings.Contains(err.Error(), "record cannot be verified") {
		t.Fatalf("expected actionable unverified record error, got %v", err)
	}
	got, err := os.ReadFile(officeRecordPath(dir))
	if err != nil || string(got) != string(data) {
		t.Fatal("unverified record changed")
	}
}

func TestOfficeHandlerSecurity(t *testing.T) {
	store := NewStore(t.TempDir())
	token := strings.Repeat("a", 64)
	calls := 0
	handler := officeHandler(store, 4321, token, func() { calls++ })
	for _, tc := range []struct {
		name, method, url, host, origin, auth string
		want                                  int
	}{
		{"state authenticated", "GET", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "http://127.0.0.1:4321", "Bearer " + token, 200},
		{"missing token", "GET", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "", "", 401},
		{"query is not token", "GET", "http://127.0.0.1:4321/api/state?token=" + token, "127.0.0.1:4321", "", "", 401},
		{"wrong token", "GET", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "", "Bearer " + strings.Repeat("b", 64), 401},
		{"wrong host", "GET", "http://127.0.0.1:4321/api/state", "localhost:4321", "", "Bearer " + token, 403},
		{"foreign origin", "GET", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "http://evil.invalid", "Bearer " + token, 403},
		{"null origin", "GET", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "null", "Bearer " + token, 403},
		{"mutation", "POST", "http://127.0.0.1:4321/api/state", "127.0.0.1:4321", "", "Bearer " + token, 405},
		{"unknown file", "GET", "http://127.0.0.1:4321/etc/passwd", "127.0.0.1:4321", "", "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d", w.Code, tc.want)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unexpected CORS header")
			}
			if !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action 'none'") {
				t.Fatal("CSP does not prevent form submissions")
			}
		})
	}
	if calls != 1 {
		t.Fatalf("authenticated activity count=%d", calls)
	}
}

func TestOfficeAnswerRequiresOriginAuthenticationAndJSON(t *testing.T) {
	token := strings.Repeat("a", 64)
	handler := officeHandler(NewStore(t.TempDir()), 4321, token, nil)
	for _, tc := range []struct {
		origin, auth, content string
		want                  int
	}{
		{"", "Bearer " + token, "application/json", 403},
		{"http://evil.invalid", "Bearer " + token, "application/json", 403},
		{"http://127.0.0.1:4321", "", "application/json", 401},
		{"http://127.0.0.1:4321", "Bearer " + token, "text/plain", 415},
		{"http://127.0.0.1:4321", "Bearer " + token, "application/json", 422},
	} {
		req := httptest.NewRequest("POST", "http://127.0.0.1:4321/api/answer", strings.NewReader(`{}`))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Authorization", tc.auth)
		req.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("origin=%q status=%d want=%d", tc.origin, w.Code, tc.want)
		}
	}
}

func TestOfficeAnswerRejectsOversizedBody(t *testing.T) {
	token := strings.Repeat("a", 64)
	handler := officeHandler(NewStore(t.TempDir()), 4321, token, nil)
	body := `{"agentId":"` + strings.Repeat("x", 65536) + `"}`
	req := httptest.NewRequest("POST", "http://127.0.0.1:4321/api/answer", strings.NewReader(body))
	req.Header.Set("Origin", "http://127.0.0.1:4321")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatalf("oversized body status=%d", w.Code)
	}
}

func TestOfficeSessionBucketsOverrideIndividualRoleStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status AgentStatus
		tag    SessionTag
		want   string
	}{
		{"working", AgentStatusWorking, SessionTagNone, "working"},
		{"ready", AgentStatusIdle, SessionTagNone, "ready"},
		{"question", AgentStatusQuestion, SessionTagNone, "question"},
		{"review wins over question", AgentStatusQuestion, SessionTagWaitingReview, "review"},
		{"blocked wins over question", AgentStatusQuestion, SessionTagBlocked, "blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := Session{ID: "paired", AgentStatus: tc.status, Tag: tc.tag, Agents: map[string]PairedAgent{
				"architect": {Status: AgentStatusWorking},
				"junior":    {Status: AgentStatusIdle},
			}}
			snapshot := officeSnapshotFrom([]Session{session}, nil, nil, time.Now())
			if len(snapshot.Agents) != 2 {
				t.Fatalf("agents = %d", len(snapshot.Agents))
			}
			for _, agent := range snapshot.Agents {
				if agent.SessionState != tc.want {
					t.Fatalf("%s session state = %q, want %q", agent.Role, agent.SessionState, tc.want)
				}
			}
			if snapshot.Agents[0].Status != "working" || snapshot.Agents[1].Status != "ready" {
				t.Fatal("individual role statuses were lost")
			}
		})
	}
}

func TestOfficeCoordinationGroupPlacement(t *testing.T) {
	closed := time.Now()
	for _, tc := range []struct {
		name             string
		tag              SessionTag
		closed           bool
		missingOrganizer bool
		wantGroup        bool
		wantState        string
	}{
		{"review", SessionTagWaitingReview, false, false, true, "review"},
		{"blocked", SessionTagBlocked, false, false, true, "blocked"},
		{"unparked", SessionTagNone, false, false, true, ""},
		{"closed", SessionTagWaitingReview, true, false, false, ""},
		{"missing organizer", SessionTagWaitingReview, false, true, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := []Session{{ID: "organizer", AgentStatus: AgentStatusIdle, Tag: tc.tag}}
			group := CoordinationGroup{ID: "team", Name: "Team", OrganizerID: "organizer"}
			if tc.closed {
				group.ClosedAt = &closed
			}
			if tc.missingOrganizer {
				sessions = nil
			}
			for _, member := range []struct {
				id     string
				status AgentStatus
			}{{"ready", AgentStatusIdle}, {"working", AgentStatusWorking}, {"question", AgentStatusQuestion}} {
				sessions = append(sessions, Session{ID: member.id, AgentStatus: member.status, Agents: map[string]PairedAgent{"architect": {Status: AgentStatusWorking}, "junior": {Status: AgentStatusQuestion}}})
				group.Assignments = append(group.Assignments, CoordinationAssignment{SessionID: member.id, Task: "private task", Result: "private result"})
			}
			sessions = append(sessions, Session{ID: "ungrouped", AgentStatus: AgentStatusIdle})
			snapshot := officeSnapshotFrom(sessions, nil, []CoordinationGroup{group}, time.Now())
			for _, agent := range snapshot.Agents {
				if agent.SessionID == "organizer" {
					continue
				}
				if agent.SessionID == "ungrouped" {
					if agent.Group != nil || agent.SessionState != "ready" {
						t.Fatalf("ungrouped = %+v", agent)
					}
					continue
				}
				want := agent.SessionID
				if tc.wantState != "" {
					want = tc.wantState
				}
				if agent.SessionState != want || (agent.Group != nil) != tc.wantGroup {
					t.Fatalf("agent = %+v, want state %s, group %v", agent, want, tc.wantGroup)
				}
				if agent.Group != nil && (agent.Group.ID != "team" || agent.Group.Name != "Team") {
					t.Fatalf("group = %+v", agent.Group)
				}
			}
			data, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "private task") || strings.Contains(string(data), "private result") {
				t.Fatal("assignment details leaked")
			}
		})
	}
}

func TestOfficeGroupReadErrorReturnsFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coordination.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	req := httptest.NewRequest("GET", "http://127.0.0.1:4321/api/state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	officeHandler(NewStore(dir), 4321, token, nil).ServeHTTP(w, req)
	if w.Code != 500 {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestOfficeProjectionUsesRoleTasksAndSessionFallback(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	sessions := []Session{{ID: "one", Root: "/private/parent/project", Branch: "feature", AgentStatus: AgentStatusQuestion, Tag: SessionTagWaitingReview, Agents: map[string]PairedAgent{
		"architect": {Status: AgentStatusQuestion},
		"junior":    {Status: AgentStatusWorking, Todo: &TodoSummary{Tasks: []TodoTask{{Subject: "Implement task", Status: "in_progress"}}}},
	}}, {ID: "two", Root: "/another/repo", AgentStatus: AgentStatusAttention}}
	posts := []CampfireMessage{{SourceSessionID: "one", Message: "Shared update", At: now.Add(-time.Minute)}, {SourceSessionID: "two", Message: "Old update", At: now.Add(-11 * time.Minute)}}
	result := officeSnapshotFrom(sessions, posts, nil, now)
	if len(result.Agents) != 3 {
		t.Fatalf("agents=%d", len(result.Agents))
	}
	a, b, c := result.Agents[0], result.Agents[1], result.Agents[2]
	if a.ID != "one:architect" || a.Status != "question" || a.SessionState != "review" || a.Activity != "Shared update" || a.ActivitySource != "session" {
		t.Fatalf("architect=%+v", a)
	}
	if b.ID != "one:junior" || b.Activity != "Implement task" || b.ActivitySource != "task" {
		t.Fatalf("junior=%+v", b)
	}
	if c.ID != "two" || c.Status != "ready" || c.SessionState != "ready" || c.ActivitySource != "status" {
		t.Fatalf("legacy=%+v", c)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/private/parent") {
		t.Fatal("raw root leaked into snapshot")
	}
}
