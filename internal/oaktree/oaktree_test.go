package oaktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafeComponent(t *testing.T) {
	got := SafeComponent("../Feature/bug fix@123")
	if got != "feature-bug-fix-123" {
		t.Fatalf("SafeComponent() = %q, want %q", got, "feature-bug-fix-123")
	}
	if strings.Contains(got, "..") || strings.Contains(got, "/") {
		t.Fatalf("SafeComponent() produced unsafe component %q", got)
	}
}

func TestWorktreePathAndRepoKey(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	key, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, SafeComponent(filepath.Base(root))+"-") {
		t.Fatalf("RepoKeyFromRoot() = %q, want prefix %q", key, SafeComponent(filepath.Base(root))+"-")
	}
	worktree := WorktreePath(filepath.Join(t.TempDir(), "state"), key, "feature/new-ui")
	if strings.Contains(worktree, "..") || strings.Contains(worktree, "feature/new-ui") {
		t.Fatalf("WorktreePath() did not sanitize path: %q", worktree)
	}
}

func TestStoreSaveLoadUpdateAndList(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(stateDir)
	now := time.Now().UTC().Add(-time.Hour)
	first := Session{
		ID:            "aaa111",
		Root:          "/repo/a",
		Workdir:       "/repo/a",
		RepoKey:       "repo-a",
		OwnedWorktree: false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	second := Session{
		ID:            "bbb222",
		Root:          "/repo/b",
		Workdir:       "/repo/b",
		RepoKey:       "repo-b",
		OwnedWorktree: true,
		CreatedAt:     now.Add(time.Minute),
		UpdatedAt:     now.Add(time.Minute),
	}
	if err := store.SaveSession(first); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(second); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{stateDir, filepath.Join(stateDir, "sessions"), SessionFilePath(stateDir, first.ID)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o700)
		if !info.IsDir() {
			want = 0o600
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
	if err := store.UpdateSession("aaa111", func(s *Session) error {
		s.Branch = "feature/x"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession("aaa111")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Branch != "feature/x" {
		t.Fatalf("UpdateSession() = %q, want %q", loaded.Branch, "feature/x")
	}
	list, err := store.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListSessions() len = %d, want 2", len(list))
	}
	if list[0].ID != "aaa111" {
		t.Fatalf("ListSessions() order = %q first, want aaa111 after update", list[0].ID)
	}
}

func TestStoreSerializesUpdatesAcrossInstances(t *testing.T) {
	stateDir := t.TempDir()
	first, second := NewStore(stateDir), NewStore(stateDir)
	if err := first.SaveSession(Session{ID: "shared", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	firstEntered := make(chan struct{})
	secondStarted := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		errs <- first.UpdateSession("shared", func(session *Session) error {
			close(firstEntered)
			<-releaseFirst
			session.Note = "first"
			return nil
		})
	}()
	<-firstEntered
	go func() {
		close(secondStarted)
		errs <- second.UpdateSession("shared", func(session *Session) error {
			close(secondEntered)
			session.Branch = "second"
			return nil
		})
	}()

	<-secondStarted
	secondWasBlocked := false
	select {
	case <-secondEntered:
	case <-time.After(50 * time.Millisecond):
		secondWasBlocked = true
	}
	close(releaseFirst)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if !secondWasBlocked {
		t.Fatal("second Store entered UpdateSession before the first released its file lock")
	}
	loaded, err := first.LoadSession("shared")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Note != "first" || loaded.Branch != "second" {
		t.Fatalf("concurrent updates lost data: %#v", loaded)
	}
}

func TestStoreSerializesCampfireUpdatesAcrossInstances(t *testing.T) {
	stateDir := t.TempDir()
	first, second := NewStore(stateDir), NewStore(stateDir)
	firstEntered := make(chan struct{})
	secondStarted := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		errs <- first.UpdateCampfire(func(messages *[]CampfireMessage) error {
			close(firstEntered)
			<-releaseFirst
			*messages = append(*messages, CampfireMessage{Message: "first"})
			return nil
		})
	}()
	<-firstEntered
	go func() {
		close(secondStarted)
		errs <- second.UpdateCampfire(func(messages *[]CampfireMessage) error {
			close(secondEntered)
			*messages = append(*messages, CampfireMessage{Message: "second"})
			return nil
		})
	}()

	<-secondStarted
	secondWasBlocked := false
	select {
	case <-secondEntered:
	case <-time.After(50 * time.Millisecond):
		secondWasBlocked = true
	}
	close(releaseFirst)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if !secondWasBlocked {
		t.Fatal("second Store entered UpdateCampfire before the first released its file lock")
	}
	messages, err := first.LoadCampfire()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Message != "first" || messages[1].Message != "second" {
		t.Fatalf("concurrent Campfire updates lost data: %#v", messages)
	}
}

func TestStoreCampfireRetainsLatestByTimestamp(t *testing.T) {
	store := NewStore(t.TempDir())
	base := time.Now().UTC().Add(-time.Hour)
	if err := store.UpdateCampfire(func(messages *[]CampfireMessage) error {
		for i := maxCampfireMessages; i >= 0; i-- {
			*messages = append(*messages, CampfireMessage{At: base.Add(time.Duration(i) * time.Second), Message: fmt.Sprintf("message %d", i)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	messages, err := store.LoadCampfire()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != maxCampfireMessages || messages[0].Message != "message 1" || messages[len(messages)-1].Message != fmt.Sprintf("message %d", maxCampfireMessages) {
		t.Fatalf("global Campfire did not retain latest timestamps: first=%#v last=%#v len=%d", messages[0], messages[len(messages)-1], len(messages))
	}
}

func TestStoreDeleteWaitsForConcurrentUpdate(t *testing.T) {
	stateDir := t.TempDir()
	updater, deleter := NewStore(stateDir), NewStore(stateDir)
	if err := updater.SaveSession(Session{ID: "deleting", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	updateDone := make(chan error, 1)
	deleteDone := make(chan error, 1)
	go func() {
		updateDone <- updater.UpdateSession("deleting", func(session *Session) error {
			close(updateEntered)
			<-releaseUpdate
			session.Note = "updated"
			return nil
		})
	}()
	<-updateEntered
	go func() { deleteDone <- deleter.DeleteSession("deleting") }()

	deleteWasBlocked := false
	select {
	case err := <-deleteDone:
		deleteDone <- err
	case <-time.After(50 * time.Millisecond):
		deleteWasBlocked = true
	}
	close(releaseUpdate)
	if err := <-updateDone; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	if !deleteWasBlocked {
		t.Fatal("DeleteSession did not wait for the active update")
	}
	if _, err := updater.LoadSession("deleting"); !os.IsNotExist(err) {
		t.Fatalf("deleted session was resurrected: %v", err)
	}
}

func TestStoreDeleteArchivesLegacyCampfire(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	at := time.Now().UTC().Add(-time.Minute)
	if err := store.SaveSession(Session{ID: "closing", Root: "/repo/api", Campfire: []CampfireMessage{{At: at, Kind: "plan", Message: "Legacy spark."}}, CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSession("closing"); err != nil {
		t.Fatal(err)
	}
	messages, err := store.LoadCampfire()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Message != "Legacy spark." || messages[0].SourceSessionID != "closing" || messages[0].Project != "api" {
		t.Fatalf("archived Campfire = %#v", messages)
	}
	if _, err := store.LoadSession("closing"); !os.IsNotExist(err) {
		t.Fatalf("session still exists after archive: %v", err)
	}
}

func TestStoreDeleteIgnoresBrokenCampfireArchive(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	if err := store.SaveSession(Session{ID: "closing", Campfire: []CampfireMessage{{At: time.Now().UTC(), Kind: "plan", Message: "Legacy spark."}}, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CampfireFilePath(stateDir), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSession("closing"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSession("closing"); !os.IsNotExist(err) {
		t.Fatalf("broken Campfire archive blocked deletion: %v", err)
	}
}

func TestLoadSessionRestrictsExistingFile(t *testing.T) {
	stateDir := t.TempDir()
	path := SessionFilePath(stateDir, "legacy")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"legacy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(stateDir).LoadSession("legacy"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("legacy session mode = %o, want 600", got)
	}
}

func TestEnsurePrivateWorktreeParentRestrictsExistingDirs(t *testing.T) {
	stateDir := t.TempDir()
	worktreesDir := filepath.Join(stateDir, "worktrees")
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{stateDir, worktreesDir} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	worktreePath := filepath.Join(worktreesDir, "example-repo", "feature")
	if err := ensurePrivateWorktreeParent(Paths{StateDir: stateDir}, worktreePath); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{stateDir, worktreesDir, filepath.Dir(worktreePath)} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s mode = %o, want 700", dir, got)
		}
	}
}

func TestFetchBranchUsesPruningRemoteTrackingRefspec(t *testing.T) {
	runner := &recordingRunner{}
	if err := FetchBranch(context.Background(), runner, "/repo", "main"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(runner.calls[0], "\x00")
	want := strings.Join([]string{"git", "-C", "/repo", "fetch", "--prune", "origin", "+refs/heads/main:refs/remotes/origin/main"}, "\x00")
	if got != want {
		t.Fatalf("FetchBranch call = %#v, want %#v", runner.calls[0], strings.Split(want, "\x00"))
	}
}

func TestTmuxCommandArgs(t *testing.T) {
	newArgs := NewSessionCommandArgs("oak-123", "/repo", []string{"nvim"})
	wantNew := []string{"new-session", "-d", "-P", "-F", "#{session_id}\t#{pane_id}", "-s", "oak-123", "-c", "/repo", "nvim"}
	if strings.Join(newArgs, "\x00") != strings.Join(wantNew, "\x00") {
		t.Fatalf("NewSessionCommandArgs() = %#v, want %#v", newArgs, wantNew)
	}
	splitArgs := SplitWindowCommandArgs("%1", "/repo", []string{"pi"})
	wantSplit := []string{"split-window", "-h", "-b", "-p", "60", "-t", "%1", "-c", "/repo", "-P", "-F", "#{pane_id}", "pi"}
	if strings.Join(splitArgs, "\x00") != strings.Join(wantSplit, "\x00") {
		t.Fatalf("SplitWindowCommandArgs() = %#v, want %#v", splitArgs, wantSplit)
	}
	paneArgs := PaneIdCommandArgs("%1")
	wantPane := []string{"display-message", "-p", "-t", "%1", "#{pane_id}"}
	if strings.Join(paneArgs, "\x00") != strings.Join(wantPane, "\x00") {
		t.Fatalf("PaneIdCommandArgs() = %#v, want %#v", paneArgs, wantPane)
	}
	listPanesArgs := ListPanesArgs("oak-123")
	wantListPanes := []string{"list-panes", "-s", "-t", "oak-123", "-F", "#{pane_id}\t#{pane_current_command}\t#{pane_current_path}"}
	if strings.Join(listPanesArgs, "\x00") != strings.Join(wantListPanes, "\x00") {
		t.Fatalf("ListPanesArgs() = %#v, want %#v", listPanesArgs, wantListPanes)
	}
	hasArgs := HasSessionArgs("oak-123")
	wantHas := []string{"has-session", "-t", "oak-123"}
	if strings.Join(hasArgs, "\x00") != strings.Join(wantHas, "\x00") {
		t.Fatalf("HasSessionArgs() = %#v, want %#v", hasArgs, wantHas)
	}
}

func TestCreateTmuxSessionTargetsInitialPaneAndCleansUpOnSplitFailure(t *testing.T) {
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case len(args) > 0 && args[0] == "new-session":
				return []byte("%7\t%1\n"), nil
			case len(args) > 0 && args[0] == "split-window":
				if len(args) < 7 || args[5] != "-t" || args[6] != "%1" {
					t.Fatalf("split-window target = %#v, want %q", args, "%1")
				}
				return nil, errors.New("split failed")
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
			}
			return nil, nil
		},
		runFunc: func(name string, args []string) error {
			if name != "tmux" || len(args) == 0 || args[0] != "kill-session" {
				t.Fatalf("unexpected Run call: %s %#v", name, args)
			}
			if got := strings.Join(args, "\x00"); got != strings.Join([]string{"kill-session", "-t", "oak-123"}, "\x00") {
				t.Fatalf("kill-session args = %#v, want oak-123 cleanup", args)
			}
			return nil
		},
	}

	session, err := CreateTmuxSession(context.Background(), runner, "oak-123", "/repo")
	if err == nil {
		t.Fatal("CreateTmuxSession() error = nil, want failure")
	}
	if session.Name != "" || session.ID != "" || session.LeftPaneID != "" || session.RightPaneID != "" {
		t.Fatalf("CreateTmuxSession() session = %#v, want zero value on error", session)
	}
	if !strings.Contains(err.Error(), "split failed") {
		t.Fatalf("CreateTmuxSession() error = %v, want split failure", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("runner calls = %#v, want 3 calls", runner.calls)
	}
	wantNew := []string{"tmux", "new-session", "-d", "-P", "-F", "#{session_id}\t#{pane_id}", "-s", "oak-123", "-c", "/repo", "nvim"}
	wantSplit := []string{"tmux", "split-window", "-h", "-b", "-p", "60", "-t", "%1", "-c", "/repo", "-P", "-F", "#{pane_id}", "pi"}
	wantKill := []string{"tmux", "kill-session", "-t", "oak-123"}
	if strings.Join(runner.calls[0], "\x00") != strings.Join(wantNew, "\x00") {
		t.Fatalf("new-session call = %#v, want %#v", runner.calls[0], wantNew)
	}
	if strings.Join(runner.calls[1], "\x00") != strings.Join(wantSplit, "\x00") {
		t.Fatalf("split-window call = %#v, want %#v", runner.calls[1], wantSplit)
	}
	if strings.Join(runner.calls[2], "\x00") != strings.Join(wantKill, "\x00") {
		t.Fatalf("kill-session call = %#v, want %#v", runner.calls[2], wantKill)
	}
}

func TestStableHookExecutablePrefersPathForGoRunBinary(t *testing.T) {
	got, err := stableHookExecutable("/tmp/go-build123/b001/exe/oak-tree", func(name string) (string, error) {
		if name != "oak-tree" {
			t.Fatalf("lookPath name = %q, want oak-tree", name)
		}
		return "/home/me/go/bin/oak-tree", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/me/go/bin/oak-tree" {
		t.Fatalf("stableHookExecutable() = %q, want installed binary", got)
	}
}

func TestStableHookExecutableRejectsGoRunBinaryWithoutPathFallback(t *testing.T) {
	_, err := stableHookExecutable("/tmp/go-build123/b001/exe/oak-tree", func(name string) (string, error) {
		return "", errors.New("not found")
	})
	if err == nil {
		t.Fatal("stableHookExecutable() error = nil, want go run guidance")
	}
	if !strings.Contains(err.Error(), "go install") {
		t.Fatalf("stableHookExecutable() error = %q, want go install guidance", err)
	}
}

type recordingRunner struct {
	calls        [][]string
	outputFunc   func(name string, args []string) ([]byte, error)
	combinedFunc func(name string, args []string) ([]byte, error)
	runFunc      func(name string, args []string) error
}

func (r *recordingRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.outputFunc != nil {
		return r.outputFunc(name, args)
	}
	return nil, nil
}

func (r *recordingRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.combinedFunc != nil {
		return r.combinedFunc(name, args)
	}
	if r.outputFunc != nil {
		return r.outputFunc(name, args)
	}
	return nil, nil
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.runFunc != nil {
		return r.runFunc(name, args)
	}
	return nil
}
