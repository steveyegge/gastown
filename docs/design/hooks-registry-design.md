# Design: Claude Code Hooks Registry and Management

**Bead:** gt-gow8b
**Status:** Design complete, implementation in progress
**Author:** gastown/crew/gus
**Date:** 2026-02-06
**Updated:** 2026-02-26 by gastown/polecats/nux (gap closure)

## Executive Summary

Most of the infrastructure requested in gt-gow8b was implemented by the gt-ja40n
epic (closed 2026-02-06). This design documents what exists, identifies remaining
gaps, and proposes targeted work to close them.

## Current State (Implemented)

### What Exists

| Requirement | Status | Implementation |
|---|---|---|
| Central hook registry | Done | `~/gt/hooks/registry.toml` |
| Base config + overrides | Done | `~/.gt/hooks-base.json` + `~/.gt/hooks-overrides/` |
| Sync all settings.json | Done | `gt hooks sync` |
| Diff before sync | Done | `gt hooks diff` |
| List managed targets | Done | `gt hooks list` |
| Scan existing hooks | Done | `gt hooks scan` |
| Bootstrap from existing | Done | `gt hooks init` |
| Browse registry | Done | `gt hooks registry` |
| Install from registry | Done | `gt hooks install` |
| Per-matcher merge | Done | Same matcher = replace, different = coexist, empty = disable |
| Settings roundtrip | Done | Unknown JSON fields preserved on write |
| gt tap guard | Done | `gt tap guard pr-workflow`, `gt tap guard dangerous-command` |
| gt tap list | Done | `gt tap list` — lists available handlers |

### Architecture

```
~/gt/hooks/registry.toml           <-- Hook definitions (committed to repo)

~/.gt/hooks-base.json              <-- Shared base config (local, per-machine)
~/.gt/hooks-overrides/             <-- Per-role/rig overrides (local)
  crew.json
  witness.json
  gastown__crew.json

~/gt/<rig>/<role>/.claude/settings.json   <-- Generated output (20+ files)
```

**Merge chain:** `base -> role override -> rig+role override`

**Registry vs base/overrides:** The registry (`registry.toml`) is a catalog of
available hooks. The base/overrides system is the active configuration that
generates settings.json files. `gt hooks install` bridges the two by copying
a registry entry into the base or override config.

### Current Registry Hooks (11 defined, 9 enabled)

| Hook | Event | Enabled | Roles |
|---|---|---|---|
| pr-workflow-guard | PreToolUse | Yes | crew, polecat |
| session-prime | SessionStart | Yes | all |
| pre-compact-prime | PreCompact | Yes | all |
| mail-check | UserPromptSubmit | Yes | all |
| costs-record | Stop | Yes | crew, polecat, witness, refinery |
| legacy-beads-guard | PreToolUse | Yes | all |
| bd-init-guard | PreToolUse | Yes | crew, polecat |
| patrol-formula-guard | PreToolUse | Yes | witness |
| task-dispatch-guard | PreToolUse | Yes | mayor |
| clone-guard | PreToolUse | No | crew, polecat |
| dangerous-command-guard | PreToolUse | Yes | crew, polecat |

## Gaps and Proposed Work

### Gap 1: Base/overrides not bootstrapped

The `~/.gt/hooks-base.json` and `~/.gt/hooks-overrides/` don't exist yet.
The settings.json files were manually created and are the source of truth.
Running `gt hooks init` would bootstrap the base/override system from what exists.

**Proposal:** Run `gt hooks init` to extract base + overrides from the 20+
existing settings files. Then `gt hooks sync` becomes the canonical way to
update hooks across the workspace.

**Risk:** Low. `gt hooks init --dry-run` previews first. The init extracts
common hooks as the base and per-target differences as overrides.

### Gap 2: Registry doesn't cover all active hooks — **RESOLVED**

Previously missing hooks have been added to registry.toml:
- **bd-init-guard** — now in registry (enabled)
- **patrol-formula-guard** — now in registry (enabled)
- **legacy-beads-guard** — now in registry (enabled)
- **task-dispatch-guard** — now in registry (enabled)

Remaining environment-specific hooks (tmux-clear, cwd-validation) are
intentionally kept as inline scripts, not registry entries, since they
depend on the local environment.

