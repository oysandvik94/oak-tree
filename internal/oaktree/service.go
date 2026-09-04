package oaktree

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BranchMode string

const (
	BranchModeCreateNew    BranchMode = "create_new"
	BranchModeOpenExisting BranchMode = "open_existing"
)

type CreateSessionInput struct {
	Root       string
	Branch     string
	BranchMode BranchMode
}

type Service struct {
	Paths Paths
	Store *Store
	Exec  Runner
}

func NewService(paths Paths, store *Store, exec Runner) *Service {
	return &Service{Paths: paths, Store: store, Exec: exec}
}

func (s *Service) ListSessions(ctx context.Context) ([]Session, error) {
	sessions, err := s.Store.ListSessions()
	if err != nil {
		return nil, err
	}
	updated, err := s.withGitStatus(ctx, sessions)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) withGitStatus(ctx context.Context, sessions []Session) ([]Session, error) {
	for i := range sessions {
		status := s.sessionGitStatus(ctx, sessions[i])
		sessions[i].GitStatus = status
		if status == nil || status.Branch == "" || status.Branch == sessions[i].Branch {
			continue
		}
		branch := status.Branch
		sessions[i].Branch = branch
		sessions[i].PR = nil
		if err := s.Store.UpdateSession(sessions[i].ID, func(session *Session) error {
			session.Branch = branch
			session.PR = nil
			return nil
		}); err != nil {
			return nil, fmt.Errorf("reconcile session %s branch: %w", sessions[i].ID, err)
		}
	}
	return sessions, nil
}

