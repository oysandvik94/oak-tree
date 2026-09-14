package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/oysandvik94/oak-tree/internal/oaktree"
)

func TestCampfireReadHookPrintsNewestTwelveFirst(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	svc, err := newService()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if err := svc.Store.UpdateCampfire(func(messages *[]oaktree.CampfireMessage) error {
		for i := range 13 {
			*messages = append(*messages, oaktree.CampfireMessage{At: at.Add(time.Duration(i) * time.Second), Message: fmt.Sprintf("message %d", i)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCampfireReadHookCommand()
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got []oaktree.CampfireMessage
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 || got[0].Message != "message 12" || got[len(got)-1].Message != "message 1" {
		t.Fatalf("Campfire = %#v, want newest 12 first", got)
	}
}