### Gap 3: No `gt tap` commands beyond pr-workflow — **PARTIALLY RESOLVED**

Implemented:
- `gt tap guard dangerous-command` — blocks rm -rf /, force push, hard reset, git clean -f
  - Full test coverage in `tap_guard_dangerous_test.go`
  - Enabled in registry.toml

Remaining (lower priority, currently handled by inline scripts):
- `gt tap guard bd-init` — inline script works, Go port optional
- `gt tap guard mol-patrol` — handled by DefaultOverrides() in hooks/config.go
- `gt tap audit git-push` — PostToolUse logging, future enhancement

### Gap 4: No `gt tap list` / `gt tap enable` / `gt tap disable` — **PARTIALLY RESOLVED**

Implemented:
- `gt tap list` — lists all tap handlers (guards, audits, etc.) with their
  status, event types, and matchers. Supports `--guards` filter.

Remaining:
- `gt tap disable/enable` — deferred. The existing override mechanism
  (`gt hooks override`) provides this functionality. A convenience wrapper
  can be added if agents need simpler syntax.

### Gap 5: Private vs public hooks (settings.local.json)

Claude Code supports `settings.local.json` (gitignored) for personal overrides.
Gas Town doesn't manage these yet.

**Proposal:** Defer. The current `settings.json` (committed) approach works for
agent-managed worktrees. `settings.local.json` is relevant for human developers
who want personal hook tweaks. Since Gas Town is primarily agent-operated, this
is low priority. If needed later, add `gt hooks override --local` to write to
a `.local` override file.

### Gap 6: Hook ordering

When multiple hooks match the same event+matcher, Claude Code merges arrays.
The order depends on precedence (global -> project -> local). Within a single
settings file, hooks fire in array order.

**Proposal:** No action needed. The current system puts hooks in a deterministic
order (base first, overrides appended/replaced). The per-matcher merge ensures
each matcher has exactly one entry per event type, avoiding ambiguity.

### Gap 7: Shell script hooks

The bead mentions "Joe's shell script hook." The registry already supports
this pattern — the `clone-guard` hook points to `~/gt/hooks/scripts/block-clone-into-town.sh`.

**Proposal:** No architectural change needed. Shell scripts work as hook commands.
The convention is: `~/gt/hooks/scripts/` for standalone scripts, `gt tap <cmd>`
for Go-based hooks. The registry supports both.

## Recommended Execution Order

1. **Bootstrap base/overrides** - `gt hooks init` to establish the base+override
   system from current reality. Verify with `gt hooks diff` showing no changes.

2. **Add missing hooks to registry** - Update `registry.toml` with the 4 missing
   hooks (bd-init-guard, mol-patrol-guard, tmux-clear, cwd-validation).

3. **Implement `gt tap` guards** - Move inline script hooks to Go commands:
   `gt tap guard dangerous-command`, `gt tap guard bd-init`, `gt tap guard mol-patrol`.

4. **Add convenience commands** - `gt tap disable/enable` as thin wrappers around
   the override mechanism.

5. **Integration** - Ensure `gt rig add` and `gt doctor` use the hooks system.

## Decision: Registry as Source of Truth vs Catalog

The registry (`registry.toml`) can serve two roles:

**Option A: Catalog** (current) - Registry lists available hooks. Base/overrides
define what's active. `gt hooks install` copies from registry to base/overrides.
Settings.json is generated from base/overrides.

**Option B: Source of truth** - Registry defines all hooks AND which roles get them.
`gt hooks sync` reads directly from registry.toml to generate settings.json.
No separate base/overrides layer.

**Recommendation: Stay with Option A.** The base/override layer provides:
- Per-machine customization (PATH differences across machines)
- Per-role overrides without polluting the shared registry
- Separation between "what hooks exist" and "what hooks are active where"

The registry is the menu. The base/overrides are the order.

## Non-Goals

- **Parent directory traversal** - Claude Code doesn't traverse parent dirs for
  settings. Until upstream supports this (#12962), we generate per-worktree files.
- **Dynamic hook discovery** - No runtime hook detection. All hooks are statically
  configured in settings.json.
- **Hook marketplace** - No hook sharing between Gas Town instances.
