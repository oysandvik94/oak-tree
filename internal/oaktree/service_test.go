package oaktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTmuxSessionNameDescribesRepositoryAndBranch(t *testing.T) {
	id := "a1b2c3d4e5f6"
	if got, want := tmuxSessionName("/repos/oak-tree", "feature/session names", id), "oak-oak-tree-feature-session-names-a1b2c3"; got != want {
		t.Fatalf("tmuxSessionName() = %q, want %q", got, want)
	}
	if got, want := tmuxSessionName("/repos/oak-tree", "", id), "oak-oak-tree-a1b2c3"; got != want {
		t.Fatalf("tmuxSessionName() without branch = %q, want %q", got, want)
	}
}

type stubRunner struct {
	outputFunc func(name string, args []string) ([]byte, error)
	runFunc    func(name string, args []string) error
}

func (r *stubRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.outputFunc != nil {
		return r.outputFunc(name, args)
	}
	return nil, nil
}

func (r *stubRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.Output(ctx, name, args...)
}

func (r *stubRunner) Run(ctx context.Context, name string, args ...string) error {
	if r.runFunc != nil {
		return r.runFunc(name, args)
	}
	return nil
}

func TestParseGitStatusPorcelainCountsDirtyEntries(t *testing.T) {
	status := parseGitStatusPorcelain("## main...origin/main [ahead 2]\n M tracked.go\nA  added.go\n?? notes.txt\nR  old.go -> new.go\n")

	if status.Clean {
		t.Fatal("Clean = true, want dirty status")
	}
	if status.Ahead != 2 {
		t.Fatalf("Ahead = %d, want 2", status.Ahead)
	}
	if status.Changed != 3 {
		t.Fatalf("Changed = %d, want 3", status.Changed)
	}
	if status.Untracked != 1 {
		t.Fatalf("Untracked = %d, want 1", status.Untracked)
	}
}

func TestListSessionsIncludesGitStatus(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{
		ID:        "session-git-status",
		Root:      "/repo",
		Workdir:   "/repo",
		RepoKey:   "repo",
		Branch:    "old-branch",
		PR:        &PRInfo{Found: true, Number: 4},
		CreatedAt: testTime(),
		UpdatedAt: testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			if name == "git" && argsContain(args, "status") && argsContain(args, "--branch") && argsContain(args, "--porcelain=v1") {
				return []byte("## feature/test...origin/feature/test [ahead 1]\n M tracked.go\n?? scratch.txt\n"), nil
			}
			t.Fatalf("unexpected Output call: %s %#v", name, args)
			return nil, nil
		},
	}
	svc := NewService(Paths{StateDir: stateDir}, store, runner)

	sessions, err := svc.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	status := sessions[0].GitStatus
	if status == nil {
		t.Fatal("GitStatus = nil, want status")
	}
	if status.Clean || status.Changed != 1 || status.Untracked != 1 || status.Ahead != 1 || status.Branch != "feature/test" {
		t.Fatalf("GitStatus = %#v, want branch and one changed, one untracked, and one unpushed commit", status)
	}
	if sessions[0].Branch != "feature/test" || sessions[0].PR != nil {
		t.Fatalf("reconciled session = %#v, want current branch and cleared PR", sessions[0])
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Branch != "feature/test" || loaded.PR != nil {
		t.Fatalf("persisted session = %#v, want current branch and cleared PR", loaded)
	}
}

func TestParseGitStatusPorcelainDetachedHeadHasNoBranch(t *testing.T) {
	status := parseGitStatusPorcelain("## HEAD (no branch)\n M tracked.go\n")
	if status.Branch != "" {
		t.Fatalf("Branch = %q, want empty for detached HEAD", status.Branch)
	}
}

