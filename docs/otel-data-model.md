# Gastown OTel Data Model

All Gastown telemetry events are OTel log records exported via OTLP
(`GT_OTEL_LOGS_URL`). Every record carries a `run.id` attribute — a UUID
generated once per agent spawn — so all records from a single agent session
can be retrieved and correlated.

---

## 1. Identity hierarchy

### 1.1 Instance

The outermost grouping. Derived at agent spawn time from the machine hostname
and the town root directory basename.

| Attribute | Type | Description |
|---|---|---|
| `instance` | string | `hostname:basename(town_root)` — e.g. `"laptop:gt"` |
| `town_root` | string | absolute path to the town root — e.g. `"/Users/pa/gt"` |

### 1.2 Run

Each agent spawn generates one `run.id` UUID. All OTel records for that
session carry the same `run.id`.

| Attribute | Type | Source |
|---|---|---|
| `run.id` | string (UUID v4) | generated at spawn; propagated via `GT_RUN` |
| `instance` | string | `hostname:basename(town_root)` |
| `town_root` | string | absolute town root path |
| `agent_type` | string | `"claudecode"`, `"opencode"`, … |
| `role` | string | `polecat` · `witness` · `mayor` · `refinery` · `crew` · `deacon` · `dog` · `boot` |
| `agent_name` | string | specific name within the role (e.g. `"wyvern-Toast"`); equals role for singletons |
| `session_id` | string | tmux pane name |
| `rig` | string | rig name; empty for town-level agents |

---

## 2. Events

### `agent.instantiate`

Emitted once per agent spawn. Anchors all subsequent events for that run.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `instance` | string | `hostname:basename(town_root)` |
| `town_root` | string | absolute town root path |
| `agent_type` | string | `"claudecode"` · `"opencode"` · … |
| `role` | string | Gastown role |
| `agent_name` | string | agent name |
| `session_id` | string | tmux pane name |
| `rig` | string | rig name (empty = town-level) |
| `issue_id` | string | bead ID of the work item assigned to this agent |
| `git_branch` | string | git branch of the working directory at spawn time |
| `git_commit` | string | HEAD SHA of the working directory at spawn time |

---

### `session.start` / `session.stop`

tmux session lifecycle events.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `session_id` | string | tmux pane name |
| `role` | string | Gastown role |
| `status` | string | `"ok"` · `"error"` |

---

### `prime`

Emitted on each `gt prime` invocation. The rendered formula is emitted
separately as `prime.context` (same attributes plus `formula`).

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `role` | string | Gastown role |
| `hook_mode` | bool | true when invoked from a hook |
| `formula` | string | full rendered formula (`prime.context` only) |
| `status` | string | `"ok"` · `"error"` |

---

### `prompt.send`

Each `gt sendkeys` dispatch to an agent's tmux pane.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `session` | string | tmux pane name |
| `keys` | string | prompt text (opt-in: `GT_LOG_PROMPT_KEYS=true`; truncated to 256 bytes) |
| `keys_len` | int | prompt length in bytes |
| `debounce_ms` | int | applied debounce delay |
| `status` | string | `"ok"` · `"error"` |

---

### `agent.event`

One record per content block in the agent's conversation log.
Only emitted when `GT_LOG_AGENT_OUTPUT=true`.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `session` | string | tmux pane name |
| `native_session_id` | string | agent-native session UUID (Claude Code: JSONL filename UUID) |
| `agent_type` | string | adapter name |
| `event_type` | string | `"text"` · `"tool_use"` · `"tool_result"` · `"thinking"` |
| `role` | string | `"assistant"` · `"user"` |
| `content` | string | content truncated to 512 bytes (set `GT_LOG_AGENT_CONTENT_LIMIT=0` to disable) |

For `tool_use`: `content = "<tool_name>: <truncated_json_input>"`
For `tool_result`: `content = <truncated tool output>`

---

### `agent.usage`

One record per assistant turn (not per content block, to avoid
double-counting). Only emitted when `GT_LOG_AGENT_OUTPUT=true`.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `session` | string | tmux pane name |
| `native_session_id` | string | agent-native session UUID |
| `input_tokens` | int | `input_tokens` from the API usage field |
| `output_tokens` | int | `output_tokens` from the API usage field |
| `cache_read_tokens` | int | `cache_read_input_tokens` |
| `cache_creation_tokens` | int | `cache_creation_input_tokens` |

---

### `bd.call`

Each invocation of the `bd` CLI, whether by the Go daemon or by the agent
in a shell.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `subcommand` | string | bd subcommand (`"ready"`, `"update"`, `"create"`, …) |
| `args` | string | full argument list |
| `duration_ms` | float | wall-clock duration in milliseconds |
| `stdout` | string | full stdout (opt-in: `GT_LOG_BD_OUTPUT=true`) |
| `stderr` | string | full stderr (opt-in: `GT_LOG_BD_OUTPUT=true`) |
| `status` | string | `"ok"` · `"error"` |

---

### `mail`

