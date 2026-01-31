# Robot Mode CLI Design for `gt` (Gas Town) v2

> **Status**: Design Document
> **Repo**: `steveyegge/gastown`
> **Updated**: 2026-01-31
> **Version**: 2.1
> **Contract Version**: 1
> **API Version**: 1
> **Primary audience**: AI coding agents + automation scripts
> **Secondary audience**: humans in a terminal
> **Reference**: Patterns inspired by [cass](https://github.com/Dicklesworthstone/coding_agent_session_search) robot mode

Gas Town (the `gt` Go binary) is a multi-agent workspace manager that persists work state (e.g., hooks) and coordinates agents across rigs.
The CLI is implemented using **Cobra** (`github.com/spf13/cobra`).

This document defines a **robot mode** that makes *every* command predictable, parseable, token-efficient, and recoverable for agent workflows—while keeping human UX intact when running interactively.

---

## 0. Goals and Non-Goals

### Goals (MUST)
1. **JSON output everywhere**: a `--json` flag on *every* command.
2. **Auto JSON when piped**: TTY detection flips to robot output when stdout is not a terminal.
3. **Token-efficient**: compact envelopes, abbreviated keys, no decorative text in robot mode.
4. **Structured errors**: error responses include `code`, `msg`, and ordered `hint[]`.
5. **Meaningful exit codes**: stable, semantic codes for automation.
6. **Discoverability**: schema/commands introspection so agents can explore without docs.
7. **Performance under concurrency**: safe for high fan-out agent usage.

### Non-Goals (for now)
- Replacing human output styling/formatting.
- YAML output.
- Full OpenAPI spec (export path deferred).
- Interactive prompts in robot mode.

---

## 1. Design Principles

### 1.1 Output Philosophy

| Audience | Needs | Human Mode | Robot Mode |
|----------|-------|------------|------------|
| Human | Visual hierarchy, colors, progress | Lipgloss styling, emojis | Disabled |
| AI Agent | Parseable, minimal, structured | N/A | Default |
| Scripts | Exit codes, quiet mode | `--quiet` | Enhanced |

### 1.2 Core Tenets

1. **Stderr for progress, stdout for results** - Agents can ignore stderr
2. **Exit codes are semantic** - Every failure mode has a unique code
3. **JSON is the contract** - Human output is a "pretty-print" of JSON
4. **Fail fast, fail informatively** - Errors include recovery paths
5. **Idempotency declared** - Commands expose side effects explicitly

---

## 2. Activation Modes

### 2.1 Flag Hierarchy (highest wins)

```
--human         Force human mode (styled, decorated)
--robot         Force robot mode (JSON envelope + robot-friendly behavior)
--json          Alias for --robot (for "always JSON" expectation)
--quiet         Suppress non-error output (still sets exit codes)
--robot-format  Output format: json (pretty), jsonl (streaming), compact (single-line)
--robot-meta    Include extended metadata (_meta block)
--robot-help    Deterministic machine-first help (no TUI, no ANSI)
--request-id    Echo ID in response for correlation/tracing
```

### 2.2 TTY Auto-Detection

Robot mode is auto-enabled when stdout is not a terminal:
```bash
gt status | jq         # Robot mode
gt status > out.json   # Robot mode
gt status              # Human mode (TTY)
```

```go
func selectOutputMode(cmd *cobra.Command) OutputMode {
    if flags.Human { return HumanMode }
    if flags.Robot || flags.JSON { return RobotMode }
    if !term.IsTerminal(os.Stdout.Fd()) { return RobotMode }
    return HumanMode
}
```

### 2.3 Environment Variable Override

```bash
GT_OUTPUT_MODE=robot gt status   # Force robot mode
GT_OUTPUT_MODE=human gt status   # Force human mode
GT_OUTPUT_MODE=quiet gt status   # Suppress non-error output
```

---

## 3. Response Envelope

Robot mode output is always a single JSON object per command invocation (unless streaming, see §3.4).

### 3.1 Success Response

```json
{
  "ok": true,
  "cmd": "gt rig list",
  "data": { "...": "..." },
  "meta": { "ts": "2026-01-31T10:30:00Z", "ms": 42, "v": "0.9.0" }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ok` | bool | True if command succeeded |
| `cmd` | string | `cmd.CommandPath()` + args as executed |
| `data` | object | Command-specific payload |
| `meta.ts` | string | ISO 8601 timestamp |
| `meta.ms` | int | Execution duration in milliseconds |
| `meta.v` | string | CLI version |

### 3.2 Error Response

```json
{
  "ok": false,
  "cmd": "gt rig show nope",
  "exit": 3,
  "error": {
    "code": "E_RIG_NOT_FOUND",
    "msg": "Rig 'nope' not found",
    "hint": [
      "Run 'gt rig list' to see available rigs",
      "Check if the rig was removed or renamed"
    ],
    "ctx": {
      "searched": "nope",
      "similar": ["node", "notes"]
    }
  },
  "meta": { "ts": "2026-01-31T10:30:00Z", "ms": 12, "v": "0.9.0" }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `error.code` | string | Machine-readable error code |
| `error.msg` | string | Human-readable message |
| `error.hint` | array | Recovery suggestions (ordered by likelihood) |
| `error.ctx` | object | Debugging context (command-specific) |
| `exit` | int | Exit code (always included in errors) |

### 3.3 List Response (Paginated)

```json
{
  "ok": true,
  "cmd": "gt polecat list --all",
  "data": {
    "items": [ ... ],
    "page": {
      "total": 150,
      "offset": 0,
      "limit": 50,
      "more": true
    }
  },
  "meta": { ... }
}
```

### 3.4 Streaming Response (Long Operations)

For operations like convoys that produce progress, emit newline-delimited JSON (JSONL):

- **stderr**: may contain human progress *only in human mode*
- **stdout** (robot mode): newline-delimited JSON events:

```jsonl
{"ev":"start","op":"gt convoy run","ts":"..."}
{"ev":"prog","pct":25,"msg":"Spawning polecats"}
{"ev":"prog","pct":75,"msg":"Collecting results"}
{"ev":"done","ok":true,"ms":5432,"data":{...}}
```

| Field | Description |
|-------|-------------|
| `ev` | Event type: `start`, `prog`, `done`, `err` |
| `op` | Operation being performed |
| `pct` | Progress percentage (0-100) |
| `msg` | Status message |

### 3.5 Token-Efficiency Rules

- Use abbreviated keys: `ok`, `cmd`, `data`, `meta.ts`, `meta.ms`, `meta.v`, `msg`, `hint`, `ctx`, `ev`, `pct`.
- Omit null/empty fields.
- Prefer stable enums/strings over verbose paragraphs.

**Compact mode** (`--compact`) additionally:
- Omits `meta` entirely
- Flattens simple data structures

```bash
gt status --compact
```
```json
{"ok":true,"agents":3,"rigs":1,"idle":2,"busy":1}
```

---

## 4. Exit Codes

### 4.1 Code Definitions

| Code | Name | Description |
|------|------|-------------|
| 0 | `EXIT_OK` | Success |
| 1 | `EXIT_ERROR` | General/unknown error |
| 2 | `EXIT_USAGE` | Invalid arguments, bad syntax |
| 3 | `EXIT_NOT_FOUND` | Resource doesn't exist |
| 4 | `EXIT_CONFLICT` | Already exists, state conflict |
| 5 | `EXIT_FORBIDDEN` | Permission denied, auth required |
| 6 | `EXIT_TIMEOUT` | Operation timed out |
| 7 | `EXIT_EXTERNAL` | External dependency failure (git, tmux, network, beads) |
| 8 | `EXIT_INTERNAL` | Internal error, bug |
| 10 | `EXIT_PARTIAL` | Partial success (some items failed) |
| 20 | `EXIT_NOOP` | No action taken (already in desired state) |

### 4.2 Exit Code Mapping

The process exit status always matches the table above. Robot mode includes `exit` in error responses for programmatic access.

---

## 5. Error Code Taxonomy

### 5.1 Namespacing

Error codes follow: `E_<DOMAIN>_<SPECIFIC>`

| Domain | Prefix | Examples |
|--------|--------|----------|
| General | `E_` | `E_INVALID_ARG`, `E_TIMEOUT` |
| Workspace | `E_TOWN_` | `E_TOWN_NOT_FOUND`, `E_TOWN_DIRTY` |
| Rig | `E_RIG_` | `E_RIG_NOT_FOUND`, `E_RIG_LOCKED` |
| Polecat | `E_POLECAT_` | `E_POLECAT_NOT_FOUND`, `E_POLECAT_BUSY` |
| Crew | `E_CREW_` | `E_CREW_FULL`, `E_CREW_NOT_RUNNING` |
| Convoy | `E_CONVOY_` | `E_CONVOY_FAILED`, `E_CONVOY_PARTIAL` |
| Hook | `E_HOOK_` | `E_HOOK_NOT_FOUND`, `E_HOOK_CONFLICT` |
| Mail | `E_MAIL_` | `E_MAIL_NO_INBOX`, `E_MAIL_SEND_FAILED` |
| Beads | `E_BEADS_` | `E_BEADS_NOT_FOUND`, `E_BEADS_LOCKED` |
| Git | `E_GIT_` | `E_GIT_CONFLICT`, `E_GIT_DETACHED` |
| Tmux | `E_TMUX_` | `E_TMUX_NOT_RUNNING`, `E_TMUX_SESSION_EXISTS` |
| Auth | `E_AUTH_` | `E_AUTH_REQUIRED`, `E_AUTH_EXPIRED` |
| Internal | `E_INTERNAL_` | `E_INTERNAL_UNREGISTERED` |

### 5.2 Error Registry (Single Source of Truth)

All error codes registered in `internal/robot/errors.go`:

```go
var ErrorRegistry = map[string]ErrorDef{
    "E_RIG_NOT_FOUND": {
        Exit:    EXIT_NOT_FOUND,
        Message: "Rig '%s' not found",
        Hints:   []string{"Run 'gt rig list'", "Check rig name spelling"},
    },
    "E_POLECAT_BUSY": {
        Exit:    EXIT_CONFLICT,
        Message: "Polecat '%s' is busy with another task",
        Hints:   []string{"Wait for current task", "Use 'gt polecat nuke' to force stop"},
    },
    // ...
}
```

Unregistered errors are wrapped as `E_INTERNAL_UNREGISTERED` (exit 8) with debug context.

---

## 6. Quick Start (≤100 Tokens)

### 6.1 Bare Command Output

When `gt` is run with no arguments:

**Human mode (~60 tokens):**
```
gt - Gas Town multi-agent orchestration

Commands:
  start     Start town/rig         status    System state
  polecat   Worker agents          mail      Agent messaging
  sling     Dispatch work          done      Signal completion
  convoy    Batch operations       hook      Current assignment

Flags:
  --robot   Machine output         --json    JSON only
  --help    Full help              --version Version

Run: gt <cmd> --help
```

**Robot mode:**
```json
{"ok":true,"data":{"hint":"Run 'gt --commands' for command list","v":"0.9.0"}}
```

### 6.2 Command Discovery

```bash
gt --commands --robot
```

```json
{
  "ok": true,
  "data": {
    "commands": [
      {"name": "install", "group": "setup", "desc": "Create a town workspace"},
      {"name": "start", "group": "services", "desc": "Start town or rig"},
      {"name": "status", "group": "diag", "desc": "Show system state"},
      {"name": "rig", "group": "rigs", "desc": "Manage rigs"},
      {"name": "polecat", "group": "agents", "desc": "Manage worker agents"}
    ]
  }
}
```

### 6.3 Command Schema

```bash
gt polecat --schema --robot
```

```json
{
  "ok": true,
  "data": {
    "name": "polecat",
    "desc": "Manage worker agents",
    "subcommands": [
      {
        "name": "list",
        "desc": "List polecats",
        "args": [],
        "flags": [
          {"name": "all", "short": "a", "type": "bool", "desc": "Include all rigs"},
          {"name": "json", "type": "bool", "desc": "JSON output"}
        ],
        "examples": ["gt polecat list", "gt polecat list --all"],
        "effects": {"reads": ["rig:*"], "mutates": []}
      }
    ]
  }
}
```

Schema fields:
- `name`, `desc` - command identification
- `args` - positional arguments (name/type/required)
- `flags` - flag definitions
- `examples` - usage examples
- `effects` - declared side effects (see §8)
- `has_json_output` - boolean indicating JSON support

---

## 7. API Versioning and Introspection

*Inspired by cass's comprehensive introspection system.*

### 7.1 Version Contract

Every robot response can include version info (with `--robot-meta`):

```json
{
  "_meta": {
    "api_version": 1,
    "contract_version": "1",
    "v": "0.9.0"
  }
}
```

| Field | Description |
|-------|-------------|
| `api_version` | Integer, increments on breaking changes |
| `contract_version` | String, response schema version |
| `v` | CLI binary version (semver) |

### 7.2 Introspection Commands

#### `gt introspect --json`

Full API schema discovery without documentation:

```json
{
  "api_version": 1,
  "contract_version": "1",
  "global_flags": [
    {"name": "robot", "arg_type": "flag", "description": "Machine-readable output"},
    {"name": "quiet", "short": "q", "arg_type": "flag", "description": "Suppress non-error output"}
  ],
  "commands": [
    {
      "name": "status",
      "description": "Show system state",
      "arguments": [],
      "has_json_output": true
    }
  ],
  "response_schemas": {
    "status": { "type": "object", "properties": {...} }
  }
}
```

#### `gt capabilities --json`

Discover features, versions, and limits:

```json
{
  "crate_version": "0.9.0",
  "api_version": 1,
  "contract_version": "1",
  "features": [
    "json_output", "jsonl_output", "robot_meta",
    "dry_run", "cursor_pagination", "request_id"
  ],
  "limits": {
    "max_polecats": 10,
    "max_convoy_size": 50
  }
}
```

#### `gt health --json`

Fast preflight check (<50ms) for agent startup:

```bash
gt health --json
# Exit 0 = healthy, 1 = unhealthy
```

```json
{
  "healthy": true,
  "latency_ms": 12,
  "state": {
    "town": true,
    "daemon": true,
    "tmux": true,
    "beads": true
  }
}
```

#### `gt robot-docs <topic>`

Machine-focused documentation by topic:

```bash
gt robot-docs commands    # List all commands
gt robot-docs schemas     # Response schemas
gt robot-docs exit-codes  # Exit code reference
gt robot-docs examples    # Usage examples
gt robot-docs env         # Environment variables
```

### 7.3 Status with Recommendations

Status output includes actionable next steps:

```json
{
  "healthy": false,
  "daemon": { "running": false },
  "recommended_action": "Run 'gt daemon start' to start the daemon",
  "_warning": "Daemon not running - agent operations will fail"
}
```

---

## 8. Side Effects and Idempotency

### 8.1 Effect Declaration

Mutating commands declare effects in robot mode using token-efficient keys:

```json
{
  "ok": true,
  "data": { "...": "..." },
  "fx": {
    "c": ["polecat:furiosa-xyz"],
    "m": ["hook:gt-abc"],
    "d": [],
    "x": ["git:push origin polecat/xyz"]
  }
}
```

| Key | Meaning | Examples |
|-----|---------|----------|
| `fx.c` | Created resources | `polecat:xyz`, `rig:myproj` |
| `fx.m` | Modified resources | `hook:gt-abc`, `bead:xyz` |
| `fx.d` | Deleted resources | `session:gt-foo` |
| `fx.x` | External side effects | `git:push`, `tmux:kill-session` |

### 8.2 Dry Run Mode

All mutating commands support `--dry-run`:

```bash
gt polecat nuke foo --dry-run --robot
```

```json
{
  "ok": true,
  "dry_run": true,
  "would": {
    "d": ["session:gt-gastown-foo", "worktree:/path/to/foo"],
    "m": ["bead:agent-foo"]
  },
  "exit": 0
}
```

---

## 9. Agent State Queries

### 9.1 Context Command

AI agents need to understand their current context:

```bash
gt context --robot
```

```json
{
  "ok": true,
  "data": {
    "role": "polecat",
    "rig": "gastown",
    "agent": "furiosa-abc123",
    "hook": "gt-xyz789",
    "branch": "polecat/furiosa-abc123",
    "cwd": "/Users/x/gt/gastown/polecats/furiosa-abc123",
    "town": "/Users/x/gt"
  }
}
```

### 9.2 Capabilities Query

```bash
gt caps --robot
```

```json
{
  "ok": true,
  "data": {
    "can_sling": false,
    "can_mail": true,
    "can_done": true,
    "can_spawn": false,
    "reason": {
      "can_sling": "Polecats cannot dispatch work",
      "can_spawn": "Only witness/mayor can spawn"
    }
  }
}
```

---

## 10. Performance: Preflight and Concurrency

Gas Town is often run concurrently across many agent sessions. There is a known class of contention where `gt` indirectly triggers expensive subprocess checks (especially git) frequently.

### 10.1 Design Requirement

Robot mode must be safe for "high fan-out" usage:
- Many short invocations
- Piped JSON parsing
- Parallel calls from multiple agents

### 10.2 Preflight Policy Matrix

Each Cobra command declares its preflight requirements:

| Policy | Description | Examples |
|--------|-------------|----------|
| `NoPreflight` | No checks needed | `--help`, `--commands`, `--schema` |
| `NeedsTown` | Must be in town workspace | `status`, `rig list` |
| `NeedsRig` | Requires specific rig context | `polecat list`, `witness start` |
| `NeedsGit` | Git operations required | `sling`, `done`, `handoff` |
| `NeedsTmux` | Tmux must be available | `start`, `down` |
| `NeedsBeads` | Beads/bd integration | `hook`, `mail` |

**Rule**: Robot mode + introspection commands (`--commands`, `--schema`, `--help`) must **not** trigger heavy preflight.

```go
type PreflightPolicy struct {
    NeedsTown  bool
    NeedsRig   bool
    NeedsGit   bool
    NeedsTmux  bool
    NeedsBeads bool
}

// Introspection commands use:
var NoPreflightPolicy = PreflightPolicy{}
```

### 10.3 Caching Expensive Checks

For checks like beads version / git identity / fork protection:

| Check | TTL | Storage |
|-------|-----|---------|
| Git identity | 5 seconds | In-memory |
| Beads version | 10 seconds | In-memory |
| Rig discovery | 2 seconds | In-memory |
| Town root | Per-process | Cached on first lookup |

```go
var checkCache = &sync.Map{}

func cachedCheck(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
    if cached, ok := checkCache.Load(key); ok {
        entry := cached.(*cacheEntry)
        if time.Since(entry.at) < ttl {
            return entry.val, entry.err
        }
    }
    val, err := fn()
    checkCache.Store(key, &cacheEntry{val: val, err: err, at: time.Now()})
    return val, err
}
```

---

## 11. Implementation Architecture

### 11.1 Package Structure

```
internal/robot/
├── mode.go        # Mode selection, env, TTY detection
├── envelope.go    # Response/Error/Meta/Effects types
├── output.go      # Output(cmd, data), Fail(cmd, err)
├── errors.go      # Error registry and codes
├── schema.go      # Schema generation from Cobra tree
├── preflight.go   # Preflight policy + caching
└── middleware.go  # Cobra middleware for robot mode
```

### 11.2 Root Command Integration

```go
// root.go - Add global flags
rootCmd.PersistentFlags().Bool("robot", false, "Machine-readable output")
rootCmd.PersistentFlags().Bool("human", false, "Force human-readable output")
rootCmd.PersistentFlags().Bool("json", false, "Alias for --robot")
rootCmd.PersistentFlags().Bool("quiet", false, "Suppress non-error output")
rootCmd.PersistentFlags().Bool("compact", false, "Minimal JSON output")
rootCmd.PersistentFlags().Bool("dry-run", false, "Preview without action")

// root.go - PersistentPreRunE
func persistentPreRun(cmd *cobra.Command, args []string) error {
    robot.SelectMode(cmd)

    // Skip preflight for introspection
    if robot.IsIntrospection(cmd) {
        return nil
    }

    return robot.RunPreflight(cmd)
}
```

### 11.3 Command Refactor Pattern

**Before (typical):**
```go
func runStatus(cmd *cobra.Command, args []string) error {
    status := collectStatus()
    if jsonOutput {
        return json.NewEncoder(os.Stdout).Encode(status)
    }
    printHumanStatus(status)
    return nil
}
```

**After (robot mode pattern):**
```go
func runStatus(cmd *cobra.Command, args []string) error {
    status := collectStatus()
    return robot.Output(cmd, status)
}
```

### 11.4 robot.Output Implementation

```go
func Output(cmd *cobra.Command, data any) error {
    mode := GetMode(cmd)
    start := GetStartTime(cmd)

    resp := &Response{
        OK:   true,
        Cmd:  cmd.CommandPath(),
        Data: data,
        Meta: &Meta{
            TS: time.Now().UTC().Format(time.RFC3339),
            MS: time.Since(start).Milliseconds(),
            V:  version.Version,
        },
    }

    switch mode {
    case ModeRobot, ModeJSON:
        return json.NewEncoder(os.Stdout).Encode(resp)
    case ModeCompact:
        resp.Meta = nil
        return json.NewEncoder(os.Stdout).Encode(resp.Data)
    case ModeHuman:
        return humanOutput(cmd, data)
    case ModeQuiet:
        return nil
    }
    return nil
}
```

### 11.5 Error Handling

```go
// Create structured errors
err := robot.E("E_RIG_NOT_FOUND").
    WithCtx("searched", rigName).
    WithHint("Check 'gt rig list'")

// Cobra wrapper ensures proper output + exit code
func wrapRunE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        if err := fn(cmd, args); err != nil {
            return robot.Fail(cmd, err)
        }
        return nil
    }
}
```

---

## 12. Testing Strategy

### 12.1 Golden Tests for Envelopes

```go
func TestRobotOutput(t *testing.T) {
    out := captureOutput(func() {
        runCommand("gt", "status", "--robot")
    })

    var resp robot.Response
    require.NoError(t, json.Unmarshal(out, &resp))

    assert.True(t, resp.OK)
    assert.Equal(t, "gt status", resp.Cmd)
    assert.NotNil(t, resp.Data)
    assert.NotEmpty(t, resp.Meta.V)
}
```

### 12.2 Exit Code Tests

```go
func TestExitCodes(t *testing.T) {
    tests := []struct {
        args     []string
        wantCode int
    }{
        {[]string{"status"}, 0},
        {[]string{"rig", "show", "nonexistent"}, 3},
        {[]string{"invalid", "command"}, 2},
    }
    for _, tt := range tests {
        code := runCommandExitCode(tt.args...)
        assert.Equal(t, tt.wantCode, code)
    }
}
```

### 12.3 TTY Detection Tests

```go
func TestTTYDetection(t *testing.T) {
    // Pipe stdout
    out := runWithPipedStdout("gt", "status")

    var resp robot.Response
    err := json.Unmarshal(out, &resp)
    assert.NoError(t, err, "piped output should be JSON")
}
```

### 12.4 Token Budget Tests

```go
func TestTokenEfficiency(t *testing.T) {
    out := runCommand("gt", "--robot")
    tokens := countTokens(string(out))
    assert.Less(t, tokens, 100, "Quick start should be under 100 tokens")
}
```

### 12.5 Preflight Performance Tests

```go
func TestIntrospectionNoPreflight(t *testing.T) {
    start := time.Now()
    runCommand("gt", "--commands", "--robot")
    elapsed := time.Since(start)

    assert.Less(t, elapsed, 50*time.Millisecond,
        "Introspection should not trigger slow preflight")
}
```

---

## 13. Migration Strategy

### 13.1 Phase 1: Foundation

- [ ] Create `internal/robot/` package
- [ ] Add persistent flags (`--robot`, `--human`, `--json`, `--quiet`, `--robot-format`, `--robot-meta`, `--robot-help`, `--request-id`, `--dry-run`)
- [ ] Implement TTY detection and mode selection
- [ ] Define response envelope types with `_meta` and `_warning` support
- [ ] Define error code registry
- [ ] Implement preflight policy framework
- [ ] Add `gt health` command (<50ms preflight check)
- [ ] Add `gt introspect` command (full schema)
- [ ] Add `gt capabilities` command (features and limits)
- [ ] Add `gt robot-docs` command (machine-readable docs)

### 13.2 Phase 2: Core Commands

- [ ] Migrate `gt status` to robot mode
- [ ] Migrate `gt rig list/show/add`
- [ ] Migrate `gt polecat list/show/nuke`
- [ ] Migrate `gt mail inbox/send`
- [ ] Migrate `gt hook/unhook`
- [ ] Migrate `gt done`

### 13.3 Phase 3: Full Coverage

- [ ] Migrate remaining commands
- [ ] Add `gt context` command
- [ ] Add `gt caps` command
- [ ] Add `--schema` flag to all command groups
- [ ] Add `--commands` to root

### 13.4 Phase 4: Documentation

- [ ] Generate error code reference from registry
- [ ] Generate command schema reference
- [ ] Add robot mode examples to all `--help`
- [ ] Create AI agent integration guide

---

## 14. Compatibility Notes

### 14.1 Backward Compatibility

- `--json` flag continues to work (alias for `--robot`)
- Human output remains default when TTY detected
- Existing scripts using `--json` need no changes

### 14.2 Breaking Changes

None. Robot mode is opt-in.

### 14.3 Deprecation Path

- `--json` may eventually become alias for `--robot` (it already is)
- No immediate deprecation planned

---

## Appendix A: Full Error Code Reference

| Code | Exit | Description |
|------|------|-------------|
| `E_INVALID_ARG` | 2 | Invalid argument provided |
| `E_MISSING_ARG` | 2 | Required argument missing |
| `E_UNKNOWN_CMD` | 2 | Unknown command or subcommand |
| `E_TOWN_NOT_FOUND` | 3 | Not in a Gas Town workspace |
| `E_RIG_NOT_FOUND` | 3 | Rig does not exist |
| `E_POLECAT_NOT_FOUND` | 3 | Polecat does not exist |
| `E_HOOK_NOT_FOUND` | 3 | Hook/bead does not exist |
| `E_MAIL_NOT_FOUND` | 3 | Mail message not found |
| `E_ALREADY_EXISTS` | 4 | Resource already exists |
| `E_STATE_CONFLICT` | 4 | Operation conflicts with current state |
| `E_NOT_RUNNING` | 4 | Service/agent not running |
| `E_ALREADY_RUNNING` | 4 | Service/agent already running |
| `E_POLECAT_BUSY` | 4 | Polecat engaged with task |
| `E_FORBIDDEN` | 5 | Operation not permitted for role |
| `E_AUTH_REQUIRED` | 5 | Authentication required |
| `E_TIMEOUT` | 6 | Operation timed out |
| `E_GIT_ERROR` | 7 | Git operation failed |
| `E_TMUX_ERROR` | 7 | Tmux operation failed |
| `E_BEADS_ERROR` | 7 | Beads/bd operation failed |
| `E_NETWORK_ERROR` | 7 | Network operation failed |
| `E_INTERNAL` | 8 | Internal error (bug) |
| `E_INTERNAL_UNREGISTERED` | 8 | Unregistered error (debug context included) |
| `E_PARTIAL_SUCCESS` | 10 | Some items succeeded, some failed |
| `E_NOOP` | 20 | No action taken, already in desired state |

---

## Appendix B: Response Type Definitions

```go
// Response is the standard envelope for all robot mode output
type Response struct {
    OK        bool     `json:"ok"`
    Cmd       string   `json:"cmd,omitempty"`
    Data      any      `json:"data,omitempty"`
    Exit      int      `json:"exit,omitempty"`
    Err       *RError  `json:"error,omitempty"`
    Fx        *Effects `json:"fx,omitempty"`
    Meta      *Meta    `json:"_meta,omitempty"`      // With --robot-meta
    Warning   string   `json:"_warning,omitempty"`   // Inline warnings
    DryRun    bool     `json:"dry_run,omitempty"`
    Would     any      `json:"would,omitempty"`
    RequestID string   `json:"request_id,omitempty"` // Echo from --request-id
    RecAction string   `json:"recommended_action,omitempty"` // Actionable next step
}