func TestCreateSessionWrapsHighValueFailures(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("default branch", func(t *testing.T) {
		paths := Paths{StateDir: t.TempDir()}
		store := NewStore(paths.StateDir)
		runner := &stubRunner{
			outputFunc: func(name string, args []string) ([]byte, error) {
				return nil, errors.New("exit status 1")
			},
		}
		svc := NewService(paths, store, runner)
		_, err := svc.CreateSession(context.Background(), CreateSessionInput{
			Root:   root,
			Branch: "feature/a",
		})
		if err == nil {
			t.Fatal("CreateSession() error = nil, want failure")
		}
		assertErrorContains(t, err, "create session for "+root)
		assertErrorContains(t, err, "determine default branch")
	})

	t.Run("add worktree", func(t *testing.T) {
		paths := Paths{StateDir: t.TempDir()}
		store := NewStore(paths.StateDir)
		runner := &stubRunner{
			outputFunc: func(name string, args []string) ([]byte, error) {
				return []byte("main\n"), nil
			},
			runFunc: func(name string, args []string) error {
				if name == "git" && argsContain(args, "worktree") && argsContain(args, "add") {
					return errors.New("exit status 1")
				}
				return nil
			},
		}
		svc := NewService(paths, store, runner)
		_, err := svc.CreateSession(context.Background(), CreateSessionInput{
			Root:   root,
			Branch: "feature/b",
		})
		if err == nil {
			t.Fatal("CreateSession() error = nil, want failure")
		}
		assertErrorContains(t, err, "create session for "+root)
		assertErrorContains(t, err, "add worktree for branch feature/b")
	})

	t.Run("tmux session", func(t *testing.T) {
		paths := Paths{StateDir: t.TempDir()}
		store := NewStore(paths.StateDir)
		runner := &stubRunner{
			outputFunc: func(name string, args []string) ([]byte, error) {
				return nil, errors.New("exit status 1")
			},
		}
		svc := NewService(paths, store, runner)
		_, err := svc.CreateSession(context.Background(), CreateSessionInput{
			Root: root,
		})
		if err == nil {
			t.Fatal("CreateSession() error = nil, want failure")
		}
		assertErrorContains(t, err, "create session for "+root)
		assertErrorContains(t, err, "create tmux session")
	})

	t.Run("save state", func(t *testing.T) {
		stateFile := filepath.Join(t.TempDir(), "state")
		if err := os.WriteFile(stateFile, []byte("locked"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths := Paths{StateDir: stateFile}
		store := NewStore(paths.StateDir)
		outputs := []string{"%1\t%2\n", "%3\n"}
		outputIndex := 0
		runner := &stubRunner{
			outputFunc: func(name string, args []string) ([]byte, error) {
				if outputIndex >= len(outputs) {
					return nil, errors.New("unexpected output call")
				}
				out := outputs[outputIndex]
				outputIndex++
				return []byte(out), nil
			},
			runFunc: func(name string, args []string) error {
				return nil
			},
		}
		svc := NewService(paths, store, runner)
		_, err := svc.CreateSession(context.Background(), CreateSessionInput{
			Root: root,
		})
		if err == nil {
			t.Fatal("CreateSession() error = nil, want failure")
		}
		assertErrorContains(t, err, "create session for "+root)
		assertErrorContains(t, err, "save session state")
	})
}

func TestCreateSessionNewBranchConfiguresSameNameUpstream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "remove_config_validator"
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "symbolic-ref"):
				return []byte("origin/develop\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return []byte("%1\t%2\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "split-window":
				return []byte("%3\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			if name == "git" && argsContain(args, "rev-parse") && argsContain(args, "refs/heads/"+branch) {
				return errors.New("branch does not exist")
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	session, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:   root,
		Branch: branch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Workdir != worktreePath {
		t.Fatalf("CreateSession() workdir = %q, want %q", session.Workdir, worktreePath)
	}
	wantFetch := []string{"git", "-C", root, "fetch", "--prune", "origin", "+refs/heads/develop:refs/remotes/origin/develop"}
	wantAdd := []string{"git", "-C", root, "worktree", "add", "-b", branch, "--no-track", worktreePath, "origin/develop"}
	wantRemote := []string{"git", "-C", root, "config", "branch." + branch + ".remote", "origin"}
	wantMerge := []string{"git", "-C", root, "config", "branch." + branch + ".merge", "refs/heads/" + branch}
	assertRecordedCall(t, runner.calls, wantFetch)
	assertRecordedCall(t, runner.calls, wantAdd)
	assertRecordedCall(t, runner.calls, wantRemote)
	assertRecordedCall(t, runner.calls, wantMerge)
}

func TestCreateSessionExistingCreateBranchRepairsSameNameUpstream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "remove_config_validator"
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "symbolic-ref"):
				return []byte("origin/develop\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return []byte("%1\t%2\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "split-window":
				return []byte("%3\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	session, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:   root,
		Branch: branch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Workdir != worktreePath {
		t.Fatalf("CreateSession() workdir = %q, want %q", session.Workdir, worktreePath)
	}
	wantAdd := []string{"git", "-C", root, "worktree", "add", worktreePath, branch}
	wantRemote := []string{"git", "-C", root, "config", "branch." + branch + ".remote", "origin"}
	wantMerge := []string{"git", "-C", root, "config", "branch." + branch + ".merge", "refs/heads/" + branch}
	assertRecordedCall(t, runner.calls, wantAdd)
	assertRecordedCall(t, runner.calls, wantRemote)
	assertRecordedCall(t, runner.calls, wantMerge)
}

func TestCreateSessionOpenExistingUsesLocalBranchWorktree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/local"
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + root + "\nHEAD abc123\nbranch refs/heads/main\n\n"), nil
			case name == "git" && argsContain(args, "ls-remote"):
				return []byte(""), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return []byte("%1\t%2\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "split-window":
				return []byte("%3\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			switch {
			case name == "git" && argsContain(args, "rev-parse") && argsContain(args, "refs/heads/"+branch):
				return nil
			case name == "git" && argsContain(args, "fetch"):
				t.Fatalf("unexpected fetch call for local branch: %#v", args)
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "add"):
				wantAdd := []string{"git", "-C", root, "worktree", "add", worktreePath, branch}
				if strings.Join(append([]string{name}, args...), "\x00") != strings.Join(wantAdd, "\x00") {
					t.Fatalf("worktree add args = %#v, want %#v", append([]string{name}, args...), wantAdd)
				}
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	session, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !session.OwnedWorktree {
		t.Fatal("CreateSession() OwnedWorktree = false, want true for local branch worktree")
	}
	if session.Workdir != worktreePath {
		t.Fatalf("CreateSession() workdir = %q, want %q", session.Workdir, worktreePath)
	}
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "worktree", "list", "--porcelain"})
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/" + branch})
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "worktree", "add", worktreePath, branch})
}

