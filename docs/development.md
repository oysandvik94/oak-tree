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

## Session Naming Verification

`TestPiSessionNaming` runs the embedded extension against a stub model registry, without model calls or live Pi sessions. It covers a scoped list with no small model, current-model fallback, inexpensive same-provider selection, first-input-only behavior, deliberate names, manual-rename/shutdown races, empty/error responses and timeout warnings. Metadata/card tests verify branch-first and project-plus-ID fallbacks, generated names and retained project/branch context. Existing conversations must not be backfilled.

## Manual Office Verification

1. Run `oak-tree office`, or press `f` in the dashboard. Confirm a local browser page opens with separate paired-agent characters, live task text, and all sessions in the roster.
2. Change an agent's task/state through normal Pi activity. Confirm the page updates without reloading. With no agent selected or hovered, verify every onscreen agent has a compact underfoot nameplate and that a new role-attributed Campfire post appears above its author, even while that role has an in-progress task. Check two speakers at once, ten-second expiry, no replay on selection/reload, and full text retained in the sidebar. Labels/ribbons must avoid character hit targets, remain inside the scene and use thin tethers instead of large cards.
3. Drag to orbit, right-drag to pan, and select a roster entry. Confirm no real agent state changes. Click empty space or press scene-focused Escape to deselect; polling must keep selection clear. Double-clicking or Open session should focus only the intended managed session.
4. Zoom, pan, press `f` with the canvas focused, and resize to a narrow browser window. Confirm hit-testing stays aligned and the roster and activity feed stack below the scene. Confirm no header or scene control panels remain.
5. Confirm the training wing has eight unique stations: squat, deadlift, pec deck, bench press, leg press, lat pulldown, seated row and leg extension. Observe bent-knee barbell squats, straight-arm deadlifts, seated flies, moving pec-deck handles and weight stacks, and staggered sets followed by floor rests and station rotation. Check that machine changes, floor rests and spotter travel stay inside training rather than detouring through the hallway, including when retargeted mid-walk. Raise a question during a set or in transit: the agent must visibly run to coaching at 2.5 times walking speed, then stop running on arrival. Review/blocked overrides and ordinary travel must still walk. With more than eight workers, verify waiting agents get turns, exercise choices vary without immediate repeats, and no station has unrelated users. Verify an idle partner spots the working architect or junior, follows station changes, and leaves when its own work or question starts. The session must remain busy while either partner works. In isolated fixtures, increment a role's failure counter and verify that exercise's failed lift and spotter assistance; raise a new question and complete the whole session to verify their distinct reactions. First load, unchanged polling, and bridge generation changes must not replay old failures. Change status or remove a worker during a set and during travel: reservations must release, routes must stay in clear lanes, and bars must return to their resting positions. Also observe individually timed curls, presses, squats, towel breaks, stretches, drinks, reading, hand raises and pacing. Confirm two ready agents sit on the recovery sofa with bent knees, two review agents use the table, and remaining agents keep separate floor stations. Check sofa arrivals/departures go around the sides and pacing stays within its zone. With reduced motion enabled, routines and the animation clock must stop while activity keeps updating and state changes snap to the correct station. Press Space with the scene focused and confirm it does not change the motion preference. Check disconnection as well.
   In isolated fixtures, vary each role's `todo.completed` independently from zero through four and above. Verify one extra plate pair per completion (capped at four extras), larger dumbbells and more selected/moving stack blocks, without transferring progress to the partner. Reopen/reset tasks and omit a summary: explicit lower counts reduce the load; missing data retains the last known load. Check these changes still render with reduced motion. Observe a plate handoff during a shared floor rest, one carried plate and subsequent machine loading/chalk, with no duplicated props or overlapping reservations. Complete a working pair and verify adjacent recovery places, a single fist bump with meeting hands, paired drinks, bench wiping and occasional flexing. Interrupt with work, questions, parking, unknown status or removal and confirm no stale interaction remains. Initial ready snapshots must not invent a completion. No cardio workout routines or sound should appear; only urgent question-room travel should run.
