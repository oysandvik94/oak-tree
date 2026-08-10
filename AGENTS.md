# oak-tree Agent Notes

## Project Purpose

`oak-tree` is a Go TUI for managing AI coding sessions in tmux. The default UI is a Bubble Tea dashboard intended to run inside a tmux popup. Each oak-tree session owns one tmux session and one Pi session.

## Current V1 Contract

- The dashboard shows oak-tree-managed sessions only.
- New sessions open one tmux window split into two vertical panes:
  - left (60%): Pi with the oak-tree extension
  - right (40%): `nvim`
- New sessions accept a root directory and optional branch.
- If a branch is supplied, oak-tree fetches the latest default branch and creates a git worktree under oak-tree state.
- Closing a session kills the tmux session and removes oak-tree-owned worktrees, but keeps the local branch.
- Pi sessions use a state-owned extension and `oak-tree hook agent-event`. The extension also forwards `@juicesharp/rpiv-todo` task snapshots from persisted `todo` tool results.
- Session JSON stores generic `agent_session_ids` and may include a one-line `note`; kanban cards show a highlighted note icon and `e` opens the note popup.
- Dashboard root selection uses `~/.config/oak-tree/config.toml`: `root_search_dirs` contributes immediate child directories, while `roots` contributes exact directories.
- Dashboard agent states are `QUESTION`, `WORKING`, and `READY`. Manually tagged `REVIEW` and `TESTING` sessions are excluded from the active count and grouped in separate parked sections below active sessions.
- Pull request metadata is cached on session JSON. Dashboard startup may queue background refreshes for missing or stale branch-backed session PR caches; explicit `p` refresh still force-refreshes the selected session. An actual PR on the selected session gets a two-line inspector above the key footer with lifecycle, CI, approval, unresolved comments, and contextual open/refresh commands.
- Pi todo summaries appear in the wide session table; `space` expands or collapses the selected session's persisted task subjects inline.

## Development Commands

Use these commands from the repository root:

```sh
go test ./...
go run . --help
go run . popup
go run . new --root /path/to/repo
go run . new --root /path/to/repo --branch feature/example
```

Run formatting before finalizing Go changes:

```sh
gofmt -w .
```

## State Paths

By default, runtime state lives under:

```text
~/.local/state/oak-tree/
```

Expected subdirectories:

```text
sessions/   persisted oak-tree session records
worktrees/  oak-tree-created git worktrees
logs/       JSONL command diagnostics
```

Session records may include an optional `pr` block with cached GitHub PR state, an optional manual `tag` such as `waiting_review` or `testing`, and an optional Pi `todo` summary.

Do not store transient runtime state in the repository.

User configuration lives at:

```text
~/.config/oak-tree/config.toml
```

The v1 config schema includes:

```toml
root_search_dirs = [
  "~/dev/general",
  "~/work",
]
roots = [
  "~/src/standalone-project",
]
```

## Documentation Rule

When changing lifecycle behavior, command behavior, state schema, tmux layout, update the relevant file in `docs/` in the same change.

## Safety Notes

- Do not delete local branches on close unless the user explicitly asks for that feature.
- Refuse to remove dirty worktrees by default.
- Keep tmux and git command execution behind small interfaces where possible so behavior can be tested without requiring a live tmux server.
- Keep `gh` calls behind the service/runner layer; dashboard initialization may only queue background PR refreshes through that layer for missing or stale branch-backed session caches.