func TestCreateSessionOpenExistingUsesRemoteOnlyBranchTrackingWorktree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/remote"
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + root + "\nHEAD abc123\nbranch refs/heads/main\n\n"), nil
			case name == "git" && argsContain(args, "ls-remote"):
				return []byte("1234567890abcdef\trefs/heads/" + branch + "\n"), nil
			case name == "git" && argsContain(args, "symbolic-ref"):
				t.Fatalf("unexpected default-branch lookup in open-existing mode")
				return nil, nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return []byte("%1\t%2\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "split-window":
				return []byte("%3\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			switch {
			case name == "git" && argsContain(args, "rev-parse") && argsContain(args, "refs/heads/"+branch):
				return errors.New("branch does not exist")
			case name == "git" && argsContain(args, "fetch"):
				wantFetch := []string{"git", "-C", root, "fetch", "--prune", "origin", "+refs/heads/" + branch + ":refs/remotes/origin/" + branch}
				if strings.Join(append([]string{name}, args...), "\x00") != strings.Join(wantFetch, "\x00") {
					t.Fatalf("fetch args = %#v, want %#v", append([]string{name}, args...), wantFetch)
				}
			case name == "git" && argsContain(args, "config"):
				t.Fatalf("unexpected config call for open-existing branch: %#v", args)
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "add"):
				wantAdd := []string{"git", "-C", root, "worktree", "add", "-b", branch, "--track", worktreePath, "origin/" + branch}
				if strings.Join(append([]string{name}, args...), "\x00") != strings.Join(wantAdd, "\x00") {
					t.Fatalf("worktree add args = %#v, want %#v", append([]string{name}, args...), wantAdd)
				}
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	session, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !session.OwnedWorktree {
		t.Fatal("CreateSession() OwnedWorktree = false, want true for remote-only branch worktree")
	}
	if session.Workdir != worktreePath {
		t.Fatalf("CreateSession() workdir = %q, want %q", session.Workdir, worktreePath)
	}
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "worktree", "list", "--porcelain"})
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "ls-remote", "--heads", "origin", "refs/heads/" + branch})
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "fetch", "--prune", "origin", "+refs/heads/" + branch + ":refs/remotes/origin/" + branch})
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "worktree", "add", "-b", branch, "--track", worktreePath, "origin/" + branch})
}

