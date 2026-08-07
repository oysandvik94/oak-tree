package oaktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func DetermineDefaultBranch(ctx context.Context, runner Runner, root string) (string, error) {
	data, err := runner.Output(ctx, "git", "-C", root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		value := strings.TrimSpace(string(data))
		if idx := strings.LastIndex(value, "/"); idx >= 0 {
			value = value[idx+1:]
		}
		if value != "" {
			return value, nil
		}
	}
	data, err = runner.Output(ctx, "git", "-C", root, "remote", "show", "origin")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "HEAD branch:") {
			branch := strings.TrimSpace(strings.TrimPrefix(line, "HEAD branch:"))
			if branch != "" {
				return branch, nil
			}
		}
	}
	return "", errors.New("unable to determine default branch")
}

func FetchBranch(ctx context.Context, runner Runner, root, branch string) error {
	refspec := "+refs/heads/" + branch + ":refs/remotes/origin/" + branch
	return runner.Run(ctx, "git", "-C", root, "fetch", "--prune", "origin", refspec)
}

func BranchExists(ctx context.Context, runner Runner, root, branch string) bool {
	return runner.Run(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) == nil
}

func RemoteBranchExists(ctx context.Context, runner Runner, root, branch string) (bool, error) {
	data, err := runner.Output(ctx, "git", "-C", root, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "refs/heads/"+branch {
			return true, nil
		}
	}
	return false, nil
}

func ConfigureBranchUpstream(ctx context.Context, runner Runner, root, branch, remote string) error {
	if err := runner.Run(ctx, "git", "-C", root, "config", "branch."+branch+".remote", remote); err != nil {
		return err
	}
	return runner.Run(ctx, "git", "-C", root, "config", "branch."+branch+".merge", "refs/heads/"+branch)
}

