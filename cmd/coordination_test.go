package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func TestCoordinationCommandCreatesListsAndReportsManagedAssignments(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	store := oaktree.NewStore(filepath.Join(state, "oak-tree"))
	for _, id := range []string{"organizer", "participant", "unrelated"} {
		if err := store.SaveSession(oaktree.Session{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	run := func(arguments ...string) []byte {
		t.Helper()
		command := newCoordinationCommand()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs(arguments)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	created := run("--oak-session", "organizer", "--request", `{"action":"create","name":"Shared fix","assignments":[{"session_id":"participant","task":"Apply shared fix"}]}`)
	var group oaktree.CoordinationGroup
	if err := json.Unmarshal(created, &group); err != nil {
		t.Fatal(err)
	}
	if group.OrganizerID != "organizer" || len(group.Assignments) != 1 {
		t.Fatalf("created group = %#v", group)
	}
	request, err := json.Marshal(oaktree.CoordinationRequest{Action: "report", GroupID: group.ID, Status: oaktree.AssignmentReturned, Result: "Fix applied"})
	if err != nil {
		t.Fatal(err)
	}
	reported := run("--oak-session", "participant", "--request", string(request))
	if err := json.Unmarshal(reported, &group); err != nil || group.Assignments[0].Status != oaktree.AssignmentReturned {
		t.Fatalf("reported group = %#v, %v", group, err)
	}
	request, err = json.Marshal(oaktree.CoordinationRequest{Action: "list", GroupID: group.ID})
	if err != nil {
		t.Fatal(err)
	}
	listed := run("--request", string(request))
	var snapshot struct {
		Groups   []oaktree.CoordinationGroup `json:"groups"`
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(listed, &snapshot); err != nil || len(snapshot.Groups) != 1 || len(snapshot.Sessions) != 2 {
		t.Fatalf("group-scoped list = %#v, %v", snapshot, err)
	}
}

func TestCoordinationCommandRejectsUnknownCallingSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	command := newCoordinationCommand()
	command.SilenceErrors = true
	command.SetArgs([]string{"--oak-session", "unknown"})
	if err := command.Execute(); err == nil {
		t.Fatal("unknown calling session was accepted")
	}
}
