# Architecture

## Overview

`oak-tree` is a Go/Bubble Tea dashboard for Pi coding sessions in tmux. Each managed session owns:

- a tmux session with one visible Pi agent in the left 60% pane and `nvim` in the right 40% pane, plus a second Pi agent in a background window;
- an optional oak-tree-owned Git worktree; and
- a JSON session record under `~/.local/state/oak-tree/sessions/`.

## Components

- `cmd/`: Cobra commands for the dashboard, session lifecycle, and Pi event hook.
- `internal/oaktree/service.go`: session, tmux, worktree, status, PR, and notification orchestration.
- `internal/oaktree/pi.go`: the state-owned Pi extension and Pi launch command.
- `internal/oaktree/store.go`: atomic state persistence.
- `internal/oaktree/coordination.go`: coordination records, locked mutations, and assignment validation.
- `internal/oaktree/tui.go`: dashboard and create-flow UI.
- `internal/oaktree/tui_coordination.go`: folded board projection and on-demand group management.
- `internal/oaktree/office_server.go`: loopback-only browser office server, private startup record, activity projection, and authenticated answer endpoint.
- `internal/oaktree/office_questions.go`: bounded Unix-socket discovery and routing to the exact current Pi session.
- `internal/oaktree/office_focus.go`: fail-closed tmux client and Hyprland window resolution for opening a managed session.
- `internal/oaktree/pi_office_bridge.mjs`: embedded Pi event bridge for pending pi-ask forms; no model-facing tools or messages.
- `internal/oaktree/officeweb/`: embedded Canvas office, pixel assets, and browser controls.
- `tools/office-art/generate.py`: reproducible original artwork and collision/depth metadata.

## Configuration

`~/.config/oak-tree/config.toml` supplies only root discovery:

```toml
root_search_dirs = ["~/dev/general", "~/work"]
roots = ["~/src/standalone-project"]
```

Each new session launches two independent Pi processes: an architect and a junior, both using openai-codex/gpt-6.1-terra with high thinking. The architect handles routine work directly and delegates isolated or parallel work to the junior when useful. For difficult questions it can request a bounded, fresh-context, read-only Astra consultation through the existing Agent/Explore tool; if unavailable, it reports the limitation rather than using an expensive fallback. Existing subagent approval requirements still apply. They share the checkout and use pi-intercom for communication; pi-intercom must be installed and enabled. Alt+A or `/oak-tree switch` swaps their tmux panes while retaining both real Pi interfaces. The Pi widget indicates the active agent. Oak-tree writes its extension to `<state>/pi/oak-tree-extension.ts`, installs an auto-discovered copy in Pi's global extension directory, and launches managed Pi processes with `OAK_TREE_SESSION_ID` and `OAK_TREE_HOOK` set. The globally loaded extension stays inert outside managed sessions except for `/oak-tree register`, which can resolve a replacement Pi process from its current tmux pane.

## Lifecycle

The extension sends events through:

```sh
oak-tree hook agent-event
```

It records Pi session identity, `session_start`, `agent_start`, `agent_settled`, `session_shutdown`, question state, optional `rpiv-todo` summaries, and agent-authored Campfire updates. After managed-session registration succeeds, the embedded extension registers `campfire_update` as an active Pi tool with low-noise prompt guidance, then forwards its typed message through the same event hook. The hook validates, deduplicates, rate-limits per source session, and appends to a bounded global activity archive. Pi question events are authoritative; prose-question detection remains a conservative fallback. Transitions to `question` and from `working` to `agent_settled` trigger best-effort `notify-send` desktop notifications.

Registered managed Pi sessions also receive `oak_coordination`, which calls `oak-tree coordination` to manage the same groups as the dashboard. Group metadata is explicit; ordinary intercom messages do not create groups or change assignment progress. Coordination does not deliver messages or start agents.

## State

Session records contain tmux pane IDs, worktree ownership, generic Pi session IDs/files, agent status, optional todo summaries, Git status, and cached PR metadata. Paired sessions additionally persist each agent's pane, Pi identity, status and todos. The dashboard combines todos and prioritizes question over working, attention and idle. `campfire.json` retains the latest 200 Campfire messages independently of session lifetime; legacy per-session messages remain visible and are archived when their session closes. The kanban dashboard reads the archive and merges that compatibility data without duplicating messages. Unknown fields in older session JSON are ignored when records are read and disappear after the next write.

Runtime state also includes `worktrees/`, `campfire.json`, `coordination.json`, `cache/usage.json`, `dashboard.json`, `logs/`, and the Pi extension directory. `coordination.json` stores active and closed groups, each with an organizer, participant assignments, explicit progress, results, and timestamps. Group mutations use a cross-process file lock around atomic read-modify-write operations. `dashboard.json` stores the last selected table or kanban view and each session's last-viewed status timestamp. State directories use owner-only permissions (`0700`) and state files use `0600`. No runtime state is stored in the repository.

## Dashboard

