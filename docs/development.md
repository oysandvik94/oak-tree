# Development

## Prerequisites

- Go.
- tmux.
- git.
- GitHub CLI (`gh`) for PR cache refresh/open behavior.
- Pi CLI for end-to-end manual testing.

The app is Linux-oriented in v1 because tmux popup behavior and the default paths are Unix-specific.

## Common Commands

```sh
go test ./...
go run . --help
go run . popup
go run . new --root /path/to/repo
go run . new --root /path/to/repo --branch feature/example
```

Create a local config for dashboard testing:

```sh
mkdir -p ~/.config/oak-tree
cat > ~/.config/oak-tree/config.toml <<'EOF'
root_search_dirs = [
  "~/dev/general",
  "~/work",
]
roots = [
  "~/src/standalone-project",
]
EOF
```

Format Go files before finishing changes:

```sh
gofmt -w .
```

## Manual Pi Verification

Pi integration requires an installed `pi` binary and a durable `oak-tree` executable; do not launch long-lived Pi panes from `go run`.

1. Install oak-tree.
2. Create a session and confirm tmux opens Pi on the left at 60% width and `nvim` on the right at 40%.
3. Run a prompt and confirm the dashboard changes to `working`, then `done` when Pi settles.
4. Without `@juicesharp/rpiv-ask-user-question`, ask Pi to make a choice that requires user input. Confirm oak-tree's fallback `question` tool opens, the dashboard shows `question`, and answering or cancelling restores `working`.
5. Install `@juicesharp/rpiv-ask-user-question`, start a new session, and confirm only `ask_user_question` is exposed and produces the same `question` → `working` transition.
6. Exercise `/new`, `/resume`, and `/fork`; confirm the current Pi session id/file is recorded on the same oak-tree session JSON.
7. Exit Pi and confirm the dashboard changes to `idle`.

Confirm each linked session row shows its `ccusage@20.0.20` cost after the background usage refresh completes. The selected row should be highlighted, and rows should not repeat a `tmux` label. A row is matched only when its stored Pi session ID is the same UUID as `ccusage`'s `period`; a path is accepted only if it embeds exactly one UUID.

## Usage Accounting

The dashboard deliberately parses the pinned `ccusage@20.0.20 session --json` Pi row schema and fails its refresh on a schema change rather than guessing field aliases. Its cost is scoped to the Pi session IDs stored on that oak-tree session; it is not the global Waybar amount.

For global Pi reconciliation and fork fixtures, use the dotfiles helper from the dotfiles checkout:

```sh
python3 scripts/test_pi_usage.py
python3 scripts/pi_usage.py --timezone Europe/Oslo report --since 2026-08-01 --until 2026-08-31
```

The report is local-only: do not commit its output because it contains local session paths. It records the scanned root, excluded copied entry identities, and the pinned ccusage comparison.

## Testing Guidance

Prefer unit tests for command construction, parsing, state storage, and git path decisions. Avoid requiring a live tmux server in ordinary `go test ./...` runs. If live tmux tests are added later, guard them behind an environment variable.

## Debug Logs

External command diagnostics are written to:

```text
~/.local/state/oak-tree/logs/oak-tree.log
```

Use this when debugging create failures:

```sh
tail -50 ~/.local/state/oak-tree/logs/oak-tree.log
```

## Design Notes

- Keep external commands behind small adapters.
- Keep the dashboard responsive by loading session status, Git state, PR metadata, and usage through Bubble Tea commands instead of blocking rendering.
- Keep PR metadata refreshes behind the service layer. Dashboard startup may queue background refreshes only for missing or stale branch-backed session caches; selected-session force refresh remains explicit through `p`.
- Keep documentation in sync with lifecycle and state changes.
