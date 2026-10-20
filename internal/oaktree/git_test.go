package oaktree

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestCreateWorktreeWithoutOrigin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
		current  string
		wantErr  bool
	}{
		{name: "new branch", current: "main"},
		{name: "existing branch", existing: true},
		{name: "detached HEAD", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			paths := Paths{StateDir: t.TempDir()}
			branch := "feature/local"
			runner := &recordingRunner{
				outputFunc: func(name string, args []string) ([]byte, error) {
					switch args[len(args)-1] {
					case "remote":
						return nil, nil
					case "--show-current":
						return []byte(tc.current + "\n"), nil
					default:
						t.Fatalf("unexpected output: %s %v", name, args)
						return nil, nil
					}
				},
				runFunc: func(name string, args []string) error {
					if argsContain(args, "fetch") || argsContain(args, "config") {
						t.Fatalf("unexpected remote operation: %s %v", name, args)
					}
					if argsContain(args, "rev-parse") && !tc.existing {
						return errors.New("branch does not exist")
					}
					if tc.wantErr && argsContain(args, "worktree") {
						t.Fatal("must not create a worktree from detached HEAD")
					}
					return nil
				},
			}
			got, err := CreateWorktree(context.Background(), runner, paths, root, branch)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected detached HEAD error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			repoKey, err := RepoKeyFromRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			want := WorktreePath(paths.StateDir, repoKey, branch)
			if filepath.Clean(got) != filepath.Clean(want) {
				t.Fatalf("worktree path = %q, want %q", got, want)
			}
			args := []string{"git", "-C", root, "worktree", "add"}
			start := branch
			if !tc.existing {
				args = append(args, "-b", branch, "--no-track")
				start = tc.current
			}
			assertRecordedCall(t, runner.calls, append(args, want, start))
		})
	}
}