func TestCreateSessionOpenExistingRejectsMissingBranch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/missing"
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + root + "\nHEAD abc123\nbranch refs/heads/main\n\n"), nil
			case name == "git" && argsContain(args, "ls-remote"):
				return []byte(""), nil
			default:
				t.Fatalf("unexpected Output call before branch rejection: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			if name == "git" && argsContain(args, "rev-parse") && argsContain(args, "refs/heads/"+branch) {
				return errors.New("branch does not exist")
			}
			if name == "tmux" {
				t.Fatalf("tmux should not start for missing branch: %#v", args)
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	_, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err == nil {
		t.Fatal("CreateSession() error = nil, want missing-branch failure")
	}
	assertErrorContains(t, err, "open existing branch "+branch)
	assertErrorContains(t, err, "does not exist locally or on origin")
}

func TestCreateSessionOpenExistingReusesCheckedOutWorktree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/shared"
	existingWorktree := filepath.Join(t.TempDir(), "other-worktree")
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + existingWorktree + "\nHEAD abc123\nbranch refs/heads/" + branch + "\n\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return []byte("%1\t%2\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "split-window":
				return []byte("%3\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			if name == "git" && argsContain(args, "fetch") {
				t.Fatalf("unexpected fetch call when reusing checked-out worktree: %#v", args)
			}
			if name == "git" && argsContain(args, "worktree") && argsContain(args, "add") {
				t.Fatalf("unexpected worktree add when reusing checked-out worktree: %#v", args)
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	session, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.OwnedWorktree {
		t.Fatal("CreateSession() OwnedWorktree = true, want false for reused worktree")
	}
	if session.Workdir != existingWorktree {
		t.Fatalf("CreateSession() workdir = %q, want %q", session.Workdir, existingWorktree)
	}
	assertRecordedCall(t, runner.calls, []string{"git", "-C", root, "worktree", "list", "--porcelain"})
	assertRecordedCall(t, runner.calls, []string{"tmux", "new-session", "-d", "-P", "-F", "#{session_id}\t#{pane_id}", "-s", session.TmuxSessionName, "-c", existingWorktree, "nvim"})
}

func TestCreateSessionOpenExistingDoesNotRemoveReusedWorktreeOnTmuxFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/shared"
	existingWorktree := filepath.Join(t.TempDir(), "other-worktree")
	removeCalled := false
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + existingWorktree + "\nHEAD abc123\nbranch refs/heads/" + branch + "\n\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return nil, errors.New("tmux unavailable")
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			if name == "git" && argsContain(args, "worktree") && argsContain(args, "remove") {
				removeCalled = true
			}
			if name == "git" && argsContain(args, "fetch") {
				t.Fatalf("unexpected fetch call when reusing checked-out worktree: %#v", args)
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	_, err := svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err == nil {
		t.Fatal("CreateSession() error = nil, want tmux failure")
	}
	assertErrorContains(t, err, "create tmux session")
	if removeCalled {
		t.Fatal("CreateSession() removed a reused worktree after tmux failure")
	}
}

