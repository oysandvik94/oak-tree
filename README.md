# oak-tree

`oak-tree` is a terminal dashboard for managing AI coding sessions inside tmux.

Disclaimer: This is completely vibe-coded and made personally for me, and it's only public as a reference for others who want to create similar tools.
If you want to have a similar tool, I recommend giving your personal specs to an agent, listing what tooling you use, and providing this repo as a reference project for how an orchestrator tool can be implemented.

It is built for the workflow where each AI work session gets:

- one tmux session
- one Pi coding session
- one editor pane running `nvim`
- one agent pane running Pi
- optionally, one Git worktree for a branch

The dashboard is meant to be opened in a tmux popup.

## Requirements

- Go
- tmux
- git
- GitHub CLI (`gh`) for pull request metadata
- Pi CLI
- Neovim

## Install

From this repository:

```sh
go install .
```

Or run directly while developing:

```sh
go run .
```

## First Setup

Create an oak-tree config file:

```sh
mkdir -p ~/.config/oak-tree
$EDITOR ~/.config/oak-tree/config.toml
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

The dashboard lists immediate child directories from `root_search_dirs` and exact directories from `roots`, then lets you fuzzy-search them when creating a session.

Oak-tree writes its private Pi extension under the oak-tree state directory and loads it only for oak-tree-managed Pi sessions. Pi sessions require installed `pi` and `oak-tree` executables; a temporary `go run` binary cannot receive long-lived lifecycle events.

## Open The Dashboard

Inside tmux:

```sh
oak-tree popup
```

Outside tmux:

```sh
oak-tree
```

## Create A Session

From the dashboard, press `n`.

The root selector opens as a fuzzy-searchable list based on `root_search_dirs` and `roots` from:

```text
~/.config/oak-tree/config.toml
```

Pick a root directory, then enter an optional branch name. On the branch step, press `ctrl+o` to switch between creating a new branch and opening an existing branch.

If no configured candidates are available, the create flow falls back to manual root entry.

If you provide no branch, oak-tree starts the session in the root directory.

In create-new mode, if you provide a branch, oak-tree:

1. Detects the repository default branch.
2. Fetches the latest default branch from `origin`.
3. Creates a managed Git worktree.
4. Configures the branch to push to `origin/<branch-name>`.
5. Starts tmux with the Pi on the left at 60% width and `nvim` on the right at 40%.

In open-existing mode, oak-tree opens a branch that already exists locally or on `origin`. If the branch is already checked out in another worktree, oak-tree starts the tmux session there and does not own or remove that worktree on close.

You can also create a session from the CLI:

```sh
oak-tree new --root /path/to/repo
oak-tree new --root /path/to/repo --branch feature/my-task
```

## Dashboard Keys

```text
j / down   move down
k / up     move up
h / left   move to previous kanban column
l / right  move to next kanban column
enter      switch/attach to selected tmux session
v          toggle table/kanban view
n          create session
t          set session status
e          edit selected session note
x          close selected session
o          open cached PR for selected session
p          refresh PR for selected session
q          quit
```

When closing, press `y` to confirm. If the dashboard popup is running from the session being closed, oak-tree switches the tmux client to a nearby remaining session before killing it. Kanban view groups sessions into `QUESTION`, `WORKING`, `READY`, `REVIEW`, and `TESTING` columns while keeping the same selection and actions. The last selected view is restored the next time the dashboard opens.

## Session Status

Press `t` on a selected session to choose a manual status:

```text
ACTIVE
WAITING REVIEW
TESTING
```

Sessions marked `WAITING REVIEW` or `TESTING` move into separate parked sections below active sessions and are excluded from the active count. The manual tag overrides the current agent state until changed back to `ACTIVE`. Press `e` to open the selected session's note popup, where an empty note clears it; noted kanban cards show a highlighted `✎` icon.

## Agent Status

Session rows show coding-agent turn status next to `tmux`:

```text
⠹ working
✓ ready
? question
```

`working` is set when Pi starts a turn. `ready` means no turn is running and the session is available for your next action. Pi consumes the `rpiv:ask-user:prompt` event from `@juicesharp/rpiv-ask-user-question`; when that tool is unavailable, oak-tree registers a simpler `question` fallback. Pi prose questions retain a conservative pane fallback. When a session first changes to `question`, oak-tree runs `notify-send` with a desktop notification.

## Todo Status

For Pi sessions using `@juicesharp/rpiv-todo`, the wide dashboard table shows completed and total tasks in a `TODO` column. `◐ 1/3` means a task is in progress, `○ 1/3` means work is pending with nothing active, and `✓ 3/3` means all tasks are complete. Press `space` on a session with todo details to expand or collapse its task subjects inline. Oak-tree restores the latest todo snapshot when Pi starts and updates it after each successful `todo` tool call. Compact layouts hide the summary column.

## Git Worktree Status

Session titles show a compact Git state after the branch: `clean`, `changes`, `unpushed ↑`, `changes ↑`, or `git?`. The dashboard refreshes this with `git status --branch --porcelain=v1 --untracked-files=all` on load, on manual refresh, and on a slower background timer.

## Pull Request Status

oak-tree can cache GitHub pull request metadata for a branch-backed session.

The dashboard refreshes missing or stale PR metadata for branch-backed sessions in the background through `gh`. A PR cache is considered stale after ten minutes. This resolves the repository from the session workdir's `origin` remote and looks for a PR whose head branch matches the session branch.

When the selected session has a cached PR, a two-line inspector appears above the global key footer. It shows the PR number and title, cache age, draft/ready state, CI status, approval status, and unresolved comments. The inspector also shows the contextual `o` open and `p` refresh commands; it stays hidden when no PR exists for the selected session.

Press `o` to open the cached PR in your browser through `gh pr view --web`, or `p` to refresh its metadata.

Usage comes from `bunx ccusage session --json` and is matched against agent session IDs recorded by oak-tree's Pi extension. Because ccusage is slow, oak-tree stores the parsed result in `~/.local/state/oak-tree/cache/usage.json`, uses cached data on dashboard startup, and refreshes it in the background when missing or older than one minute.

In the create flow:

```text
type       filter/search roots
enter      select root, then submit branch
ctrl+o     toggle create-new/open-existing branch mode
tab        switch between root/manual entry and branch controls
esc        cancel
```

## State Location

oak-tree stores runtime state under:

```text
~/.local/state/oak-tree/
```

Important subdirectories:

```text
sessions/   session JSON files
worktrees/  oak-tree-created Git worktrees
hooks/      local hook wrapper scripts
cache/      local dashboard metadata caches
logs/       command debug logs
```

Session JSON may include cached PR metadata under `pr`. That cache is updated by dashboard background refresh for missing or stale branch-backed sessions, and by explicit PR refresh.
Session JSON may include a manual status under `tag`, such as `waiting_review` or `testing`, and a Pi todo summary under `todo`.
Agent usage data is cached separately under `cache/usage.json`.

User configuration is stored separately under:

```text
~/.config/oak-tree/config.toml
```

## Closing Sessions

When you close a session, oak-tree:

1. Kills the tmux session.
2. Removes the managed Git worktree if oak-tree created one.
3. Runs `git worktree prune`.
4. Keeps the local branch.

Dirty managed worktrees are refused by default, so changes are not silently removed.

## Debugging

If session creation fails, oak-tree should show the failing step, command, stderr, and log file path. The command log is written as JSON Lines here:

```text
~/.local/state/oak-tree/logs/oak-tree.log
```

Useful command:

```sh
tail -50 ~/.local/state/oak-tree/logs/oak-tree.log
```

## Current Limitations

- v1 shows only sessions created or registered by oak-tree.
- Pi is launched with oak-tree's private extension.
- The editor command is currently fixed to `nvim`.
- Live tmux behavior should be manually tested after changes that touch tmux commands or dashboard preview.

Pi sessions report lifecycle and `rpiv-todo` summary events directly to `oak-tree hook agent-event` and include a blocking `question` tool. Usage totals are backed by `ccusage`.

## Development

Run tests:

```sh
go test ./...
```

Build:

```sh
go build ./...
```

Format:

```sh
gofmt -w main.go cmd internal
```

More technical notes are in:

- [docs/architecture.md](docs/architecture.md)
- [docs/session-lifecycle.md](docs/session-lifecycle.md)
- [docs/development.md](docs/development.md)
