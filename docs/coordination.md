# Coordination groups

A coordination group links one organizer session to participant sessions for a shared change. The relationship is temporary. It does not change tmux sessions, branches, PRs, or how agents send messages.

## Dashboard

Select a session and press `g`:

- For an ungrouped session, enter a group name. Select participants with `space` and press `enter` to create the group. The selected session becomes the organizer. Each initial assignment uses the group name as its task; edit individual assignments in the dialog.
- For a grouped session, open its group dialog. The current session is selected in the member list.

Kanban folds participant cards by default. The group name is the organizer card's main heading and can wrap onto two lines, followed by its branch, PR metadata, participant count, and returned-result count. Question counts appear only when nonzero. Summary and column counters still count all sessions, including folded participants. Affected columns explain how many sessions are folded into groups, so a nonzero count without a full session card is clear.

A folded participant with a live `QUESTION` appears as a compact, selectable shortcut in the QUESTION column. `enter` attaches to that participant; `g` opens its group. A grouped participant's live question remains in QUESTION even if it has a parked manual tag, whether the group is folded or expanded. This keeps question counts stable. Manually testing participants without a question remain reachable in the testing strip. An organizer in that strip also shows its group summary.

Press `space` on a grouped card to show or fold its participants. Expanded participants appear in their normal status columns. Folding a selected participant moves selection to its organizer, unless it is a question shortcut or a manually testing session. Expansion is local to the dashboard and resets when it restarts. Table view continues to show all sessions; `g` works there too.

The on-demand group dialog has these commands:

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Select a member |
| `enter` | Attach to the selected member |
| `tab` | Select the organizer |
| `+` or `=` | Add an available participant |
| `-` | Remove the selected participant |
| `o` | Swap the selected participant with the organizer |
| `n` | Rename the group |
| `e` | Replace the selected assignment |
| `w` | Mark the selected assignment working |
| `r` | Enter and return a result |
| `a` | Accept a returned result |
| `pgup` / `pgdown` | Read long assignment or result text |
| `c`, then `y` | Close the group and unfold participants |
| `esc` | Go back or return to the board |

Swapping the organizer gives the former organizer a new pending assignment named after the shared change. The new organizer leaves the participant assignment list. Replacing an assignment resets its progress and clears its old result. Removing the last participant closes the group.

## Assignment progress is separate from agent status

Assignments have four explicit states:

```text
pending → working → returned → accepted
```

A result is required to report `returned`. The organizer must explicitly accept a returned result. A participant can report pending or working again when more work is needed. A session becoming READY, stopping, or asking a question never automatically completes an assignment. A participant's live agent state and its assignment progress appear separately in the dialog. The returned count includes accepted results.

## Agent tool and CLI

Registered managed Pi sessions receive `oak_coordination`. Agents first use `action: "list"` to discover exact oak-tree session IDs. These are not Pi or intercom IDs. The list includes repository, branch, and tmux session names so the agent can match communication peers to managed sessions.

The tool supports:

- `create`: Set `name` and `assignments: [{session_id, task}]`. The calling session is the organizer by default.
- `list`: List active groups and managed sessions. Set `group_id` to limit output to one group and its members.
- `update`: Set `group_id` and a new `name` or `organizer_id`. A new organizer must already be a participant.
- `assign`: Set `group_id`, `session_id`, and `task` to add or replace an assignment.
- `report`: Set `group_id`, `status` (`pending`, `working`, or `returned`), and optional `result`. The caller's session is the default `session_id`.
- `accept`, `remove`: Set `group_id` and the participant's `session_id`.
- `close`: Set `group_id`.

Agents can manage only groups they organize. Participants can report only their own assignments. Users can manage either role through the dashboard or CLI. Each session can belong to one active group, preventing conflicting organizers and nested folding. Closed groups do not prevent new groups.

The tool records coordination metadata. It does not send instructions, deliver returned results, or trigger other agents. Continue using intercom for those messages, including the group ID when delegating. The organizing agent must explicitly register a group; ordinary intercom messages do not create one automatically. Tool output is limited to 50 KiB or 2000 lines and reports truncation.

The CLI uses the same service and validation:

```sh
oak-tree coordination

oak-tree coordination --request '{"action":"create","name":"Shared validation","organizer_id":"ORGANIZER_ID","assignments":[{"session_id":"PARTICIPANT_ID","task":"Apply the shared validation"}]}'

oak-tree coordination --request '{"action":"close","group_id":"GROUP_ID"}'
```

The managed tool supplies `--oak-session` or resolves its registered tmux pane with `--tmux-pane`. CLI calls without either flag are user actions. Session identity is local attribution, not a security boundary against other processes running as the same operating-system user.

## Persistence and lifecycle

Groups are stored in `coordination.json` under the oak-tree state directory. Records contain a group ID, name, organizer ID, participant assignments and results, timestamps, and an optional `closed_at`. The file uses owner-only permissions and atomic replacement. A cross-process file lock protects the complete read-modify-write operation, so concurrent participant reports do not overwrite one another. Group updates do not change a session's live status or reorder session records.

The dashboard refreshes groups with its one-second agent-status refresh. An older refresh cannot overwrite a newer group mutation already seen by that dashboard. A missing organizer never leaves participants hidden.

Closing a group retains its record and unfolds its sessions. It never closes sessions or removes branches. Closing an organizer session closes its active group and releases the participants. Closing a participant session removes it from the active group; removing the final participant closes that group. Existing session-close safeguards still apply before this cleanup.

After rebuilding and installing oak-tree, reopen the dashboard. Existing Pi processes need `/reload` to load the new managed tool after oak-tree has refreshed its auto-discovered extension. Replacement Pi processes can use `/oak-tree register` as usual.