6. Reopen the office and confirm the server is reused. Close the dashboard and confirm the page remains live. Verify disconnection is visible if the office server stops.
7. In an isolated test state directory, verify wrong/missing tokens, foreign/null Origins, unexpected Hosts, unsupported methods, and unknown asset paths are rejected. Answer POSTs also require a same-origin header, JSON content, and a bounded body. Use synthetic text containing HTML to confirm it is displayed as text rather than interpreted.
8. Verify each role uses its own active status: training for working, coaching for questions, recovery for ready. Session/group review and blocked overrides use their own zones and take precedence over questions. Verify an idle architect or junior spots its working partner without claiming another machine, while questions and parked states retain their own zones. Check live transitions through the connecting aisle, growth beyond six agents per zone, unknown status, agent removal, and an empty gym. Check cached PR and task details in the inspector.
9. Use synthetic pi-ask flows to exercise single, multi, preview, custom-text, note, and explicit cancellation responses. Confirm normalized values/labels reach the original flow exactly once. Check terminal completion, stale/replayed IDs, invalid answers, missing acknowledgements, reload/shutdown, disconnected sockets, and draft preservation during polling. Do not answer real pending questions as a test.

Focused checks do not require a model call:

```sh
go test ./internal/oaktree -run '^(TestOffice|TestPiOffice|TestPiCommand|TestEnsurePiExtension|TestPiSessionNaming|TestSessionNam|TestCampfirePersistsAgentRole)' -count=1
```

To also check the installed pi-ask normalizer, set `PI_ASK_REMOTE_SOURCE` to its `src/remote-ask.ts` file. That optional contract harness uses Node's TypeScript stripping (Node 24 or newer), copies only that source to a test directory, and never launches an agent or modifies the installed package. Socket and mock-event tests use isolated state directories. Use Node 24 or newer when running the bridge tests; the Go-only server tests do not require Node.

The exercise registry in `assets/gym/exercises.js` defines eight equipment exercises and three floor exercises. Every exercise provides a normal pose and handlers for every entry in `gymEventTypes`. Adding an exercise or event without the matching handlers fails registry validation and the focused gym checks. `events.js` converts real snapshot changes into visual events; do not infer conversation events from assistant prose.

The browser sources, original voxel builders and vendored Three.js r170 modules are embedded from `internal/oaktree/officeweb/`. There is no npm build or runtime Python requirement. Assets under `assets/gym/` use relative module imports and the existing self-only CSP. Three.js's MIT license is beside its vendored modules. The former PNG artwork generator remains in `tools/office-art/` but is not used by the gym.

`overlays.js` coordinates compact underfoot names and overhead notifications, accounting for onscreen character bounds. `partners.js` coordinates local paired gestures and recovery ordering; `state.js` maps each role's current completed-todo count to a bounded visual load. These features use existing snapshots and never create task history or make model calls. `TestOfficeGymRoutes` checks internal machine/floor/spotter routes, obstacle clearance, room changes and mid-walk retargeting at normal and expanded capacities; `TestOfficeGymMotion` checks question running without changing ordinary walking or reduced-motion poses.

`tests/gym-browser.cjs` exercises the embedded page with synthetic API responses. It intercepts **all** API requests, including mutations, so it never sends a real agent a message, answer, note or focus request. It covers placement, unique equipment and exclusive reservations, partner spotting, role-specific live load changes and resets under reduced motion, permanent compact nameplates, automatic per-role Campfire ribbons and expiry, measured question-running speed and arrival, mouse/keyboard interaction, forms, offline recovery, capacity changes, full-height desktop and stacked mobile layouts, Bergen seasonal lighting with reduced motion, and a WebGL failure fallback. To run it, install Playwright in an external tooling directory, build oak-tree to a temporary binary, create a private temporary state directory, and start that binary with `office --serve --state-dir <tempdir>`. Then run `node tests/gym-browser.cjs <tempdir>` with `NODE_PATH` pointing to the tooling directory's `node_modules`. If needed, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to an installed Chromium executable. `OAK_GYM_SCREENSHOT` selects an overview screenshot outside the checkout (default `/tmp/oak-gym-integrated.png`); `OAK_GYM_BUBBLES_SCREENSHOT` selects the simultaneous-notification screenshot (default `/tmp/oak-gym-notifications.png`). Stop only that isolated preview process afterward. A software-rendered browser check is not a hardware performance benchmark.

Keep browser screenshots, test state, and authentication records outside the checkout. Because assets are embedded, an already-running office server keeps its original frontend until it is restarted with the rebuilt binary; refreshing the browser alone cannot update it.

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
