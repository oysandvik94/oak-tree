# Session Lifecycle

## Create Without Branch

1. User selects a root directory from the dashboard fuzzy picker, enters it manually, or passes it with `oak-tree new --root`.
2. oak-tree validates that the directory exists.
3. oak-tree uses the root directory as the active working directory.
4. oak-tree creates a tmux session, reads back the initial pane id from `tmux new-session`, and splits that pane into two vertical panes.
5. The left pane starts the architect Pi with the state-owned extension at 60% width; the right pane runs `nvim` at 40% width.
6. A second, independent junior Pi starts in a background tmux window in the same checkout. Press Alt+A or run `/oak-tree switch` inside either Pi to switch the visible agent. Both continue running when hidden.
7. oak-tree records the session, both Pi pane ids, root, and active working directory.
8. oak-tree publishes the session's pane ids as tmux session user options (see [tmux session options](#tmux-session-options)).

## Create With Branch

1. User selects or passes a root directory and branch name in create-new mode.
2. oak-tree validates that the root is a git repository.
3. If `origin` exists, oak-tree detects its default branch.
4. If `origin` exists, oak-tree fetches the latest default branch. Remote errors stop creation, without falling back to local history.
5. oak-tree creates a git worktree under:

   ```text
   ~/.local/state/oak-tree/worktrees/<repo-name>/<safe-branch-name>
   ```

6. oak-tree starts the same two-pane tmux layout in the worktree directory.
7. For a new branch with `origin`, oak-tree starts from `origin/<default-branch>` with `--no-track`. Without `origin`, it starts from the current local branch. A detached HEAD requires selecting a local branch first. Existing branches retain their own history.
8. With `origin`, oak-tree configures the branch to push to `origin/<branch-name>` instead of tracking the default branch it was created from. This also repairs an existing local branch used through create-new mode if it still has the old default-branch upstream. Without `origin`, oak-tree does not fetch or configure upstreams. Uncommitted source changes are not copied to the worktree.
9. oak-tree records the worktree path as owned by the session.

## Open Existing Branch

1. User selects a root directory and toggles the dashboard branch mode to open existing. oak-tree fetches and prunes `origin`, then the branch field fuzzy-filters local and `origin/*` branches for that repository. If the fetch fails, cached branches remain selectable alongside the refresh error; manual entry remains available when a branch is not listed.
2. oak-tree checks `git worktree list --porcelain` to see whether the branch is already checked out in another worktree.
3. If it is checked out, oak-tree starts the tmux session in that worktree and records it as not owned by oak-tree.
4. If it is not checked out, oak-tree checks whether the branch exists locally or on `origin`.
5. If the branch exists locally, oak-tree creates a managed worktree from the local branch.
6. If only `origin/<branch-name>` exists, oak-tree fetches it and creates a local tracking branch in a managed worktree.
7. If the branch exists neither locally nor on `origin`, oak-tree fails before starting tmux.
8. If tmux startup fails, oak-tree removes only worktrees it owns.

New tmux sessions use descriptive names: `oak-<repository>-<branch>-<short-id>`, or `oak-<repository>-<short-id>` without a branch. Components are sanitized for tmux, and the short session ID keeps names unique.

## Close Session

Closing still refuses to remove a dirty oak-tree-owned worktree. When the dashboard popup is opened from the tmux session being closed, it first switches the client to the next visible oak-tree session, or the previous one when closing the last visible row, and then kills the old session. Closing the only managed session retains tmux's normal detach behavior because no fallback session exists.

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

Oak-tree installs its state-owned extension into Pi's global extension directory and launches both Pi processes with `OAK_TREE_SESSION_ID`. The architect uses `openai-codex/gpt-6.1-sol` with xhigh thinking; the junior uses `openai-codex/gpt-6.1-terra` with high thinking. The architect handles routine work directly and delegates isolated or parallel tasks to the junior when that saves work. For a difficult question, it may consult the existing read-only Agent/Explore tool using `openai-codex/gpt-6-astra` with low thinking, fresh context and at most six turns; if the tool is unavailable, it reports the limitation without an expensive fallback. User approval requirements for reviewer and implementation subagents remain in force. Their unique Pi session names let them address each other through pi-intercom, which must be installed and enabled. They share the working tree and are instructed to coordinate edits. The dashboard aggregates per-agent status with question taking priority over working, attention and idle, and combines their todo summaries. Existing sessions are not converted automatically. It also appends a managed-checkout instruction telling Pi to work in the branch and worktree oak-tree already selected, and not to create or switch branches or worktrees unless the user explicitly asks. The extension sends lifecycle events to `oak-tree hook agent-event`, including Pi's session id and session file. This keeps `/new`, `/resume`, and `/fork` linked to the same oak-tree session.

