package oaktree

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

type focusRunner struct {
	clients, windows                   string
	change, stale, partial, switchFail bool
	resolutions                        int
	commands                           []string
	dispatchArgs                       []string
}

func (r *focusRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	r.commands = append(r.commands, command)
	switch {
	case strings.HasPrefix(command, "tmux display-message"):
		r.resolutions++
		if r.stale || r.change && r.resolutions > 1 {
			return []byte("$42\twrong\n"), nil
		}
		return []byte("$42\ts\n"), nil
	case strings.HasPrefix(command, "tmux list-clients"):
		return []byte(r.clients), nil
	case command == "hyprctl -j clients":
		return []byte(r.windows), nil
	case name == "hyprctl" && len(args) > 0 && args[0] == "dispatch":
		r.dispatchArgs = append([]string(nil), args...)
		if r.partial {
			return []byte("error"), nil
		}
		return []byte("ok"), nil
	}
	return nil, errors.New("unexpected command")
}
func (r *focusRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.Output(ctx, name, args...)
}
func (r *focusRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if r.switchFail {
		return errors.New("switch failed")
	}
	return nil
}

func TestOfficeFocusRouting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(*focusRunner)
		agent    string
		want     int
		switched bool
		partial  bool
	}{
		{"success", func(*focusRunner) {}, "s:architect", 200, true, false},
		{"parent process window", func(r *focusRunner) { r.windows = fmt.Sprintf(`[{"pid":%d,"address":"0x1"}]`, os.Getppid()) }, "s:architect", 200, true, false},
		{"switch failure", func(r *focusRunner) { r.switchFail = true }, "s:architect", 502, true, false},
		{"unknown agent", func(*focusRunner) {}, "missing", 404, false, false},
		{"stale session", func(r *focusRunner) { r.stale = true }, "s:architect", 409, false, false},
		{"multiple clients", func(r *focusRunner) { r.clients += fmt.Sprintf("/dev/pts/2\t%d\n", os.Getpid()) }, "s:architect", 409, false, false},
		{"multiple windows", func(r *focusRunner) {
			r.windows = fmt.Sprintf(`[{"pid":%d,"address":"0x1"},{"pid":%d,"address":"0x2"}]`, os.Getpid(), os.Getpid())
		}, "s:architect", 409, false, false},
		{"ancestry mismatch", func(r *focusRunner) { r.windows = `[{"pid":99999999,"address":"0x1"}]` }, "s:architect", 409, false, false},
		{"revalidation changed", func(r *focusRunner) { r.change = true }, "s:architect", 409, false, false},
		{"partial focus", func(r *focusRunner) { r.partial = true }, "s:architect", 502, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, session, _ := officeQuestionFixture(t)
			session.TmuxSessionID = "$42"
			if err := store.SaveSession(session); err != nil {
				t.Fatal(err)
			}
			r := &focusRunner{clients: fmt.Sprintf("/dev/pts/1\t%d\n", os.Getpid()), windows: fmt.Sprintf(`[{"pid":%d,"address":"0x1"}]`, os.Getpid())}
			tc.setup(r)
			status, message := officeFocusSession(context.Background(), store, r, tc.agent)
			if status != tc.want {
				t.Fatalf("status %d message %q", status, message)
			}
			commands := strings.Join(r.commands, "\n")
			switched := strings.Contains(commands, "tmux switch-client -c /dev/pts/1 -t $42")
			if switched != tc.switched {
				t.Fatalf("switch=%v commands=%s", switched, commands)
			}
			if r.switchFail && len(r.dispatchArgs) != 0 {
				t.Fatal("focus dispatched after switch failure")
			}
			if (tc.want == 200 || tc.partial) && !reflect.DeepEqual(r.dispatchArgs, []string{"dispatch", `hl.dsp.focus({ window = "address:0x1" })`}) {
				t.Fatalf("unexpected dispatch argv: %#v", r.dispatchArgs)
			}
			if tc.partial && !strings.Contains(message, "was switched") {
				t.Fatalf("missing partial result: %q", message)
			}
		})
	}
}

func TestOfficeFocusSecurity(t *testing.T) {
	token := strings.Repeat("a", 64)
	handler := officeHandler(NewStore(t.TempDir()), 4321, token, nil)
	for _, tc := range []struct {
		name, origin, auth, host, body string
		want                           int
	}{
		{"missing origin", "", "Bearer " + token, "127.0.0.1:4321", `{"agentId":"x"}`, 403},
		{"wrong origin", "http://evil.invalid", "Bearer " + token, "127.0.0.1:4321", `{"agentId":"x"}`, 403},
		{"missing auth", "http://127.0.0.1:4321", "", "127.0.0.1:4321", `{"agentId":"x"}`, 401},
		{"wrong auth", "http://127.0.0.1:4321", "Bearer wrong", "127.0.0.1:4321", `{"agentId":"x"}`, 401},
		{"wrong host", "http://127.0.0.1:4321", "Bearer " + token, "localhost:4321", `{"agentId":"x"}`, 403},
		{"extra field", "http://127.0.0.1:4321", "Bearer " + token, "127.0.0.1:4321", `{"agentId":"x","tty":"/dev/pts/1"}`, 422},
		{"unknown agent", "http://127.0.0.1:4321", "Bearer " + token, "127.0.0.1:4321", `{"agentId":"x"}`, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://127.0.0.1:4321/api/focus", strings.NewReader(tc.body))
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", tc.auth)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
		})
	}
}
