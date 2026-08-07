# Session Lifecycle

## Create Without Branch

1. User selects a root directory from the dashboard fuzzy picker, enters it manually, or passes it with `oak-tree new --root`.
2. oak-tree validates that the directory exists.
3. oak-tree uses the root directory as the active working directory.
4. oak-tree creates a tmux session, reads back the initial pane id from `tmux new-session`, and splits that pane into two vertical panes.
5. The left pane starts Pi with the state-owned extension at 60% width.
6. The right pane runs `nvim` at 40% width.
7. oak-tree records the session, pane ids, root, and active working directory.

## Create With Branch

1. User selects or passes a root directory and branch name in create-new mode.
2. oak-tree validates that the root is a git repository.
3. oak-tree detects the default branch for `origin`.
4. oak-tree runs a fetch for the latest default branch.
5. oak-tree creates a git worktree under:

   ```text
   ~/.local/state/oak-tree/worktrees/<repo-name>/<safe-branch-name>
   ```

6. oak-tree starts the same two-pane tmux layout in the worktree directory.
7. For a new local branch, oak-tree creates it from `origin/<default-branch>` with `--no-track`.
8. oak-tree configures the branch to push to `origin/<branch-name>` instead of tracking the default branch it was created from. This also repairs an existing local branch used through create-new mode if it still has the old default-branch upstream.
9. oak-tree records the worktree path as owned by the session.

## Open Existing Branch

1. User selects a root directory, enters a branch name, and toggles the dashboard branch mode to open existing.
2. oak-tree checks `git worktree list --porcelain` to see whether the branch is already checked out in another worktree.
3. If it is checked out, oak-tree starts the tmux session in that worktree and records it as not owned by oak-tree.
4. If it is not checked out, oak-tree checks whether the branch exists locally or on `origin`.
5. If the branch exists locally, oak-tree creates a managed worktree from the local branch.
6. If only `origin/<branch-name>` exists, oak-tree fetches it and creates a local tracking branch in a managed worktree.
7. If the branch exists neither locally nor on `origin`, oak-tree fails before starting tmux.
8. If tmux startup fails, oak-tree removes only worktrees it owns.

## Dashboard Root Picker

The dashboard create flow reads:

```text
~/.config/oak-tree/config.toml
```

Example:

```toml
root_search_dirs = [
  "~/dev/general",
  "~/work",
]
roots = [
  "~/src/standalone-project",
]
```

For each configured search directory, oak-tree lists immediate child directories as root candidates. Directories configured in `roots` are added as exact candidates. The picker fuzzy-filters the combined list, and missing configured directories are ignored.

The CLI path remains explicit:

```sh
oak-tree new --root /path/to/repo
```

CLI creation does not require or consult `root_search_dirs` or `roots`.

## Pi lifecycle

Pi is launched with the state-owned extension and receives `OAK_TREE_SESSION_ID`; the extension sends lifecycle events to `oak-tree hook agent-event`, including Pi's session id and session file. This keeps `/new`, `/resume`, and `/fork` linked to the same oak-tree session. The extension consumes `@juicesharp/rpiv-ask-user-question`'s `rpiv:ask-user:prompt` event and observes that tool's completion. When `ask_user_question` is unavailable, it registers a simpler `question` fallback. Answering or cancelling either UI restores `working`. `agent_settled` is represented as `attention` and sends a best-effort desktop notification; shutdown is `idle`. Ordinary prose prompts still use the pane heuristic as a fallback.

When `@juicesharp/rpiv-todo` is loaded, the same extension reads the latest persisted `todo` tool-result details during `session_start` and after each successful `todo` execution. It sends visible task subjects, statuses, and counts through the agent-event hook. Deleted tasks are ignored; invalid count summaries are rejected at the hook boundary.

The extension also observes `pi-subagents` tool progress and async lifecycle events. It stores the current foreground and background team size on the oak-tree session, resets the count on shutdown, and rejects negative counts at the hook boundary.

## Dashboard Session Table

The dashboard is a full-width, dense session table optimized for session management. Each row shows agent state, project/session, branch, Git state, cached PR state, Pi todo progress on wide layouts, and usage where available. Kanban cards additionally show ` N` while a Pi session has a reported subagent team. Todo progress uses completed/total counts with distinct in-progress, pending, and complete chips; `space` expands or collapses the selected session's task subjects inline. Summary counters highlight sessions needing attention, working, ready, waiting review, and testing. Manually tagged review and testing sessions are excluded from the active count and grouped in separate parked sections below active sessions. The selected row is highlighted; `enter` attaches to the actual tmux workspace.

The dashboard no longer renders a live agent-pane preview or polls tmux capture output. Agent pane capture remains available to lifecycle/status inference and service-level integrations, but it is not part of the dashboard rendering loop.

## Pull Request Metadata

The dashboard reads cached pull request metadata from each session record immediately. After sessions load, it refreshes missing, stale, or pre-unresolved-comment PR metadata for branch-backed sessions in the background. A PR cache is stale after ten minutes.

For automatic refresh:

1. oak-tree uses the current worktree branch when available and falls back to the recorded branch if lookup fails. Git-status refresh persists a changed observed branch and clears the stale PR cache.
2. oak-tree reads the GitHub repository from the workdir's `origin` remote.
3. oak-tree calls `gh pr list` for that repository and branch.
4. oak-tree summarizes review state and status checks, then uses a paginated GitHub GraphQL query to count unresolved review threads.
5. oak-tree writes the result back to the session JSON. If the supplemental thread query fails, the other PR metadata is still cached and the unresolved-comment count is shown as unavailable until the next normal refresh.

When the selected session has an actual cached PR, the dashboard renders a two-line inspector above the global key footer with its number, title, cache age, draft/ready state, CI, approval, and unresolved-comment status. The inspector contains the contextual `o` open and `p` refresh command hints and remains hidden when the selected session has no PR. `o` opens the cached PR URL with `gh pr view --web`; `p` force-refreshes its metadata.

## Dashboard Git Status

The dashboard reads worktree and upstream state from each session workdir with `git status --branch --porcelain=v1 --untracked-files=all`. It shows inline session-title status: `clean`, `changes`, `unpushed ↑`, `changes ↑`, or `git?`. The branch in that header is also used to reconcile the session record and invalidate stale PR metadata. Git status is refreshed when sessions load and on a slower background timer.

## Close

Closing a session:

1. Confirms the selected session in the dashboard.
2. Kills the tmux session if it still exists.
3. If the session owns a worktree, checks for dirty changes.
4. Refuses cleanup for dirty worktrees by default.
5. Removes the worktree through git worktree commands.
6. Prunes stale git worktree metadata.
7. Marks the oak-tree session closed or removes the state file, depending on the implementation choice.

The local branch is kept.