The dashboard reads persisted state, refreshes Git and PR metadata through the service layer, and periodically reloads Pi status. Pressing `v` switches the same session model between the default dense table and a kanban board grouped into question, working, ready, review, and blocked columns; the choice is persisted. In kanban view, vertical movement stays within a column while horizontal movement selects the same row in the nearest non-empty column. Usage is cached from `bunx ccusage session --json` and matched to persisted Pi session IDs. The UI never depends on a global Pi configuration or extension.

In kanban, a shared visible-session projection controls both rendering and navigation: participants fold under their organizer by default, questions remain selectable shortcuts, and manually testing sessions remain in the testing strip. Counts use the complete session list, not the projection. `g` opens the group dialog or creates a group; `space` toggles participant visibility. The dialog keeps agent state separate from assignment progress. See [Coordination groups](coordination.md) for the contract.

### Browser office

`oak-tree office`, or `f` in the dashboard, opens a local browser view. A detached server binds only to `127.0.0.1` on an ephemeral port and survives the dashboard closing. A private `office.json` record stores its port, PID, and random authentication token. Startup uses a bounded file lock and authenticated health check to reuse the server. It shuts down after thirty minutes without authenticated API activity and removes only its own runtime record. Startup diagnostics go to `logs/office.log`.

The server embeds a fixed set of assets, GET health/state endpoints, and a POST answer endpoint. API access requires a bearer token supplied to the browser in the initial URL fragment, then retained in that tab's session storage; the fragment is removed from browser history. Exact Host and Origin validation, a self-only content security policy, and the absence of CORS prevent unrelated websites from reading agent activity. The state projection includes project names, branches, roles, status, and current activity, not raw session files or repository root paths.

The browser polls persisted session and Campfire data every two seconds (ten seconds in a background tab). It does not refresh Git, capture tmux panes, or call models. Each paired role has its own character and task; Campfire updates remain explicitly session-scoped. It can answer an existing pi-ask form, but cannot start arbitrary agent turns, run commands, or manage sessions.

Canvas rendering is independent of polling. Three connected rooms share one walkable floor plan. Placement uses the session's dashboard bucket for every paired role: working sessions use the work room, ready and question sessions use the central meeting room, and review or blocked sessions use the break room. For an active coordination group with a present organizer, the organizer's review or blocked tag overrides the office placement of every member, including working or questioning members; unparked groups retain per-session placement. Group ID and name provide matching colored pennants and scarves without exposing assignments. This projection does not change kanban status or counts. Parked status takes precedence over questions; pending forms remain accessible from either room. Furniture and cutaway partitions constrain paths; agents use doorways rather than crossing walls. Room buttons focus the camera without changing agent state. The floor, furniture, and sprite variants load once; local animation changes positions and walking frames, with depth sorting and collision-aware paths. Dragging, keyboard movement, zoom, and panning affect only the visual scene. Positions are temporary. Double-clicking a character requests its managed session: the server resolves its recorded tmux identity and verifies the managed session marker, then identifies exactly one tmux client and one Hyprland window through process ancestry. It rechecks these identities before switching the explicit client and focusing the window. Ambiguous mappings fail closed; a Hyprland failure after a tmux switch is reported as partial success. This opens the session, not a particular agent pane. The terminal remains the session manager, and its table/kanban preference is unchanged.

After successful managed-session registration, Pi opens a random `0600` Unix socket under the private `0700` `<state>/office-bridges/` directory. `OAK_TREE_STATE_DIR` supplies the state root for new managed processes; manually registered processes use the normal XDG default. The bridge holds validated pi-ask lifecycle events in memory. It also counts finalized failed tool executions, including nested failures, without retaining tool names, arguments, or results. The browser uses per-runtime counter deltas for a cosmetic wall-crash animation; new bridge generations establish a baseline rather than replaying history. This signal is not a durable audit log. It never writes questions, answers, or failure counts to session JSON, adds prompt text, or calls a model. Existing sessions require an idle `/reload` after the updated extension has been installed by opening the office/dashboard or creating a session.

State polling queries live sockets with bounded deadlines. A bridge must uniquely match the current managed session, role, Pi session ID, pane, and session file. Fresh opaque form IDs prevent replaying answers into a reopened form. Absent, ambiguous, or stale bridges cannot receive answers. Shutdown removes the process's own socket and invalidates its forms. Unclean exits can leave inert socket files; they do not restore pending forms.

Answer POSTs require the bearer token, exact Host, an explicit matching Origin, JSON content, and a body no larger than 64 KiB. Membership and live form identity are checked again before one delivery attempt. The Pi bridge uses pi-ask's documented submit/result events and normalizer, not terminal keystrokes. Pi remains authoritative for allowed option values and the current question type. Terminal and browser completion race safely; stale or duplicate responses are rejected. The browser warns that submitting replaces unfinished Pi drafts, retains only bounded in-memory browser drafts, and does not retry uncertain delivery. `ask:resume` and manually reopened forms retain Pi's normal follow-up behavior; live tool forms return through the existing waiting tool. Neither path adds office-specific model calls or context.
