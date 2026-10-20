package oaktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOakSessionOptionsArgsChainsNonEmptyOptions(t *testing.T) {
	got := OakSessionOptionsArgs("$5", "oak-1", "%1", "%2")
	want := []string{
		"set-option", "-t", "$5", "@oak_tree_session_id", "oak-1", ";",
		"set-option", "-t", "$5", "@oak_nvim_pane", "%1", ";",
		"set-option", "-t", "$5", "@oak_pi_pane", "%2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OakSessionOptionsArgs() = %#v, want %#v", got, want)
	}
	got = OakSessionOptionsArgs("$5", "oak-1", "", "%2")
	want = []string{
		"set-option", "-t", "$5", "@oak_tree_session_id", "oak-1", ";",
		"set-option", "-t", "$5", "@oak_pi_pane", "%2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OakSessionOptionsArgs() without nvim pane = %#v, want %#v", got, want)
	}
	if got := SetSessionOptionArgs("$5", "@oak_pi_pane", "%62"); !reflect.DeepEqual(got, []string{"set-option", "-t", "$5", "@oak_pi_pane", "%62"}) {
		t.Fatalf("SetSessionOptionArgs() = %#v", got)
	}
}

func TestSetOakSessionOptionsSkipsMissingTarget(t *testing.T) {
	runner := &recordingRunner{}
	if err := SetOakSessionOptions(context.Background(), runner, "", "oak-1", "%1", "%2"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want none without a tmux target", runner.calls)
	}
}

func TestCreateSessionPublishesTmuxPaneOptions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		optErr error
	}{
		{name: "success"},
		{name: "option failure is ignored", optErr: errors.New("set-option failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "pi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			root := filepath.Join(t.TempDir(), "repo")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			state := t.TempDir()
			paths := Paths{StateDir: state, PiDir: filepath.Join(state, "pi"), PiExtensionsDir: filepath.Join(t.TempDir(), "extensions")}
			store := NewStore(state)
			runner := &recordingRunner{
				outputFunc: func(name string, args []string) ([]byte, error) {
					switch {
					case name == "tmux" && len(args) > 0 && args[0] == "new-session":
						return []byte("$5\t%1\n"), nil
					case name == "tmux" && len(args) > 0 && args[0] == "split-window":
						return []byte("%2\n"), nil
					default:
						t.Fatalf("unexpected Output call: %s %#v", name, args)
						return nil, nil
					}
				},
				runFunc: func(name string, args []string) error {
					if name == "tmux" && len(args) > 0 && args[0] == "set-option" {
						return tc.optErr
					}
					t.Fatalf("unexpected Run call: %s %#v", name, args)
					return nil
				},
			}
			svc := NewService(paths, store, runner)

			session, err := svc.CreateSession(context.Background(), CreateSessionInput{Root: root})
			if err != nil {
				t.Fatalf("CreateSession() error = %v", err)
			}
			if session.TmuxSessionID != "$5" || session.LeftPaneID != "%1" || session.RightPaneID != "%2" {
				t.Fatalf("CreateSession() session = %#v", session)
			}
			assertRecordedCall(t, runner.calls, append([]string{"tmux"}, OakSessionOptionsArgs("$5", session.ID, "%1", "%2")...))
		})
	}
}

func TestHandleAgentEventPublishesTmuxPaneOptionsOnlyWhenNeeded(t *testing.T) {
	state := t.TempDir()
	store := NewStore(state)
	if err := store.SaveSession(Session{ID: "oak-1", TmuxSessionID: "$5", LeftPaneID: "%1", RightPaneID: "%2", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var optionCalls [][]string
	optErr := error(nil)
	runner := &stubRunner{runFunc: func(name string, args []string) error {
		if name == "tmux" {
			optionCalls = append(optionCalls, args)
			return optErr
		}
		return nil
	}}
	svc := NewService(Paths{StateDir: state}, store, runner)
	ctx := context.Background()

	for _, step := range []struct {
		event AgentEvent
		want  []string
	}{
		{event: AgentEvent{OakSessionID: "oak-1", TmuxPaneID: "%2", Event: "session_start"}, want: OakSessionOptionsArgs("$5", "oak-1", "%1", "%2")},
		{event: AgentEvent{OakSessionID: "oak-1", TmuxPaneID: "%2", Event: "agent_start"}},
		{event: AgentEvent{OakSessionID: "oak-1", Event: "agent_settled"}},
		{event: AgentEvent{OakSessionID: "oak-1", TmuxPaneID: "%9", Event: "agent_start"}, want: OakSessionOptionsArgs("$5", "oak-1", "%1", "%9")},
		{event: AgentEvent{OakSessionID: "oak-1", Event: "session_start"}, want: OakSessionOptionsArgs("$5", "oak-1", "%1", "%9")},
	} {
		optionCalls = nil
		if err := svc.HandleAgentEvent(ctx, step.event); err != nil {
			t.Fatalf("HandleAgentEvent(%#v) error = %v", step.event, err)
		}
		switch {
		case step.want == nil && len(optionCalls) != 0:
			t.Fatalf("HandleAgentEvent(%#v) tmux calls = %#v, want none", step.event, optionCalls)
		case step.want != nil && (len(optionCalls) != 1 || !reflect.DeepEqual(optionCalls[0], step.want)):
			t.Fatalf("HandleAgentEvent(%#v) tmux calls = %#v, want %#v", step.event, optionCalls, step.want)
		}
	}

	optErr = errors.New("set-option failed")
	if err := svc.HandleAgentEvent(ctx, AgentEvent{OakSessionID: "oak-1", TmuxPaneID: "%10", Event: "session_start"}); err != nil {
		t.Fatalf("HandleAgentEvent() error = %v, want tmux option failure ignored", err)
	}
	got, err := store.LoadSession("oak-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RightPaneID != "%10" {
		t.Fatalf("RightPaneID = %q, want %%10", got.RightPaneID)
	}
}