func TestCreateSessionOpenExistingRemovesOwnedWorktreeOnTmuxFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{StateDir: t.TempDir()}
	store := NewStore(paths.StateDir)
	branch := "feature/owned"
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	removeCalled := false
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "git" && argsContain(args, "worktree") && argsContain(args, "list"):
				return []byte("worktree " + root + "\nHEAD abc123\nbranch refs/heads/main\n\n"), nil
			case name == "git" && argsContain(args, "ls-remote"):
				return []byte("1234567890abcdef\trefs/heads/" + branch + "\n"), nil
			case name == "tmux" && len(args) > 0 && args[0] == "new-session":
				return nil, errors.New("tmux unavailable")
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
		runFunc: func(name string, args []string) error {
			if name == "git" && argsContain(args, "rev-parse") && argsContain(args, "refs/heads/"+branch) {
				return errors.New("branch does not exist")
			}
			if name == "git" && argsContain(args, "fetch") {
				return nil
			}
			if name == "git" && argsContain(args, "worktree") && argsContain(args, "add") {
				wantAdd := []string{"git", "-C", root, "worktree", "add", "-b", branch, "--track", worktreePath, "origin/" + branch}
				if strings.Join(append([]string{name}, args...), "\x00") != strings.Join(wantAdd, "\x00") {
					t.Fatalf("worktree add args = %#v, want %#v", append([]string{name}, args...), wantAdd)
				}
			}
			if name == "git" && argsContain(args, "worktree") && argsContain(args, "remove") {
				removeCalled = true
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	_, err = svc.CreateSession(context.Background(), CreateSessionInput{
		Root:       root,
		Branch:     branch,
		BranchMode: BranchModeOpenExisting,
	})
	if err == nil {
		t.Fatal("CreateSession() error = nil, want tmux failure")
	}
	assertErrorContains(t, err, "create tmux session")
	if !removeCalled {
		t.Fatal("CreateSession() did not remove an owned worktree after tmux failure")
	}
}

func TestRefreshSessionPRPersistsCachedInfo(t *testing.T) {
	stateDir := t.TempDir()
	paths := Paths{StateDir: stateDir}
	store := NewStore(stateDir)
	session := Session{
		ID:              "session-1",
		Root:            "/repo",
		Workdir:         "/repo",
		RepoKey:         "repo",
		Branch:          "feature/pr-cache",
		TmuxSessionName: "oak-session-1",
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch name {
			case "git":
				return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
			case "gh":
				if argsContain(args, "api") {
					return []byte(`[{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":true}]}}}}}]`), nil
				}
				return []byte(`[{"number":7,"url":"https://github.com/oysandvik94/oak-tree/pull/7","title":"Test","state":"OPEN","isDraft":false,"reviewDecision":"APPROVED","updatedAt":"2026-06-25T12:00:00Z","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}]`), nil
			default:
				t.Fatalf("unexpected command: %s %#v", name, args)
				return nil, nil
			}
		},
	}
	svc := NewService(paths, store, runner)

	updated, err := svc.RefreshSessionPR(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PR == nil || !updated.PR.Found || updated.PR.Number != 7 || updated.PR.ChecksState != "pass" || updated.PR.ReviewDecision != "approved" || updated.PR.UnresolvedComments == nil || *updated.PR.UnresolvedComments != 1 || !updated.PR.UnresolvedCommentsChecked {
		t.Fatalf("RefreshSessionPR() PR = %#v, want cached PR #7 with one unresolved comment", updated.PR)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PR == nil || loaded.PR.Number != 7 || loaded.PR.UnresolvedComments == nil || *loaded.PR.UnresolvedComments != 1 || !loaded.PR.UnresolvedCommentsChecked {
		t.Fatalf("loaded PR = %#v, want persisted PR #7 with one unresolved comment", loaded.PR)
	}
}

func TestRefreshSessionPRDoesNotLockStoreDuringFetch(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	now := testTime()
	session := Session{
		ID:                   "session-pr-refresh",
		Root:                 "/repo",
		Workdir:              "/repo",
		Branch:               "feature/pr",
		AgentStatus:          AgentStatusAttention,
		AgentStatusUpdatedAt: &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	defer close(releaseFetch)
	runner := &stubRunner{outputFunc: func(name string, args []string) ([]byte, error) {
		switch {
		case name == "git" && argsContain(args, "--show-current"):
			return []byte("feature/pr\n"), nil
		case name == "git" && argsContain(args, "get-url"):
			return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
		case name == "gh":
			close(fetchStarted)
			<-releaseFetch
			return []byte(`[]`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}}
	svc := NewService(Paths{StateDir: stateDir}, store, runner)
	refreshDone := make(chan error, 1)
	go func() {
		_, err := svc.RefreshSessionPR(context.Background(), session.ID)
		refreshDone <- err
	}()

	select {
	case <-fetchStarted:
	case <-time.After(time.Second):
		t.Fatal("PR fetch did not start")
	}
	ackDone := make(chan error, 1)
	go func() {
		_, err := svc.AcknowledgeAgentAttention(context.Background(), session.ID)
		ackDone <- err
	}()
	select {
	case err := <-ackDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("attention acknowledgment blocked on PR fetch")
	}

	releaseFetch <- struct{}{}
	if err := <-refreshDone; err != nil {
		t.Fatal(err)
	}
	updated, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AgentStatus != AgentStatusIdle {
		t.Fatalf("AgentStatus = %q, want %q", updated.AgentStatus, AgentStatusIdle)
	}
}

func TestSetSessionTagTracksWhenParkedStateChanged(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	old := testTime()
	session := Session{ID: "session-tag-age", Tag: SessionTagWaitingReview, TagUpdatedAt: &old}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: stateDir}, store, &stubRunner{})

	unchanged, err := svc.SetSessionTag(context.Background(), session.ID, SessionTagWaitingReview)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.TagUpdatedAt == nil || !unchanged.TagUpdatedAt.Equal(old) {
		t.Fatalf("unchanged tag timestamp = %v, want %s", unchanged.TagUpdatedAt, old)
	}
	changed, err := svc.SetSessionTag(context.Background(), session.ID, SessionTagTesting)
	if err != nil {
		t.Fatal(err)
	}
	if changed.TagUpdatedAt == nil || !changed.TagUpdatedAt.After(old) {
		t.Fatalf("changed tag timestamp = %v, want after %s", changed.TagUpdatedAt, old)
	}
	active, err := svc.SetSessionTag(context.Background(), session.ID, SessionTagNone)
	if err != nil {
		t.Fatal(err)
	}
	if active.TagUpdatedAt != nil {
		t.Fatalf("active tag timestamp = %v, want nil", active.TagUpdatedAt)
	}
}

func TestAcknowledgeAgentAttentionClearsAttention(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	now := testTime()
	session := Session{
		ID:                   "session-attention",
		Root:                 "/repo",
		Workdir:              "/repo",
		RepoKey:              "repo",
		TmuxSessionName:      "oak-attention",
		AgentStatus:          AgentStatusAttention,
		AgentStatusUpdatedAt: &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: stateDir}, store, &stubRunner{})

	updated, err := svc.AcknowledgeAgentAttention(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AgentStatus != AgentStatusIdle {
		t.Fatalf("AgentStatus = %q, want %q", updated.AgentStatus, AgentStatusIdle)
	}
}

func TestListSessionsWithAgentStatusDetectsQuestionWhileWorking(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{
		ID:              "session-working-question",
		Root:            "/repo",
		Workdir:         "/repo",
		RepoKey:         "repo",
		TmuxSessionName: "oak-working-question",
		RightPaneID:     "%42",
		AgentStatus:     AgentStatusWorking,
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			if name == "tmux" && argsContain(args, "capture-pane") {
				return []byte(`Please provide your plan?`), nil
			}
			t.Fatalf("unexpected Output call: %s %#v", name, args)
			return nil, nil
		},
	}
	svc := NewService(Paths{StateDir: stateDir}, store, runner)

	sessions, err := svc.ListSessionsWithAgentStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	if sessions[0].AgentStatus != AgentStatusQuestion {
		t.Fatalf("AgentStatus = %q, want %q", sessions[0].AgentStatus, AgentStatusQuestion)
	}
}

func TestPreviewSessionRecoversStaleRightPaneID(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{
		ID:              "session-stale-pane",
		Root:            "/repo",
		Workdir:         "/repo",
		RepoKey:         "repo",
		TmuxSessionName: "oak-stale-pane",
		LeftPaneID:      "%left",
		RightPaneID:     "%old",
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			switch {
			case name == "tmux" && argsContain(args, "capture-pane") && argsContain(args, "%old"):
				return nil, errors.New("can't find pane: %old")
			case name == "tmux" && argsContain(args, "list-panes") && argsContain(args, "oak-stale-pane"):
				return []byte("%left\tnvim\t/repo\n%new\tnode\t/repo\n%other\tzsh\t/repo\n"), nil
			case name == "tmux" && argsContain(args, "capture-pane") && argsContain(args, "%new"):
				return []byte("fresh Pi output\n"), nil
			default:
				t.Fatalf("unexpected Output call: %s %#v", name, args)
				return nil, nil
			}
		},
	}
	svc := NewService(Paths{StateDir: stateDir}, store, runner)

	text, err := svc.PreviewSession(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if text != "fresh Pi output\n" {
		t.Fatalf("PreviewSession() = %q, want fresh Pi output", text)
	}
	updated, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RightPaneID != "%new" {
		t.Fatalf("RightPaneID = %q, want recovered pane %%new", updated.RightPaneID)
	}
}

func TestListSessionsWithAgentStatusNotifiesQuestionOnce(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{
		ID:              "session-question-notify",
		Root:            "/repo/project",
		Workdir:         "/repo/project",
		RepoKey:         "project",
		Branch:          "feature/question",
		TmuxSessionName: "oak-question-notify",
		RightPaneID:     "%42",
		AgentStatus:     AgentStatusWorking,
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	notifyCalls := 0
	runner := &recordingRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			if name == "tmux" && argsContain(args, "capture-pane") {
				return []byte("Please provide your choice?"), nil
			}
			t.Fatalf("unexpected Output call: %s %#v", name, args)
			return nil, nil
		},
		runFunc: func(name string, args []string) error {
			if name == "notify-send" {
				notifyCalls++
				if len(args) != 2 || args[0] != "Pi question waiting" {
					t.Fatalf("notify-send args = %#v", args)
				}
				if args[1] != "project · feature/question" {
					t.Fatalf("notify-send body = %q, want project and branch", args[1])
				}
				return nil
			}
			t.Fatalf("unexpected Run call: %s %#v", name, args)
			return nil
		},
	}
	svc := NewService(Paths{StateDir: stateDir}, store, runner)

	for i := 0; i < 2; i++ {
		sessions, err := svc.ListSessionsWithAgentStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if sessions[0].AgentStatus != AgentStatusQuestion {
			t.Fatalf("poll %d AgentStatus = %q, want %q", i, sessions[0].AgentStatus, AgentStatusQuestion)
		}
	}
	if notifyCalls != 1 {
		t.Fatalf("notifyCalls = %d, want 1", notifyCalls)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentStatus != AgentStatusQuestion {
		t.Fatalf("persisted AgentStatus = %q, want %q", loaded.AgentStatus, AgentStatusQuestion)
	}
}

func TestRefreshSessionPRUsesCurrentBranchOverPersistedBranch(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStore(stateDir)
	session := Session{ID: "session-branch-change", Root: "/repo", Workdir: "/repo", RepoKey: "repo", Branch: "stale", CreatedAt: testTime(), UpdatedAt: testTime()}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{outputFunc: func(name string, args []string) ([]byte, error) {
		if name == "git" && argsContain(args, "branch") {
			return []byte("current\n"), nil
		}
		if name == "git" && argsContain(args, "remote") {
			return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
		}
		if name == "gh" {
			if argsContain(args, "api") {
				return []byte(`[{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}]`), nil
			}
			if !argsContain(args, "current") {
				t.Fatalf("gh args = %#v, want current branch", args)
			}
			return []byte(`[{"number":10,"url":"https://github.com/oysandvik94/oak-tree/pull/10","state":"OPEN"}]`), nil
		}
		t.Fatalf("unexpected command: %s %#v", name, args)
		return nil, nil
	}}
	updated, err := NewService(Paths{StateDir: stateDir}, store, runner).RefreshSessionPR(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Branch != "current" || updated.PR == nil || updated.PR.Number != 10 {
		t.Fatalf("updated session = %#v, want current branch and PR #10", updated)
	}
}

func TestRefreshSessionPRInfersMissingBranch(t *testing.T) {
	stateDir := t.TempDir()
	paths := Paths{StateDir: stateDir}
	store := NewStore(stateDir)
	session := Session{
		ID:              "session-branchless",
		Root:            "/repo",
		Workdir:         "/repo",
		RepoKey:         "repo",
		TmuxSessionName: "oak-session-branchless",
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{
		outputFunc: func(name string, args []string) ([]byte, error) {
			if name == "git" && argsContain(args, "branch") && argsContain(args, "--show-current") {
				return []byte("feature/current\n"), nil
			}
			if name == "git" && argsContain(args, "remote") {
				return []byte("git@github.com:oysandvik94/oak-tree.git\n"), nil
			}
			if name == "gh" {
				if argsContain(args, "api") {
					return []byte(`[{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}]`), nil
				}
				if !argsContain(args, "feature/current") {
					t.Fatalf("gh args = %#v, want inferred branch", args)
				}
				return []byte(`[{"number":9,"url":"https://github.com/oysandvik94/oak-tree/pull/9","title":"Test","state":"OPEN","isDraft":false,"reviewDecision":"REVIEW_REQUIRED","updatedAt":"2026-06-25T12:00:00Z","statusCheckRollup":[]}]`), nil
			}
			t.Fatalf("unexpected command: %s %#v", name, args)
			return nil, nil
		},
	}
	svc := NewService(paths, store, runner)

	updated, err := svc.RefreshSessionPR(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Branch != "feature/current" {
		t.Fatalf("RefreshSessionPR() branch = %q, want inferred branch", updated.Branch)
	}
	if updated.PR == nil || updated.PR.Number != 9 {
		t.Fatalf("RefreshSessionPR() PR = %#v, want PR #9", updated.PR)
	}
}

func TestCloseSessionRemovesStateWhenTmuxSessionIsMissing(t *testing.T) {
	stateDir := t.TempDir()
	paths := Paths{StateDir: stateDir}
	store := NewStore(stateDir)
	session := Session{
		ID:              "stale-session",
		Root:            "/repo",
		Workdir:         "/repo",
		RepoKey:         "repo",
		TmuxSessionName: "oak-stale",
		CreatedAt:       testTime(),
		UpdatedAt:       testTime(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	killCalled := false
	runner := &stubRunner{
		runFunc: func(name string, args []string) error {
			if name == "tmux" && argsContain(args, "has-session") {
				return errors.New("can't find session")
			}
			if name == "tmux" && argsContain(args, "kill-session") {
				killCalled = true
			}
			return nil
		},
	}
	svc := NewService(paths, store, runner)

	if err := svc.CloseSession(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if killCalled {
		t.Fatal("CloseSession() called kill-session for a missing tmux session")
	}
	if _, err := os.Stat(SessionFilePath(stateDir, session.ID)); !os.IsNotExist(err) {
		t.Fatalf("session file still exists or unexpected stat error: %v", err)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func testTime() time.Time {
	return time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
}

func argsContain(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func assertRecordedCall(t *testing.T, calls [][]string, want []string) {
	t.Helper()
	wantJoined := strings.Join(want, "\x00")
	for _, call := range calls {
		if strings.Join(call, "\x00") == wantJoined {
			return
		}
	}
	t.Fatalf("recorded calls did not include %#v; got %#v", want, calls)
}