func CurrentBranch(ctx context.Context, runner Runner, workdir string) (string, error) {
	data, err := runner.Output(ctx, "git", "-C", workdir, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func CreateWorktree(ctx context.Context, runner Runner, paths Paths, root, branch string) (string, error) {
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		return "", err
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("worktree already exists: %s", worktreePath)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := ensurePrivateWorktreeParent(paths, worktreePath); err != nil {
		return "", err
	}
	defaultBranch, err := DetermineDefaultBranch(ctx, runner, root)
	if err != nil {
		return "", fmt.Errorf("determine default branch: %w", err)
	}
	if err := FetchBranch(ctx, runner, root, defaultBranch); err != nil {
		return "", fmt.Errorf("fetch default branch %s: %w", defaultBranch, err)
	}
	args := []string{"-C", root, "worktree", "add"}
	branchExists := BranchExists(ctx, runner, root, branch)
	if !branchExists {
		args = append(args, "-b", branch)
	}
	startPoint := branch
	if !branchExists {
		args = append(args, "--no-track")
		startPoint = "origin/" + defaultBranch
	}
	args = append(args, worktreePath, startPoint)
	if err := runner.Run(ctx, "git", args...); err != nil {
		return "", fmt.Errorf("add worktree for branch %s: %w", branch, err)
	}
	if err := ConfigureBranchUpstream(ctx, runner, root, branch, "origin"); err != nil {
		_ = RemoveWorktree(ctx, runner, root, worktreePath)
		return "", fmt.Errorf("configure upstream for branch %s: %w", branch, err)
	}
	return worktreePath, nil
}

func ensurePrivateWorktreeParent(paths Paths, worktreePath string) error {
	for _, dir := range []string{paths.StateDir, filepath.Join(paths.StateDir, "worktrees"), filepath.Dir(worktreePath)} {
		if err := ensurePrivateDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func OpenExistingWorktree(ctx context.Context, runner Runner, paths Paths, root, branch string) (string, bool, error) {
	if existing, ok, err := WorktreeForBranch(ctx, runner, root, branch); err != nil {
		return "", false, err
	} else if ok {
		return existing, false, nil
	}

	localExists := BranchExists(ctx, runner, root, branch)
	originExists, err := RemoteBranchExists(ctx, runner, root, branch)
	if err != nil {
		if !localExists {
			return "", false, err
		}
		originExists = false
	}
	if !localExists && !originExists {
		return "", false, fmt.Errorf("branch %q does not exist locally or on origin", branch)
	}
	if originExists {
		if err := FetchBranch(ctx, runner, root, branch); err != nil && !localExists {
			return "", false, err
		}
	}

	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		return "", false, err
	}
	worktreePath := WorktreePath(paths.StateDir, repoKey, branch)
	if _, err := os.Stat(worktreePath); err == nil {
		return "", false, fmt.Errorf("worktree already exists: %s", worktreePath)
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := ensurePrivateWorktreeParent(paths, worktreePath); err != nil {
		return "", false, err
	}
	args := []string{"-C", root, "worktree", "add"}
	if localExists {
		args = append(args, worktreePath, branch)
	} else {
		args = append(args, "-b", branch, "--track", worktreePath, "origin/"+branch)
	}
	if err := runner.Run(ctx, "git", args...); err != nil {
		return "", false, err
	}
	return worktreePath, true, nil
}

func WorktreeForBranch(ctx context.Context, runner Runner, root, branch string) (string, bool, error) {
	data, err := runner.Output(ctx, "git", "-C", root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, err
	}
	worktree, ok := parseWorktreePorcelainForBranch(string(data), branch)
	return worktree, ok, nil
}

func parseWorktreePorcelainForBranch(data, branch string) (string, bool) {
	wantLocal := "refs/heads/" + branch
	wantRemote := "refs/remotes/origin/" + branch
	var currentWorktree string
	var currentBranch string
	flush := func() (string, bool) {
		if currentWorktree == "" || currentBranch == "" {
			return "", false
		}
		if currentBranch == wantLocal || currentBranch == wantRemote {
			return currentWorktree, true
		}
		return "", false
	}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if worktree, ok := flush(); ok {
				return worktree, true
			}
			currentWorktree = ""
			currentBranch = ""
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentWorktree = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			currentBranch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
		}
	}
	if worktree, ok := flush(); ok {
		return worktree, true
	}
	return "", false
}

func IsDirtyWorktree(ctx context.Context, runner Runner, workdir string) (bool, error) {
	data, err := runner.Output(ctx, "git", "-C", workdir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(data)) != "", nil
}

func WorktreeGitStatus(ctx context.Context, runner Runner, workdir string, now time.Time) (*GitStatus, error) {
	data, err := runner.Output(ctx, "git", "-C", workdir, "status", "--branch", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	status := parseGitStatusPorcelain(string(data))
	status.RefreshedAt = now
	return &status, nil
}

func parseGitStatusPorcelain(data string) GitStatus {
	var status GitStatus
	for _, line := range strings.Split(strings.TrimRight(data, "\r\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			parseGitBranchStatus(line, &status)
			continue
		}
		if strings.HasPrefix(line, "??") {
			status.Untracked++
			continue
		}
		status.Changed++
	}
	status.Clean = status.Changed == 0 && status.Untracked == 0
	return status
}

func parseGitBranchStatus(line string, status *GitStatus) {
	if status == nil {
		return
	}
	branch := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if idx := strings.Index(branch, "..."); idx >= 0 {
		branch = branch[:idx]
	}
	if !strings.Contains(branch, " ") && branch != "HEAD" {
		status.Branch = branch
	}
	start := strings.Index(line, "[")
	end := strings.LastIndex(line, "]")
	if start < 0 || end <= start {
		return
	}
	for _, part := range strings.Split(line[start+1:end], ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			status.Ahead = value
		case "behind":
			status.Behind = value
		}
	}
}

func RemoveWorktree(ctx context.Context, runner Runner, root, worktree string) error {
	if err := runner.Run(ctx, "git", "-C", root, "worktree", "remove", worktree); err != nil {
		return err
	}
	return runner.Run(ctx, "git", "-C", root, "worktree", "prune")
}
