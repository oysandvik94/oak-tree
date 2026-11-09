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
4. Without `@juicesharp/rpiv-ask-user-question` or `@eko24ive/pi-ask`, ask Pi to make a choice that requires user input. Confirm oak-tree's fallback `question` tool opens, the dashboard shows `question`, and answering or cancelling restores `working`.
5. Install `@juicesharp/rpiv-ask-user-question`, start a new session, and confirm only `ask_user_question` is exposed and produces the same `question` → `working` transition. Repeat with `@eko24ive/pi-ask` and confirm only `ask_user` is exposed.
6. Exercise `/new`, `/resume`, and `/fork`; confirm the current Pi session id/file is recorded on the same oak-tree session JSON.
7. Exit Pi and confirm the dashboard changes to `idle`.

Confirm each linked session row shows its `ccusage@20.0.20` cost after the background usage refresh completes. The selected row should be highlighted, and rows should not repeat a `tmux` label. A row is matched only when its stored Pi session ID is the same UUID as `ccusage`'s `period`; a path is accepted only if it embeds exactly one UUID.

## Manual Office Verification

1. Run `oak-tree office`, or press `f` in the dashboard. Confirm a local browser page opens with separate paired-agent characters, live task text, and all sessions in the roster.
2. Change an agent's task/state through normal Pi activity. Confirm the page updates without reloading, and shared Campfire bubbles say `Session update`.
3. Drag a character, then select a roster entry and use Space/arrow keys to move it. Confirm furniture constrains placement and no real agent state changes.
4. Zoom, pan, focus each room, use `Fit office`, and resize to a narrow browser window. Confirm hit-testing stays aligned and the `Agents` drawer remains usable.
5. Pause motion. Confirm activity still updates while automatic walking stops. Check the reduced-motion browser preference as well.
6. Reopen the office and confirm the server is reused. Close the dashboard and confirm the page remains live. Verify disconnection is visible if the office server stops.
7. In an isolated test state directory, verify wrong/missing tokens, foreign/null Origins, unexpected Hosts, unsupported methods, and unknown asset paths are rejected. Answer POSTs also require a same-origin header, JSON content, and a bounded body. Use synthetic text containing HTML to confirm it is displayed as text rather than interpreted.
8. Verify every agent follows its session's dashboard state: working sessions use the work room, ready and question sessions use the central meeting room, and review or blocked sessions use the break room. Parked states take precedence over questions. Change a state and observe a doorway crossing, not a teleport or wall crossing.
9. Use synthetic pi-ask flows to exercise single, multi, preview, custom-text, note, and explicit cancellation responses. Confirm normalized values/labels reach the original flow exactly once. Check terminal completion, stale/replayed IDs, invalid answers, missing acknowledgements, reload/shutdown, disconnected sockets, and draft preservation during polling. Do not answer real pending questions as a test.

Focused checks do not require a model call:

```sh
go test ./internal/oaktree -run '^(TestOffice|TestPiOffice|TestPiCommand|TestEnsurePiExtension)' -count=1
```

To also check the installed pi-ask normalizer, set `PI_ASK_REMOTE_SOURCE` to its `src/remote-ask.ts` file. That optional contract harness uses Node's TypeScript stripping (Node 24 or newer), copies only that source to a test directory, and never launches an agent or modifies the installed package. Socket and mock-event tests use isolated state directories. Use Node 24 or newer when running the bridge tests; the Go-only server tests do not require Node.

The browser sources and generated PNG assets are embedded from `internal/oaktree/officeweb/`. There is no npm build or runtime Python requirement. To regenerate artwork, use Python/Pillow as described in [the artwork generator](../tools/office-art/README.md). Keep browser screenshots, test state, and authentication records outside the checkout.

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