For older single-agent sessions, if Pi is closed and restarted manually inside the same oak-tree tmux session, run `/oak-tree register`. The extension resolves the oak-tree session from the current tmux pane, replaces the recorded Pi pane, and starts reporting the replacement Pi session's lifecycle and usage identity. Paired sessions do not yet support manually replacing either Pi pane; closing one agent leaves its tab unavailable until a new oak-tree session is created.

## tmux session options

Each managed tmux session carries these session-scoped user options so external tools, such as an editor/Pi bridge, can locate its panes without guessing from pane commands:

| Option | Value |
| --- | --- |
| `@oak_tree_session_id` | oak-tree session ID |
| `@oak_nvim_pane` | tmux pane ID running `nvim`, for example `%61` |
| `@oak_pi_pane` | tmux pane ID of the visible Pi, for example `%62` |
| `@oak_architect_pane` | tmux pane ID of the architect Pi |
| `@oak_junior_pane` | tmux pane ID of the junior Pi |

Oak-tree sets them with `tmux set-option -t <tmux-session-id> ...` after creating the session. For paired sessions, switching agents updates `@oak_pi_pane`; for single-agent sessions a Pi hook refreshes the original options on `session_start` or a changed Pi pane. That also backfills sessions created before this behavior once Pi restarts or runs `/oak-tree register`. Other hook events do not run tmux. The two agent-specific options are required for paired session creation; publishing the common options and later visible-pane updates is best effort. Read them with `tmux show-options -t <session> -v @oak_pi_pane`; a missing option means the session is not managed by oak-tree or the options have not been published yet, so consumers should fall back to pane discovery.

The extension consumes `@juicesharp/rpiv-ask-user-question`'s `rpiv:ask-user:prompt` event and observes that tool's completion. It also consumes `@eko24ive/pi-ask`'s `@eko24ive/pi-ask:started` and `@eko24ive/pi-ask:completed` events for `ask_user` forms. When neither `ask_user_question` nor `ask_user` is available, it registers a simpler `question` fallback. Answering or cancelling either UI restores `working`. `agent_settled` is represented as `attention` and sends a best-effort desktop notification; shutdown is `idle`. Only the top-level TUI runtime reports these events; embedded SDK sessions used by subagents run in print mode and are ignored. Ordinary prose prompts still use the pane heuristic as a fallback.

Every `agent_start` clears a `WAITING REVIEW` or `BLOCKED` tag, so a parked session returns to the active columns when any run starts, whether from a typed prompt or an extension such as pi-intercom.

When `@juicesharp/rpiv-todo` is loaded, the same extension reads the latest persisted `todo` tool-result details during `session_start` and after each successful `todo` execution. It sends visible task subjects, statuses, and counts through the agent-event hook. Deleted tasks are ignored; invalid count summaries are rejected at the hook boundary.

After a managed session registers successfully, the extension adds an active `campfire_update` tool. Its prompt guidance asks Pi to report meaningful plans, discoveries, setbacks, recoveries, test results, phase changes, milestones, and handoffs in a candid, truthful shared-room voice at most 140 characters long. Agents are invited to write in first person and pair concrete developments with genuine difficulty, frustration, surprise, relief, pride, or amusement instead of flattening every post into status-report language; feelings, progress, and drama must never be invented. Delegating work to another agent or session, and receiving a notable delegated result, should produce an update; only routine operations, tiny tasks, and recently repeated information stay silent. The extension also adds `campfire_read`, which returns the newest 12 posts from that same shared feed for occasional ambient awareness or a genuine brief reaction. It must not be polled or used for direct coordination; related work remains on pi-intercom. Campfire events use the existing agent-event hook and do not change agent status. The hook rejects unknown kinds, control characters, and oversized messages; deduplicates consecutive messages; rate-limits routine updates from each source session to one per 45 seconds; and retains the latest 200 messages in a global archive. `snag`, `milestone`, and `handoff` events bypass the time limit.

## Dashboard Session Table