// RError represents a structured error
type RError struct {
    Code string         `json:"code"`
    Msg  string         `json:"msg"`
    Hint []string       `json:"hint,omitempty"`
    Ctx  map[string]any `json:"ctx,omitempty"`
}

// Meta contains response metadata (included with --robot-meta)
type Meta struct {
    TS              string `json:"ts"`
    MS              int64  `json:"ms"`
    V               string `json:"v"`
    APIVersion      int    `json:"api_version"`
    ContractVersion string `json:"contract_version"`
    RequestID       string `json:"request_id,omitempty"`
}

// Effects describes side effects (token-efficient keys)
type Effects struct {
    C []string `json:"c,omitempty"` // created
    M []string `json:"m,omitempty"` // modified
    D []string `json:"d,omitempty"` // deleted
    X []string `json:"x,omitempty"` // external
}

// Page contains pagination info for list responses
type Page struct {
    Total  int  `json:"total"`
    Offset int  `json:"offset"`
    Limit  int  `json:"limit"`
    More   bool `json:"more"`
}

// PreflightPolicy declares what a command needs
type PreflightPolicy struct {
    NeedsTown  bool `json:"needs_town,omitempty"`
    NeedsRig   bool `json:"needs_rig,omitempty"`
    NeedsGit   bool `json:"needs_git,omitempty"`
    NeedsTmux  bool `json:"needs_tmux,omitempty"`
    NeedsBeads bool `json:"needs_beads,omitempty"`
}
```

---

## Appendix C: Example Agent Session

```bash
# Agent discovers the CLI
$ gt --robot
{"ok":true,"data":{"hint":"Run 'gt --commands' for command list","v":"0.9.0"}}

