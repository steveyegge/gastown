# Robot Mode CLI Premortem Analysis

> **Date**: 2026-01-31
> **Status**: Risk Analysis
> **Related**: `docs/design-robot-mode-cli.md` v2.1

---

## Scenario: 6 Months Later, Robot Mode Has Failed

It's August 2026. Robot mode shipped in March. Adoption is low, agent developers are frustrated, and we're considering ripping it out. Here's what went wrong.

---

## Failure Mode 1: The Envelope Wrapper Was a Mistake

### What Happened
We chose `{"ok":true,"data":{...}}` wrapper while cass uses flat responses. Agents hate the extra nesting:

```python
# Our design forces this:
items = response["data"]["items"]

# cass allows this:
items = response["items"]
```

Every agent has boilerplate to unwrap. Token waste on every response. When `--compact` flattens it, agents break because the schema changed.

### Root Cause
We prioritized explicit success/fail signaling over ergonomics. We didn't realize agents already check exit codes and can detect errors from the `error` field presence.

### Mitigation (Revised Design)
**Option A: Follow cass pattern** - Make data the root, add `_meta`/`_warning`/`error` as optional top-level fields:

```json
// Success (no wrapper)
{"items": [...], "_meta": {"ms": 42}}

// Error (error field present = failure)
{"error": {"code": "E_RIG_NOT_FOUND", "msg": "..."}, "exit": 3}
```

**Option B: Keep envelope but make it consistent** - Never flatten, even in compact mode. Accept the token cost for predictability.

**Decision**: Adopt Option A. The envelope wrapper is unnecessary complexity. Agents can detect errors via:
1. Exit code != 0
2. `error` field present in response

---

## Failure Mode 2: TTY Auto-Detection Breaks CI/CD

### What Happened
Scripts that worked for months suddenly got JSON instead of human output:

```bash
# This worked before:
gt status | grep "running"

# After robot mode shipped, CI/CD breaks:
# Output is JSON, grep finds nothing
```

GitHub Actions, Jenkins, and GitLab CI don't have TTY. Users expected human output in logs but got JSON.

### Root Cause
We assumed "piped = agent" but many humans pipe for filtering. We broke a common workflow.

### Mitigation (Revised Design)
1. **Don't auto-detect by default**. Require explicit `--robot` or `GT_OUTPUT_MODE=robot`
2. **Add `GT_AUTO_ROBOT=1`** env var for agents that want auto-detection
3. **Document the change loudly** in release notes

```go
func selectOutputMode(cmd *cobra.Command) OutputMode {
    if flags.Human { return HumanMode }
    if flags.Robot || flags.JSON { return RobotMode }
    // Only auto-detect if explicitly opted in
    if os.Getenv("GT_AUTO_ROBOT") == "1" && !term.IsTerminal(os.Stdout.Fd()) {
        return RobotMode
    }
    return HumanMode
}
```

---

## Failure Mode 3: Too Many Flags, Cognitive Overload

### What Happened
Users don't know which flag to use:
- `--robot` vs `--json` vs `--robot-format json`?
- `--compact` vs `--robot-format compact`?
- `--robot-meta` - when do I need this?

Documentation says "use `--robot`" but examples show `--json`. Inconsistency everywhere.

### Root Cause
We tried to match cass's flags without considering that gt is simpler. Over-engineering.

### Mitigation (Revised Design)
**Simplify to 3 core flags:**

| Flag | Purpose |
|------|---------|
| `--json` | JSON output (the one flag agents need) |
| `--jsonl` | Streaming JSONL for long operations |
| `--quiet` | Suppress non-error output |