`oak-tree popup` opens the dashboard at 95% of the tmux client width and height. The coordination group dialog keeps its separate size limit.

The dashboard is a full-width, dense session table optimized for session management. Each row shows agent state, project/session, branch, Git state, cached PR state, Pi todo progress on wide layouts, and usage where available. Todo progress uses completed/total counts with distinct in-progress, pending, and complete chips; `space` expands or collapses the selected session's task subjects inline. Summary counters highlight sessions needing attention, working, ready, waiting review, and blocked. A manually tagged testing session remains active and appears in a full-width `TESTING NOW` strip above the kanban columns, including its live `QUESTION`, `WORKING`, or `READY` agent state. In kanban view, the initial selection stays on the board when one is available; `tab` moves focus between the testing strip and the board, and arrow keys stay within the focused area. Manually tagged review and blocked sessions are excluded from the active count and grouped in separate parked sections below active sessions. Their kanban cards show a compact age measured from the latest tag change; legacy records fall back to session creation time because their original tag time was not recorded. Sessions may also store a one-line note opened and edited with `e`; noted kanban cards show a highlighted `✎` icon instead of inline note text. The selected row is highlighted; `enter` attaches to the actual tmux workspace.

In kanban layouts at least 120 columns wide, a `Campfire` rail appears to the right of the board. Pressing `c` hides or restores the rail; hiding it gives the kanban board the full width, and the preference persists across dashboard runs. The rail reads the global archive plus any live legacy per-session entries, deduplicates and sorts them newest-first, and shows each message's kind, project, relative age, and complete wrapped text. Closing a session leaves its archived messages intact. Narrower layouts keep the full-width board and omit the rail.

The dashboard no longer renders a live agent-pane preview or polls tmux capture output. Agent pane capture remains available to lifecycle/status inference and service-level integrations, but it is not part of the dashboard rendering loop.

## Session names

New sessions have no stored display name and show the project directory name until one is available. Generated Pi routing names (`oak-<id>-architect` and `oak-<id>-junior`) are not display names and do not prevent automatic naming. After the architect's first interactive prompt (or the single agent's prompt), oak-tree attempts to generate a short name in the background using an available inexpensive model from the active provider. Up to 4,000 characters of the prompt are sent to that provider, and the additional call may incur cost. If no suitable model is available or the call fails, the project name remains visible. A manual Pi `/name` takes precedence. Existing Pi processes need an idle `/reload` after the managed extension is refreshed.

## Agent Office

Press `f` from the dashboard, or run `oak-tree office`, to open the local browser office. The dashboard remains available for session management. Oak Athletic uses embedded voxel geometry and pixel textures in a WebGL2 view and needs neither terminal graphics support nor a JavaScript build step. Opening it requires a browser and `xdg-open`.

The gym shows one character per recorded agent, including separate architect and junior characters. All sessions remain selectable regardless of coordination folding. Each agent's own status determines its zone: working uses training, question uses coaching, and ready uses recovery. Session review and blocked tags override active status, using the review corner and help bay. For an active coordination group, a review- or blocked-tagged organizer applies that override to all members. Pending questions remain answerable in any zone. Unparked roles retain their individual states. Unknown status is shown explicitly in the shared circulation area.

Group members share a colored armband, and the roster shows the group name. This affects only the gym, not kanban status or counts. Status labels change immediately; movement to the new zone is illustrative and never delays a question. The inspector includes persisted role tasks and cached session PR/CI metadata; it does not fetch fresh GitHub state. The roster identifies each agent by project, branch and role, with a short session ID when separate sessions share a project and branch. Root sessions are labeled explicitly. On narrow browser windows, the roster and activity feed stack below the scene.

Only the hovered agent (or the selected agent when none is hovered) shows a floating label and activity bubble. Clicking empty gym space or pressing `escape` with the scene focused clears selection and hides the inspector; live refreshes keep the agent deselected until you select one again. The activity sidebar retains messages for all agents.

Bubbles and the inspector show recorded in-progress tasks, Campfire activity from the last ten minutes, or a status fallback. Campfire posts are recorded per session, not per paired agent, so the inspector labels them `Campfire update`. The browser refreshes this data without making AI calls. A disconnected view is explicitly marked; it does not invent new activity.