# Agent lists commands
$ gt --commands --robot
{"ok":true,"data":{"commands":[{"name":"status","group":"diag","desc":"Show system state"},...]}}

# Agent checks context
$ gt context --robot
{"ok":true,"data":{"role":"polecat","rig":"gastown","hook":"gt-xyz"}}

# Agent checks capabilities
$ gt caps --robot
{"ok":true,"data":{"can_sling":false,"can_mail":true,"can_done":true}}

# Agent checks current work
$ gt hook --robot
{"ok":true,"data":{"bead":"gt-xyz","title":"Fix bug #123","step":"implement"}}

# Agent signals completion
$ gt done --robot
{"ok":true,"data":{"mr":"mr-abc","branch":"polecat/xyz"},"fx":{"c":["mr:mr-abc"]}}

# Error example
$ gt rig show nonexistent --robot
{"ok":false,"exit":3,"error":{"code":"E_RIG_NOT_FOUND","msg":"Rig 'nonexistent' not found","hint":["Run 'gt rig list'"]}}
```

---

## Appendix D: Comparison with cass Robot Mode

This design incorporates lessons from the [cass](https://github.com/Dicklesworthstone/coding_agent_session_search) robot mode implementation.

### Adopted from cass

| Feature | cass | gt (this design) |
|---------|------|------------------|
| `--robot-format json\|jsonl\|compact` | ✓ | ✓ |
| `--robot-meta` for optional metadata | ✓ | ✓ |
| `--robot-help` deterministic help | ✓ | ✓ |
| `--request-id` correlation | ✓ | ✓ |
| `_warning` inline warnings | ✓ | ✓ |
| `recommended_action` field | ✓ | ✓ |
| `health` command (<50ms) | ✓ | ✓ |
| `introspect` command | ✓ | ✓ |
| `capabilities` command | ✓ | ✓ |
| `robot-docs` command | ✓ | ✓ |
| Contract versioning | ✓ | ✓ |
| Response schemas in introspect | ✓ | ✓ |
| `has_json_output` per command | ✓ | ✓ |

### Different from cass

| Feature | cass | gt (this design) | Reason |
|---------|------|------------------|--------|
| Envelope wrapper | None (data is root) | `{"ok":true,"data":{...}}` | Explicit success/fail signaling |
| Side effects | Not tracked | `fx.c/m/d/x` | Important for multi-agent coordination |
| `--dry-run` | Search only | All mutating commands | Agent safety |
| Exit codes | Standard | Extended (10=partial, 20=noop) | More granular for automation |

### Key Insights from cass

1. **Introspection is essential**: Agents need `introspect` to discover CLI without docs
2. **Health checks must be fast**: <50ms for preflight validation
3. **Metadata should be optional**: `--robot-meta` keeps output lean by default
4. **Warnings belong in response**: `_warning` is more useful than stderr
5. **Recommended actions help**: Status should include actionable next steps

---

*End of Design Document v2.1*