All operations on the Gastown mail system.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `operation` | string | `"send"` · `"read"` · `"archive"` · `"list"` · `"delete"` · … |
| `msg.id` | string | message identifier |
| `msg.from` | string | sender address |
| `msg.to` | string | recipient(s), comma-separated |
| `msg.subject` | string | subject |
| `msg.body` | string | message body (opt-in: `GT_LOG_MAIL_BODY=true`; truncated to 256 bytes) |
| `msg.thread_id` | string | thread ID |
| `msg.priority` | string | `"high"` · `"normal"` · `"low"` |
| `msg.type` | string | message type (`"work"`, `"notify"`, `"queue"`, …) |
| `status` | string | `"ok"` · `"error"` |

Use `RecordMailMessage(ctx, operation, MailMessageInfo{…}, err)` for operations
where the message is available (send, read). Use `RecordMail(ctx, operation, err)`
for content-less operations (list, archive-by-id).

---

### `agent.state_change`

Emitted whenever an agent transitions to a new state (idle → working, etc.).

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `agent_id` | string | agent identifier |
| `new_state` | string | new state (`"idle"`, `"working"`, `"done"`, …) |
| `hook_bead` | string | bead ID the agent is currently processing; empty if none |
| `status` | string | `"ok"` · `"error"` |

---

### `mol.cook` / `mol.wisp` / `mol.squash` / `mol.burn`

Molecule lifecycle events emitted at each stage of the formula workflow.

**`mol.cook`** — formula compiled to a proto (prerequisite for wisp creation):

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `formula_name` | string | formula name (e.g. `"mol-polecat-work"`) |
| `status` | string | `"ok"` · `"error"` |

**`mol.wisp`** — proto instantiated as a live wisp (ephemeral molecule instance):

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `formula_name` | string | formula name |
| `wisp_root_id` | string | root bead ID of the created wisp |
| `bead_id` | string | base bead bonded to the wisp; empty for standalone formula slinging |
| `status` | string | `"ok"` · `"error"` |

**`mol.squash`** — molecule execution completed and collapsed to a digest:

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `mol_id` | string | molecule root bead ID |
| `done_steps` | int | number of steps completed |
| `total_steps` | int | total steps in the molecule |
| `digest_created` | bool | false when `--no-digest` flag was set |
| `status` | string | `"ok"` · `"error"` |

**`mol.burn`** — molecule destroyed without creating a digest:

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `mol_id` | string | molecule root bead ID |
| `children_closed` | int | number of descendant step beads closed |
| `status` | string | `"ok"` · `"error"` |

---

### `bead.create`

Emitted for each child bead created during molecule instantiation
(`bd mol pour` / `InstantiateMolecule`). Allows tracing the full
parent → child bead graph for a given molecule.

| Attribute | Type | Description |
|---|---|---|
| `run.id` | string | run UUID |
| `bead_id` | string | newly created child bead ID |
| `parent_id` | string | parent (wisp root / base) bead ID |
| `mol_source` | string | molecule proto bead ID that drove the instantiation |

---

### Other events

All carry `run.id`.

| Event body | Key attributes |
|---|---|
| `sling` | `bead`, `target`, `status` |
| `nudge` | `target`, `status` |
| `done` | `exit_type` (`COMPLETED` · `ESCALATED` · `DEFERRED`), `status` |
| `polecat.spawn` | `name`, `status` |
| `polecat.remove` | `name`, `status` |
| `formula.instantiate` | `formula_name`, `bead_id`, `status` (top-level formula-on-bead result) |
| `convoy.create` | `bead_id`, `status` |
| `daemon.restart` | `agent_type` |
| `pane.output` | `session`, `content` (opt-in: `GT_LOG_PANE_OUTPUT=true`) |

---

## 3. Recommended indexed attributes

```
run.id, instance, town_root, session_id, rig, role, agent_type,
event_type, msg.thread_id, msg.from, msg.to
```

---

## 4. Environment variables

| Variable | Set by | Description |
|---|---|---|
| `GT_RUN` | tmux session env + subprocess | run UUID; correlation key across all events |
| `GT_OTEL_LOGS_URL` | daemon startup | OTLP logs endpoint URL |
| `GT_OTEL_METRICS_URL` | daemon startup | OTLP metrics endpoint URL |
| `GT_LOG_AGENT_OUTPUT` | operator | opt-in: stream Claude JSONL conversation events (content truncated to 512 bytes by default) |
| `GT_LOG_AGENT_CONTENT_LIMIT` | operator | override content truncation in `agent.event`; set `0` to disable (experts only) |
| `GT_LOG_BD_OUTPUT` | operator | opt-in: include bd stdout/stderr in `bd.call` records |
| `GT_LOG_PANE_OUTPUT` | operator | opt-in: stream raw tmux pane output |
| `GT_LOG_MAIL_BODY` | operator | opt-in: include mail body in `mail` records (truncated to 256 bytes) |
| `GT_LOG_PROMPT_KEYS` | operator | opt-in: include prompt text in `prompt.send` records (truncated to 256 bytes) |
| `GT_LOG_PRIME_CONTEXT` | operator | opt-in: log full rendered formula in `prime.context` records |

`GT_RUN` is also surfaced as `gt.run_id` in `OTEL_RESOURCE_ATTRIBUTES` for `bd`
subprocesses, correlating their own telemetry to the parent run.
