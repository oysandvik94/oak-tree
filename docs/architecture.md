# Architecture

## Overview

`oak-tree` is a Go/Bubble Tea dashboard for Pi coding sessions in tmux. Each managed session owns:

- a tmux session with Pi in the left 60% pane and `nvim` in the right 40% pane;
- an optional oak-tree-owned Git worktree; and
- a JSON session record under `~/.local/state/oak-tree/sessions/`.

## Components

- `cmd/`: Cobra commands for the dashboard, session lifecycle, and Pi event hook.
- `internal/oaktree/service.go`: session, tmux, worktree, status, PR, and notification orchestration.
- `internal/oaktree/pi.go`: the state-owned Pi extension and Pi launch command.
- `internal/oaktree/store.go`: atomic state persistence.
- `internal/oaktree/tui.go`: dashboard and create-flow UI.

## Configuration

`~/.config/oak-tree/config.toml` supplies only root discovery:

```toml
root_search_dirs = ["~/dev/general", "~/work"]
roots = ["~/src/standalone-project"]
```

Pi is the sole agent. Oak-tree writes its private extension to `<state>/pi/oak-tree-extension.ts` and launches Pi with `OAK_TREE_SESSION_ID` and `OAK_TREE_HOOK` set.

## Lifecycle

The extension sends events through:

```sh
oak-tree hook agent-event
```

It records Pi session identity, `session_start`, `agent_start`, `agent_settled`, `session_shutdown`, question state, and optional `rpiv-todo` summaries. The event hook updates the matching session record. Pi question events are authoritative; prose-question detection remains a conservative fallback. Transitions to `question` and from `working` to `agent_settled` trigger best-effort `notify-send` desktop notifications.

## State

Session records contain tmux pane IDs, worktree ownership, generic Pi session IDs/files, agent status, active subagent count, optional todo summaries, Git status, and cached PR metadata. Unknown fields in older session JSON are ignored when records are read and disappear after the next write.

Runtime state also includes `worktrees/`, `cache/usage.json`, `dashboard.json`, `logs/`, and the Pi extension directory. `dashboard.json` stores the last selected table or kanban view and each session's last-viewed status timestamp. State directories use owner-only permissions (`0700`) and state files use `0600`. No runtime state is stored in the repository.

## Dashboard

The dashboard reads persisted state, refreshes Git and PR metadata through the service layer, and periodically reloads Pi status. Pressing `v` switches the same session model between the default dense table and a kanban board grouped into question, working, ready, review, and testing columns; the choice is persisted. In kanban view, vertical movement stays within a column while horizontal movement selects the same row in the nearest non-empty column. Usage is cached from `bunx ccusage session --json` and matched to persisted Pi session IDs. The UI never depends on a global Pi configuration or extension.