| Control | Action |
| --- | --- |
| Click a character or roster entry | Select the agent and show its activity |
| Click empty gym space, or scene-focused `escape` | Deselect the agent and hide the inspector |
| Double-click a character | Switch to its session and focus the terminal window in Hyprland |
| Left mouse drag | Orbit the 3D camera |
| Scroll wheel | Zoom |
| Right mouse drag | Pan the camera |
| `f`, with the scene focused | Fit the whole gym |
| Question bubble or `Answer question` | Open the selected agent's live pi-ask form |
| Scene-focused `enter` | Answer a pending question, otherwise open the selected session |
| `Open session` | Focus the selected agent's managed session |

The scene fills the available height without a header, toolbar, room tabs or camera buttons. Walls remain in place. Lighting uses the current instant and Bergen’s coordinates (60.393° N, 5.324° E) to approximate solar elevation using NOAA’s equations. It blends from warm night lighting below −6° (civil twilight) to full daylight above +6°, reflecting winter darkness and long summer evenings. The calculation runs locally without location permission, network calls or timezone/DST jumps, and naturally handles polar day/night. Indoor lamps remain warm at night. This models seasonal sunlight, not clouds or local mountain shadows. It updates each minute, even with reduced motion enabled, and catches up when a hidden tab returns.

Characters use varied, individually timed local routines without changing real agent state. The expanded training wing has eight different stations: barbell squat, deadlift, pec-deck fly, bench press, leg press, lat pulldown, seated cable row and leg extension. There are no duplicate machines. Together with dumbbell curls, dumbbell overhead presses and bodyweight squats, this gives eleven training variations. Working agents reserve one station at a time, complete staggered sets with a short rest, then return to their own floor station before rotating to available equipment. Station choices use a shuffled bag and avoid immediately repeating an exercise. Waiting agents get priority over recent users; overflow agents keep varied free-weight routines. If one partner is working while the other is idle, the idle architect or junior spots the lifter instead of resting. The roster labels this `Spotting · Session busy`; a session is not shown as available while either partner works. Questions, review and blocked states still take precedence. Barbells follow the lifters' grips, while pec-deck handles and weight stacks move with each seated fly. Ready agents stretch or drink, with two reserved places on the recovery sofa. Two review agents use the table while others read clipboards nearby. Questions bring hand raises and short pacing; blocked agents ponder, fold their arms or pace. Breathing, head turns and weight shifts continue between routines. Real tool failures trigger exercise-specific failed lifts: for example a squat tumble, deadlift back-grab or bench-press bailout. A spotter reaches in to help. New questions trigger attention gestures, and both partners celebrate only when the whole session becomes ready after work. These are cosmetic reactions, not invented progress or messages. Historical failure counters are not replayed on first load or a bridge restart.

Pacing stays within an agent's floor slot. State transitions and equipment rotations use clear side lanes and the connecting aisle. Lifters enter each bay from the front, and a departing user must clear the station before another can reserve it. Sofa users approach and leave around its sides rather than walking through it. A capacity increase expands the available floor and reflows characters. Reduced-motion preference keeps agents at their assigned stations, freezes routines and disables camera tweening. Disconnection freezes animation. The previous sprite pickup/arrow-movement controls are replaced by camera orbit controls. Double-clicking opens the session, not a specific agent pane. This requires one tmux client, Hyprland with a Lua configuration, and a uniquely identifiable terminal window; ambiguous or unavailable targets are reported without guessing. Use the terminal dashboard to create or close sessions. If WebGL is unavailable or lost, the roster and existing question, reply, note and session controls remain usable.

Activity bubbles display each newly observed message once for ten seconds. Agents can speak simultaneously without a rotating speaker limit; additional messages from the same agent queue behind its current bubble. Selecting an agent does not replay its old activity. When bubbles cannot fit without overlap, they remain visible rather than suppressing another speaker.

The `Recent activity` sidebar keeps the latest 200 observed updates in local oak-tree state (`office-activity.json`), including messages from sessions that have closed. History survives browser refreshes and office-server restarts; opening the page restores the log without replaying old bubbles. Deduplication remembers the latest 2,000 message identities. `Clear` removes the shared log but retains that deduplication memory. The log records activity observed while the office polls, not a complete Pi transcript; individual entries are capped at 4,000 characters. The left-hand sidebar uses 16px game-style chat lines, subtle timestamps, and stable per-agent name colors that also appear in the roster. It moves below the office on narrower screens.

