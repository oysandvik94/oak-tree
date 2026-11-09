package oaktree

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestOfficeActivityBrowserTracking(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source, err := officeAssets.ReadFile("officeweb/activity.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source) + `
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const tracker = createActivityTracker();
const old = {key:"old"}, first = {key:"first"}, second = {key:"second"};
assert(tracker.observe([old]).length === 0, "restored history replayed");
assert(tracker.observe([old, first]).length === 1, "new message missing");
assert(tracker.observe([old, first]).length === 0, "poll replayed a message");
tracker.observe([]);
assert(tracker.observe([first]).length === 0, "clear replayed a message");
assert(tracker.observe([second])[0] === second, "new message after clear missing");
const actors = Array.from({length:8}, () => ({bubbleQueue:[first, second], bubble:null, bubbleUntil:0}));
assert(actors.filter(actor => advanceBubble(actor, 0)).length === 8, "concurrent speakers capped");
assert(actors.every(actor => advanceBubble(actor, 9999) === first), "new message displaced active bubble");
assert(actors.every(actor => advanceBubble(actor, 10000) === second), "queued message missing");
assert(actors.every(actor => advanceBubble(actor, 20000) === null), "expired message replayed");
assert(actors.every(actor => advanceBubble(actor, 30000) === null), "rotation replayed a message");
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("activity tracking: %v\n%s", err, output)
	}
}