**Remove:**
- `--robot` (alias for `--json`, confusing)
- `--robot-format` (just use `--json` or `--jsonl`)
- `--robot-meta` (always include `_meta`, it's small)
- `--robot-help` (just use `--help --json`)
- `--compact` (premature optimization)

**Keep `--request-id`** - actually useful for tracing.

---

## Failure Mode 4: Introspection Commands Are Never Used

### What Happened
We built `gt introspect`, `gt capabilities`, `gt robot-docs`. Usage metrics: ~0. Agents use AGENTS.md and hardcoded knowledge. The commands rot.

### Root Cause
1. LLMs already have gt knowledge from training data
2. AGENTS.md is in the repo, always up to date
3. Introspection adds latency vs. just knowing the commands

### Mitigation (Revised Design)
1. **Don't build introspection commands in Phase 1**. Wait for user demand.
2. **Invest in AGENTS.md instead** - comprehensive, version-controlled, zero latency
3. **If we build introspection later**, make it a single command: `gt schema --json`

**Remove from Phase 1:**
- `gt introspect`
- `gt capabilities`
- `gt robot-docs`

**Keep:**
- `gt health --json` (actually useful for preflight)

---

## Failure Mode 5: Side Effects Tracking (`fx`) Is Unreliable

### What Happened
Agents trust `fx.c` (created) but:
- Commands report "created" when they actually modified
- Git operations in `fx.x` are wrong
- Partial failures report success with incomplete `fx`

Agents make decisions based on `fx` and get wrong results.

### Root Cause
`fx` is manually maintained per command. Developers forget to update it. No validation that `fx` matches reality.

### Mitigation (Revised Design)
1. **Don't promise accurate side effects**. Document that `fx` is "best effort"
2. **Only track high-confidence effects**: created/deleted resources we control (polecats, hooks)
3. **Don't track external effects** (`fx.x`) - too unreliable
4. **Add integration tests** that verify `fx` matches actual state changes

```go
// Simplified effects - only track what we're confident about
type Effects struct {
    Created []string `json:"created,omitempty"` // Resources we definitely created
    Deleted []string `json:"deleted,omitempty"` // Resources we definitely deleted
}
// No "modified" or "external" - too unreliable
```

---

## Failure Mode 6: Error Code Registry Is Maintenance Burden

### What Happened
We defined 30+ error codes. After 6 months:
- Half are never used
- New errors use E_INTERNAL because developers don't want to add to registry
- Hints are stale ("Run 'gt foo'" but foo was renamed to 'gt bar')
- Nobody looks up error codes programmatically

### Root Cause
Over-engineering. Agents don't parse error codes - they show the message to users or retry.

### Mitigation (Revised Design)
**Simplify to exit codes + message:**

| Exit | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (check `error.msg`) |
| 2 | Usage error |

**Remove granular exit codes** (3-8, 10, 20). They're not used in practice.

**Remove error code registry**. Just use descriptive messages:

```json
{
  "error": {
    "msg": "Rig 'foo' not found. Run 'gt rig list' to see available rigs."
  },
  "exit": 1
}
```

**Keep hints** but embed them in `msg` rather than separate array.

---

## Failure Mode 7: `--dry-run` Lies

### What Happened
`gt polecat nuke foo --dry-run` says "would delete session:gt-foo" but actual run deletes different resources because state changed between dry-run and execution.

Agents use dry-run for planning, execute based on that, and get different results.

### Root Cause
Dry-run computes effects at time T, execution happens at time T+1 when state may have changed.

### Mitigation (Revised Design)
1. **Document that dry-run is advisory**, not a guarantee
2. **Don't use dry-run for agent planning** - agents should be idempotent
3. **Focus dry-run on destructive operations only** (nuke, down --all)
4. **Consider removing dry-run** if it causes more confusion than value

---

## Failure Mode 8: Performance Preflight Caching Doesn't Help

### What Happened
We added 5s TTL caching for git/beads checks. But:
- Each gt invocation is a new process, cache is cold
- Concurrent agents still contend on subprocess spawning
- Preflight policy adds complexity, real bottleneck is tmux/git

### Root Cause
In-process caching doesn't help when each command is a new process.

### Mitigation (Revised Design)
1. **Use file-based cache** in town directory for cross-process benefit
2. **Or: Don't cache at all**. Accept that preflight has a cost.
3. **Focus on reducing preflight scope** instead of caching:
   - `gt status --json` shouldn't check git identity
   - `gt rig list --json` shouldn't validate beads

```go
// Simpler: just skip unnecessary checks for read-only commands
func needsFullPreflight(cmd string) bool {
    readOnly := []string{"status", "rig list", "polecat list", "mail inbox"}
    return !contains(readOnly, cmd)
}
```

---

## Failure Mode 9: Streaming JSONL Is Never Tested

### What Happened
We implemented JSONL for convoy operations. But:
- Convoys are rare (weekly, not hourly)
- Agents don't handle streaming - they wait for completion
- JSONL parsing bugs in agent code cause crashes

### Root Cause
YAGNI. We built streaming because cass has it, not because we needed it.

### Mitigation (Revised Design)
**Remove JSONL from Phase 1**. Convoy can return a single JSON response when complete:

```json
{
  "results": [...],
  "duration_ms": 5432,
  "partial_failures": [...]
}
```

**If streaming is needed later**, add it as a separate flag (`--stream`).

---

## Failure Mode 10: Contract Versioning Is Meaningless

### What Happened
We shipped with `api_version: 1`, `contract_version: "1"`. Six months later, still version 1. We've made changes but didn't bump versions because:
- "It's not really breaking"
- "Agents should handle new fields gracefully"
- Nobody checks the version anyway

Version fields are noise, not signal.

### Root Cause
Versioning is useful when you're willing to break compatibility. We're not - we just add fields.

### Mitigation (Revised Design)
**Remove explicit versioning**. Instead:
1. **Never remove fields** from responses
2. **New fields are always optional**
3. **Document schema changes in CHANGELOG**

Agents should be written to ignore unknown fields. This is more robust than version checking.

---

## Revised Design Summary

Based on this premortem, here are the key changes:

### Remove (Over-Engineering)
- [ ] Envelope wrapper (`{"ok":true,"data":{...}}`) → Flat responses
- [ ] TTY auto-detection → Require explicit `--json`
- [ ] `--robot`, `--robot-format`, `--robot-meta`, `--compact` → Just `--json` and `--jsonl`
- [ ] Granular exit codes (3-20) → Just 0, 1, 2
- [ ] Error code registry → Just descriptive messages
- [ ] `gt introspect`, `gt capabilities`, `gt robot-docs` → Defer to Phase 2+
- [ ] Contract versioning → Add fields, never remove
- [ ] JSONL streaming → Defer, batch responses for now
- [ ] `fx.m` (modified), `fx.x` (external) → Only `created`/`deleted`
- [ ] In-process preflight caching → File-based or skip

### Keep (Actually Useful)
- [x] `--json` flag on every command
- [x] `--quiet` for scripts
- [x] `--request-id` for correlation
- [x] `--dry-run` for destructive operations (with caveats documented)
- [x] `gt health --json` for preflight
- [x] `_meta` block (always included, small overhead)
- [x] `_warning` for inline warnings
- [x] `recommended_action` in status
- [x] `error` object with `msg` field
- [x] `fx.created`/`fx.deleted` (high-confidence only)

### Simplified Response Format

**Success:**
```json
{
  "rigs": ["foo", "bar"],
  "_meta": {"ms": 42, "v": "0.9.0"}
}
```

**Error:**
```json
{
  "error": {"msg": "Rig 'baz' not found. Run 'gt rig list' to see available rigs."},
  "exit": 1
}
```

**With side effects:**
```json
{
  "polecat": "furiosa-xyz",
  "created": ["polecat:furiosa-xyz", "worktree:/path/to/xyz"],
  "_meta": {"ms": 1234, "v": "0.9.0"}
}
```

---

## Risk Matrix (Post-Mitigation)

| Risk | Likelihood | Impact | Mitigation Status |
|------|------------|--------|-------------------|
| Envelope wrapper complexity | ~~High~~ Low | ~~High~~ Low | Removed |
| TTY auto-detect breaks CI | ~~High~~ Low | ~~Medium~~ Low | Opt-in only |
| Flag confusion | ~~High~~ Low | ~~Medium~~ Low | Simplified to 3 flags |
| Unused introspection | ~~High~~ N/A | ~~Low~~ N/A | Deferred |
| Unreliable `fx` | ~~Medium~~ Low | ~~High~~ Medium | Scoped to high-confidence |
| Error code maintenance | ~~High~~ Low | ~~Medium~~ Low | Simplified |
| Dry-run lies | Medium | Medium | Documented as advisory |
| Preflight caching useless | ~~Medium~~ Low | ~~Low~~ Low | Simplified approach |
| JSONL untested | ~~Medium~~ N/A | ~~Medium~~ N/A | Deferred |
| Version meaningless | ~~Medium~~ N/A | ~~Low~~ N/A | Removed |

---

## Recommendation

Update `design-robot-mode-cli.md` to v3 incorporating these simplifications. The core insight: **robot mode should be `--json` and nothing else**. Everything else is premature optimization or over-engineering.

The cass project has more flags because it's a search tool with complex output needs. Gas Town is simpler - commands return small JSON objects. We don't need cass's full complexity.

---

*"Plans are worthless, but planning is everything." - Eisenhower*