func (s *Service) sessionGitStatus(ctx context.Context, session Session) *GitStatus {
	if s == nil || s.Exec == nil {
		return nil
	}
	workdir := strings.TrimSpace(session.Workdir)
	if workdir == "" {
		workdir = strings.TrimSpace(session.Root)
	}
	if workdir == "" {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	status, err := WorktreeGitStatus(statusCtx, s.Exec, workdir, time.Now().UTC())
	if err == nil {
		return status
	}
	return &GitStatus{
		RefreshedAt: time.Now().UTC(),
		Error:       err.Error(),
	}
}

func (s *Service) CreateSession(ctx context.Context, input CreateSessionInput) (Session, error) {
	root, err := filepath.Abs(input.Root)
	if err != nil {
		return Session{}, err
	}
	if _, err := os.Stat(root); err != nil {
		return Session{}, err
	}
	repoKey, err := RepoKeyFromRoot(root)
	if err != nil {
		return Session{}, err
	}
	branchMode, err := normalizeBranchMode(input.BranchMode)
	if err != nil {
		return Session{}, err
	}
	workdir := root
	ownedWorktree := false
	branch := strings.TrimSpace(input.Branch)
	if branch != "" {
		switch branchMode {
		case BranchModeCreateNew:
			worktreePath, err := CreateWorktree(ctx, s.Exec, s.Paths, root, branch)
			if err != nil {
				return Session{}, createSessionStepError(root, fmt.Sprintf("add worktree for branch %s", branch), err)
			}
			workdir = worktreePath
			ownedWorktree = true
		case BranchModeOpenExisting:
			worktreePath, owned, err := OpenExistingWorktree(ctx, s.Exec, s.Paths, root, branch)
			if err != nil {
				return Session{}, createSessionStepError(root, fmt.Sprintf("open existing branch %s", branch), err)
			}
			workdir = worktreePath
			ownedWorktree = owned
		default:
			return Session{}, createSessionStepError(root, fmt.Sprintf("create session for branch %s", branch), fmt.Errorf("unsupported branch mode %q", input.BranchMode))
		}
	}
	id, err := newSessionID()
	if err != nil {
		return Session{}, err
	}
	sessionName := tmuxSessionName(root, branch, id)
	now := time.Now().UTC()
	session := Session{ID: id, Root: root, Workdir: workdir, RepoKey: repoKey, Branch: branch, OwnedWorktree: ownedWorktree, TmuxSessionName: sessionName, CreatedAt: now, UpdatedAt: now}
	// Pi may emit session_start as soon as the pane starts, so make identity
	// resolvable before launching tmux. The record is removed on any failure.
	if err := s.Store.SaveSession(session); err != nil {
		if ownedWorktree {
			_ = RemoveWorktree(ctx, s.Exec, root, workdir)
		}
		return Session{}, createSessionStepError(root, fmt.Sprintf("save session state for %s", sessionName), err)
	}
	rightCommand, err := PiCommand(ctx, s.Paths, id)
	if err != nil {
		_ = os.Remove(SessionFilePath(s.Paths.StateDir, id))
		if ownedWorktree {
			_ = RemoveWorktree(ctx, s.Exec, root, workdir)
		}
		return Session{}, createSessionStepError(root, "prepare Pi", err)
	}
	tmuxSession, err := CreateTmuxSessionWithCommand(ctx, s.Exec, sessionName, workdir, rightCommand)
	if err != nil {
		if ownedWorktree {
			_ = RemoveWorktree(ctx, s.Exec, root, workdir)
		}
		_ = os.Remove(SessionFilePath(s.Paths.StateDir, id))
		return Session{}, createSessionStepError(root, fmt.Sprintf("create tmux session %s in %s", sessionName, workdir), err)
	}
	session.TmuxSessionName, session.TmuxSessionID = tmuxSession.Name, tmuxSession.ID
	session.LeftPaneID, session.RightPaneID, session.UpdatedAt = tmuxSession.LeftPaneID, tmuxSession.RightPaneID, time.Now().UTC()
	if err := s.Store.UpdateSession(session.ID, func(stored *Session) error {
		stored.TmuxSessionName, stored.TmuxSessionID = session.TmuxSessionName, session.TmuxSessionID
		stored.LeftPaneID, stored.RightPaneID, stored.UpdatedAt = session.LeftPaneID, session.RightPaneID, session.UpdatedAt
		return nil
	}); err != nil {
		_ = KillTmuxSession(ctx, s.Exec, tmuxSession.Name)
		_ = os.Remove(SessionFilePath(s.Paths.StateDir, id))
		if ownedWorktree {
			_ = RemoveWorktree(ctx, s.Exec, root, workdir)
		}
		return Session{}, createSessionStepError(root, fmt.Sprintf("save session state for %s", sessionName), err)
	}
	updated, err := s.Store.LoadSession(session.ID)
	if err != nil {
		return Session{}, err
	}
	return updated, nil
}

func tmuxSessionName(root, branch, id string) string {
	parts := []string{"oak", SafeComponent(filepath.Base(root))}
	if branch != "" {
		parts = append(parts, SafeComponent(branch))
	}
	return strings.Join(append(parts, id[:6]), "-")
}

func normalizeBranchMode(mode BranchMode) (BranchMode, error) {
	switch normalized := BranchMode(strings.ToLower(strings.TrimSpace(string(mode)))); normalized {
	case "", BranchModeCreateNew:
		return BranchModeCreateNew, nil
	case BranchModeOpenExisting:
		return BranchModeOpenExisting, nil
	default:
		return "", fmt.Errorf("unknown branch mode %q", mode)
	}
}

func normalizeAgentStatus(status AgentStatus) (AgentStatus, error) {
	switch normalized := AgentStatus(strings.ToLower(strings.TrimSpace(string(status)))); normalized {
	case AgentStatusUnknown:
		return AgentStatusUnknown, nil
	case AgentStatusWorking:
		return AgentStatusWorking, nil
	case AgentStatusAttention:
		return AgentStatusAttention, nil
	case AgentStatusQuestion:
		return AgentStatusQuestion, nil
	case AgentStatusIdle:
		return AgentStatusIdle, nil
	default:
		return "", fmt.Errorf("unknown agent status %q", status)
	}
}

func normalizeSessionTag(tag SessionTag) (SessionTag, error) {
	switch normalized := SessionTag(strings.ToLower(strings.TrimSpace(string(tag)))); normalized {
	case SessionTagNone:
		return SessionTagNone, nil
	case SessionTagWaitingReview:
		return SessionTagWaitingReview, nil
	case SessionTagBlocked:
		return SessionTagBlocked, nil
	default:
		return "", fmt.Errorf("unknown session tag %q", tag)
	}
}

func (s *Service) SetSessionTag(ctx context.Context, id string, tag SessionTag) (Session, error) {
	normalized, err := normalizeSessionTag(tag)
	if err != nil {
		return Session{}, err
	}
	err = s.Store.UpdateSession(id, func(session *Session) error {
		if session.Tag == normalized {
			return nil
		}
		session.Tag = normalized
		if normalized == SessionTagNone {
			session.TagUpdatedAt = nil
		} else {
			now := time.Now().UTC()
			session.TagUpdatedAt = &now
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.Store.LoadSession(id)
}

func (s *Service) SetSessionNote(ctx context.Context, id, note string) (Session, error) {
	err := s.Store.UpdateSession(id, func(session *Session) error {
		session.Note = strings.TrimSpace(note)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.Store.LoadSession(id)
}

func (s *Service) AcknowledgeAgentAttention(ctx context.Context, id string) (Session, error) {
	err := s.Store.UpdateSession(id, func(session *Session) error {
		if session.AgentStatus != AgentStatusAttention && session.AgentStatus != AgentStatusQuestion {
			return nil
		}
		now := time.Now().UTC()
		session.AgentStatus = AgentStatusIdle
		session.AgentStatusUpdatedAt = &now
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.Store.LoadSession(id)
}

func (s *Service) CloseSession(ctx context.Context, id string) error {
	return s.CloseSessionWithFallback(ctx, id, "")
}

func (s *Service) CloseSessionWithFallback(ctx context.Context, id, fallbackTmuxSession string) error {
	session, err := s.Store.FindSessionByID(id)
	if err != nil {
		return err
	}
	if session.OwnedWorktree {
		dirty, err := IsDirtyWorktree(ctx, s.Exec, session.Workdir)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("refusing to remove dirty worktree %s", session.Workdir)
		}
	}
	if HasTmuxSession(ctx, s.Exec, session.TmuxSessionName) {
		current, err := CurrentTmuxSession(ctx, s.Exec)
		if err != nil {
			return fmt.Errorf("detect current tmux session: %w", err)
		}
		if current == session.TmuxSessionName && fallbackTmuxSession != "" {
			if !HasTmuxSession(ctx, s.Exec, fallbackTmuxSession) {
				return fmt.Errorf("fallback tmux session %s is unavailable", fallbackTmuxSession)
			}
			if err := SwitchTmuxSession(ctx, s.Exec, fallbackTmuxSession); err != nil {
				return fmt.Errorf("switch to fallback tmux session %s: %w", fallbackTmuxSession, err)
			}
		}
		if err := KillTmuxSession(ctx, s.Exec, session.TmuxSessionName); err != nil {
			return err
		}
	}
	if session.OwnedWorktree {
		if err := RemoveWorktree(ctx, s.Exec, session.Root, session.Workdir); err != nil {
			return err
		}
	}
	return os.Remove(SessionFilePath(s.Paths.StateDir, session.ID))
}

func (s *Service) PreviewSession(ctx context.Context, session Session) (string, error) {
	if session.RightPaneID != "" {
		text, err := CapturePane(ctx, s.Exec, session.RightPaneID, 120)
		if err == nil {
			return text, nil
		}
		if target := s.resolvePreviewPane(ctx, session); target != "" {
			text, resolvedErr := CapturePane(ctx, s.Exec, target, 120)
			if resolvedErr == nil {
				s.rememberRightPaneID(session.ID, target)
				return text, nil
			}
		}
		if session.LeftPaneID == "" {
			return "", err
		}
	}
	if session.LeftPaneID != "" {
		return CapturePane(ctx, s.Exec, session.LeftPaneID, 120)
	}
	if target := s.resolvePreviewPane(ctx, session); target != "" {
		text, err := CapturePane(ctx, s.Exec, target, 120)
		if err == nil {
			s.rememberRightPaneID(session.ID, target)
			return text, nil
		}
		return "", err
	}
	if session.RightPaneID == "" {
		return "", errors.New("session has no tmux panes")
	}
	return "", errors.New("session preview pane unavailable")
}

func (s *Service) resolvePreviewPane(ctx context.Context, session Session) string {
	if session.TmuxSessionName == "" {
		return ""
	}
	panes, err := ListPanes(ctx, s.Exec, session.TmuxSessionName)
	if err != nil {
		return ""
	}
	for _, pane := range panes {
		if pane.ID == "" || pane.ID == session.LeftPaneID {
			continue
		}
		if pane.Path != "" && session.Workdir != "" && pane.Path != session.Workdir {
			continue
		}
		if pane.Command == "pi" || pane.Command == "node" || pane.Command == "env" {
			return pane.ID
		}
	}
	return ""
}

func (s *Service) rememberRightPaneID(sessionID, paneID string) {
	if s.Store == nil || sessionID == "" || paneID == "" {
		return
	}
	_ = s.Store.UpdateSession(sessionID, func(session *Session) error {
		session.RightPaneID = paneID
		return nil
	})
}

func (s *Service) AttachSession(ctx context.Context, session Session) error {
	return AttachOrSwitch(ctx, s.Exec, session.TmuxSessionName)
}

func (s *Service) RefreshSession(ctx context.Context, id string) (Session, error) {
	return s.Store.LoadSession(id)
}

func (s *Service) ListSessionsWithAgentStatus(ctx context.Context) ([]Session, error) {
	sessions, err := s.Store.ListSessions()
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i].AgentStatus != AgentStatusWorking &&
			sessions[i].AgentStatus != AgentStatusAttention &&
			sessions[i].AgentStatus != AgentStatusQuestion {
			continue
		}
		text, err := s.PreviewSession(ctx, sessions[i])
		if err != nil {
			continue
		}
		if sessions[i].AgentStatus == AgentStatusQuestion {
			// The extension's question tool is authoritative; Pi's UI text is
			// not guaranteed to resemble its question renderer.
			continue
		}
		if paneLooksLikeAgentQuestion(text) {
			if sessions[i].AgentStatus != AgentStatusQuestion {
				s.notifyAgentQuestion(ctx, sessions[i])
			}
			sessions[i].AgentStatus = AgentStatusQuestion
			continue
		}
		if sessions[i].AgentStatus == AgentStatusQuestion {
			status := staleQuestionFallbackStatus(text)
			sessions[i].AgentStatus = status
			s.clearStaleAgentQuestion(ctx, sessions[i], status)
		}
	}
	return sessions, nil
}

func (s *Service) RefreshSessionPR(ctx context.Context, id string) (Session, error) {
	session, err := s.Store.LoadSession(id)
	if err != nil {
		return Session{}, err
	}
	branch := strings.TrimSpace(session.Branch)
	workdir := strings.TrimSpace(session.Workdir)
	if workdir == "" {
		workdir = strings.TrimSpace(session.Root)
	}
	current, currentErr := CurrentBranch(ctx, s.Exec, workdir)
	if currentErr == nil && strings.TrimSpace(current) != "" {
		branch = strings.TrimSpace(current)
	} else if branch == "" && currentErr != nil {
		return Session{}, currentErr
	}
	info, err := RefreshPullRequestInfo(ctx, s.Exec, workdir, branch, time.Now())
	if err != nil {
		return Session{}, err
	}
	err = s.Store.UpdateSession(id, func(session *Session) error {
		if branch != "" {
			session.Branch = branch
		}
		session.PR = info
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.Store.LoadSession(id)
}

func (s *Service) OpenSessionPR(ctx context.Context, session Session) error {
	if session.PR == nil {
		return errors.New("PR status has not been refreshed")
	}
	if !session.PR.Found || strings.TrimSpace(session.PR.URL) == "" {
		return errors.New("no PR cached for selected session")
	}
	return s.Exec.Run(ctx, "gh", "pr", "view", session.PR.URL, "--web")
}

func paneLooksLikeAgentQuestion(text string) bool {
	// Pi's question tool is authoritative; this detector remains a fallback for
	// ordinary prose questions and third-party extensions.
	lower := strings.ToLower(recentPaneText(text, 20))
	return strings.Contains(lower, "?") && (strings.Contains(lower, "please provide") || strings.Contains(lower, "waiting for your") || strings.Contains(lower, "your input"))
}

func recentPaneText(text string, maxLines int) string {
	if maxLines <= 0 {
		return text
	}
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func staleQuestionFallbackStatus(text string) AgentStatus {
	lower := strings.ToLower(recentPaneText(text, 12))
	if strings.Contains(lower, "· working") || strings.Contains(lower, " working ·") {
		return AgentStatusWorking
	}
	return AgentStatusAttention
}

func (s *Service) clearStaleAgentQuestion(ctx context.Context, session Session, status AgentStatus) {
	now := time.Now().UTC()
	_ = s.Store.UpdateSession(session.ID, func(stored *Session) error {
		if stored.AgentStatus != AgentStatusQuestion {
			return nil
		}
		stored.AgentStatus = status
		stored.AgentStatusUpdatedAt = &now
		return nil
	})
}

func (s *Service) notifyAgentQuestion(ctx context.Context, session Session) {
	now := time.Now().UTC()
	shouldNotify := false
	err := s.Store.UpdateSession(session.ID, func(stored *Session) error {
		if stored.AgentStatus == AgentStatusQuestion {
			return nil
		}
		stored.AgentStatus = AgentStatusQuestion
		stored.AgentStatusUpdatedAt = &now
		shouldNotify = true
		return nil
	})
	if err != nil || !shouldNotify {
		return
	}
	_ = s.Exec.Run(ctx, "notify-send", "Pi question waiting", agentQuestionNotificationBody(session))
}

func agentQuestionNotificationBody(session Session) string {
	root := strings.TrimSpace(session.Root)
	if root == "" {
		root = strings.TrimSpace(session.Workdir)
	}
	project := filepath.Base(filepath.Clean(root))
	if project == "." || project == string(filepath.Separator) || project == "" {
		project = strings.TrimSpace(root)
	}
	if branch := strings.TrimSpace(session.Branch); branch != "" {
		project += " · " + branch
	}
	return project
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func createSessionStepError(root, step string, err error) error {
	return fmt.Errorf("create session for %s: %s: %w", root, step, err)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func hookExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return stableHookExecutable(exe, osexec.LookPath)
}

func stableHookExecutable(exe string, lookPath func(string) (string, error)) (string, error) {
	if !looksLikeGoRunExecutable(exe) {
		return exe, nil
	}
	path, err := lookPath(filepath.Base(exe))
	if err == nil {
		if abs, absErr := filepath.Abs(path); absErr == nil {
			path = abs
		}
		if path != exe {
			return path, nil
		}
	}
	return "", fmt.Errorf("oak-tree was run from temporary go run binary %s; run go install . first", exe)
}

func looksLikeGoRunExecutable(path string) bool {
	path = filepath.ToSlash(path)
	return strings.Contains(path, "/go-build") && strings.Contains(path, "/exe/")
}