Select an agent and choose `Latest answer & reply` to read its latest available assistant answer and send a follow-up. A working agent receives the reply through Pi's follow-up queue; an idle agent starts a turn. If delivery cannot be confirmed, check Pi before sending again. The agent must have a live office bridge.

Choose `Sticky notes` to add or delete separate reminders for the selected agent, even while it is working. Notes persist in the oak-tree session record and are not sent to the model. In the corresponding Pi session, use `/oak-notes` to view them when convenient; copy an idea into a prompt when you want to act on it. Each agent can have up to 100 notes of 2,000 characters each. Notes disappear with the oak-tree session record.

The local server is reused between openings and survives closing the dashboard. It binds only to loopback and requires a private token for activity reads, explicit question answers, and session focus requests. Closing the browser does not affect agents; the server stops after thirty minutes without authenticated API requests. The runtime `office.json` file contains the token and must not be shared or committed.

### Answering questions

Browser answers support `@eko24ive/pi-ask` forms through its documented local event contract. After installing an oak-tree update, open the office/dashboard or create a session to refresh the managed extension, then use `/reload` in existing Pi sessions **when idle**. Do not interrupt working agents just to enable the office. Sessions without a live supported bridge still appear in the office; answer their questions in Pi.

The question window shows the actual prompts, stable option values, descriptions, recommendations, and plain-text previews. It supports single/preview choices, multiple choices, custom text, and notes. Required indicators remain advisory, as in Pi. Closing the window preserves its browser draft and does not answer or cancel anything. `Cancel question in Pi` requires explicit confirmation.

Sending replaces any unfinished answers in the terminal form. The first completed response wins. An answered, cancelled, restarted, or unavailable form cannot silently receive a stale response. If delivery cannot be confirmed, the browser does not retry; check Pi. Browser drafts stay in memory, not local storage, and do not synchronize with unfinished terminal edits. If the question type was changed in Pi after opening, Pi's current validation applies.

Office animation and routing use ordinary local code, without agent chatter or prompt instructions. Explicit replies start or queue normal Pi user messages. Answers to live tool questions return through the original waiting tool. Reopened or resumed forms follow the same Pi follow-up behavior—and token usage—as answering them in the terminal. The agent's normal response after your answer still uses tokens.

## Coordination Groups

Sessions can temporarily organize or participate in one active coordination group. Press `g` to create or manage a group, or use the managed Pi `oak_coordination` tool. Kanban folds participant cards by default, preserves complete live counts, and retains selectable shortcuts for folded participants with questions. Manually testing participants remain in the testing strip. `space` shows or folds participants; group details and management appear only in the on-demand dialog. Table view still shows every session.

Assignment progress is explicit (`pending`, `working`, `returned`, `accepted`) and independent of agent lifecycle events. Groups persist in `coordination.json` and refresh with agent status. Closing a group releases its sessions without closing them. Closing an organizer session closes its group; closing a participant removes its assignment and closes the group if no participants remain. See [Coordination groups](coordination.md) for commands and validation.

## Pull Request Metadata

The dashboard reads cached pull request metadata from each session record immediately. After sessions load, it refreshes missing, stale, or pre-unresolved-comment PR metadata for branch-backed sessions in the background. A PR cache is stale after ten minutes.

For automatic refresh:

1. oak-tree uses the current worktree branch when available and falls back to the recorded branch if lookup fails. Git-status refresh persists a changed observed branch and clears the stale PR cache.
2. oak-tree reads the GitHub repository from the workdir's `origin` remote.
3. oak-tree calls `gh pr list` for that repository and branch.
4. oak-tree summarizes review state and status checks, then uses a paginated GitHub GraphQL query to count unresolved review threads.
5. oak-tree writes the result back to the session JSON. If the supplemental thread query fails, the other PR metadata is still cached and the unresolved-comment count is shown as unavailable until the next normal refresh.

Kanban cards show a colored, text-labelled cached PR status before the PR number and other metadata, without needing selection: `MERGED`, `CLOSED`, `DRAFT`, `APPROVED`, `CHANGES` (changes requested), `REVIEW` (review required), or `OPEN` (review unknown). Lifecycle takes priority over approval. Status takes priority over secondary metadata in narrow columns, keeping the existing three-line card density. `APPROVED` describes review only, not CI or merge readiness; the inspector retains those details. These badges use the same cached metadata and refresh timing as the inspector.

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
