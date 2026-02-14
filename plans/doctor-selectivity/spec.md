# doctor-selectivity — Design Specification

**Created:** 2026-02-14
**Status:** Validated
**Brainstorming Mode:** With scope questions (P0+P1, 45 questions)

---

## Overview

Add support for selective doctor execution: `gt doctor <check-name>` runs only the
specified check(s) instead of the full suite (~72 base checks, ~83 with rig checks).
This addresses three validated pain points: speed (full runs are slow), noise (users
care about one check but see all of them),
and surgical fixing (users know what's broken and want to fix it directly).

The feature adds positional arguments to the existing `gt doctor` command, a `list`
subcommand for discoverability, and category-based filtering. It preserves full backward
compatibility — `gt doctor` with no arguments continues to run all checks exactly as before.

**Target users:** Gas Town developers and operators who run `gt doctor` for diagnostics
and maintenance. Both interactive and scripted/CI use cases.

---

## Scope Questions & Answers

### Summary
- **Questions addressed:** 45 (P0 + P1)
- **Auto-answered (best practices):** 22
- **Human decisions:** 23
- **Deferred to future:** 19 (P3 parking lot)

### P0: Critical Decisions

| # | Question | Answer | How Decided |
|---|----------|--------|-------------|
| 1 | Does `gt doctor <name>` also fix, or is `--fix` required? | `--fix` still required | Auto: Codebase separates `RunStreaming`/`FixStreaming`. Safety model preserved. |
| 2 | Scoped execution or filtered output? | Scoped execution (run only targeted checks) | Human choice — fast, isolated. Checks verified independent in code. |
| 3 | Backward compatible with no args? | Yes, 100% | Auto: Cobra positional args are optional by design. |
| 4 | Primary discovery mechanism? | `gt doctor list` subcommand, grouped by category | Human choice |
| 5 | What does `gt doctor list` show? | Name + description + category + fixable icon, grouped by category | Human choice |
| 6 | Error for unknown check name? | Error + "did you mean?" suggestions + "run `gt doctor list`" | Human choice |
| 7 | Positional arg or flag? | Positional arg (`gt doctor orphan-sessions`) | Human choice — natural, concise |
| 8 | Multiple check names? | Yes (`gt doctor orphan-sessions wisp-gc`) | Human choice |
| 9 | Category filtering? | Yes, v1 (`gt doctor cleanup` runs all Cleanup checks) | Human choice — categories already defined in code |
| 10 | Check dependencies / isolation safety? | No dependencies. Checks are independent. | Auto: Verified in `RunStreaming` — no shared state. |
| 11 | Confirmation prompt before fix? | No prompt. `--fix` is sufficient consent. | Human choice — consistent with current behavior, scriptable |
| 12 | Single-check output format? | Simplified — single result line + details, no headers/summary | Human choice |
| 13 | Summary line for single check? | No summary block for 1 check | Cascaded from Q12 |
| 14 | Performance target? | Sub-second for single checks | Cascaded from Q2 (scoped execution) |
| 15 | Fuzzy matching? | Exact match + "did you mean?" suggestions | Human choice — safe, clear |
| 16 | Input normalization? | Normalize hyphens/underscores + case-insensitive | Human choice |
| 17 | Exit codes consistent? | Yes, non-zero on errors | Auto: Same `report.HasErrors()` path. |
| 18 | Non-TTY output? | Auto-detect, strip ANSI codes and streaming for pipes | Human choice |

### P1: Important Decisions

| # | Question | Answer | How Decided |
|---|----------|--------|-------------|
| 19 | `--fix` on non-fixable check? | Check runs, shows result. Notes "does not support auto-fix". | Auto: `CanFix()` guard in code. |
| 20 | Auto re-run after fix? | Yes, already implemented | Auto: `FixStreaming` re-runs after `Fix()`. |
| 21 | Partial fix success? | Reports what succeeded/remains via re-run | Auto: Per-item fixes + verification. |
| 22 | Immediate feedback? | Yes, streaming "○ name..." before result | Auto: Existing pattern. |
| 23 | All flags compose? | Yes (`--verbose`, `--slow`, `--rig`, etc.) | Auto: Flags orthogonal to selection. |
| 24 | Rig check without `--rig`? | "Unknown check" + did-you-mean + "--rig" hint. Rig checks not registered without flag, so they appear as unknown. | Review: Updated from original — rig checks are conditionally registered. |
| 25 | Verbose default for targeted? | No, keep `--verbose` opt-in | Human choice — targeting = focus, not depth |
| 26 | Tab completion? | Yes, in v1 | Human choice — Cobra has built-in support |
| 27 | Help text update? | Prominent — update Usage line + examples | Auto: Standard practice. |
| 28 | Hint in full output? | Yes, subtle one-time tip | Auto: Drives adoption. |
| 29 | `gt doctor all`? | Error + redirect to `gt doctor` with no args | Auto |
| 30 | Reserved words? | Reserve `list` as subcommand, `all` as redirect to no-args | Auto: No check name conflicts. |
| 31 | Ctrl+C during fix? | Safe — per-item atomic fixes | Auto: Existing check design. |
| 32 | Concurrent runs? | No locking — checks are idempotent | Auto |
| 33 | Outside workspace? | Already handled: clear error | Auto |
| 34 | Real problem? | All three: speed + noise + surgical fixing | Human choice |
| 35-36 | Better output or `gt fix`? | Selective doctor addresses all pain points | Cascaded from Q34 |
| 37 | Visual scope signaling? | Header line: "Running: orphan-sessions" | Human choice |
| 38 | List shows fixable? | Yes, 🔧 icon | Auto: `CanFix()` available on all checks. |
| 39 | Stale check names? | "Did you mean?" + `gt doctor list` | Auto |
| 40 | Check descriptions? | Yes, `Description()` already exists. Surface in list. | Auto |
| 41 | Passing check output? | Single informative line, no summary | Auto |
| 42 | Cleanup in doctor? | Yes, stays in doctor | Cascaded from Q34 |
| 43 | Flag ordering? | Both work (Cobra handles this) | Auto |
| 44 | Wildcard/glob support? | Defer to v2 | Implicit — not raised in dialogue |
| 45 | Dry-run mode? | Yes, `--dry-run` in v1 | Human choice — safety feature worth cost |

### Deferred (P3 Parking Lot)

| # | Question | Defer Reason |
|---|----------|-------------|
| 71 | Exclusion patterns (`--except`) | v2 — inverse of selection |
| 72 | Saved check profiles | v2 — power user feature |
| 73 | Result caching | v2 — optimization |
| 76 | Re-run failed checks | v2 — requires state persistence |
| 77 | Scheduled/cron execution | Separate feature |
| 83 | List shows currently-failing | Requires cached state |
| 86 | JSON output for listing | v2 |
| 89 | Drill-down mode (more detail than full run) | v2 |

---

## Design

### Architecture Overview

Selective execution adds a filtering layer between check registration and check execution.
Filtering happens in `cmd/doctor.go` before calling `RunStreaming` or `FixStreaming`.

**Architectural assumption:** Checks are self-contained — each check's `Run()` and `Fix()`
operate only on the check's own struct state (e.g., `missingRigs []string` populated by
`Run()` and consumed by `Fix()`). No check reads results from other checks. This is verified
in the codebase and is a requirement for selective execution to work safely.

```
User Input → Parse Args → Filter Checks → Doctor.RunStreaming/FixStreaming → Report
                ↓                ↓
          check names      register only
          category names   matching checks
          (or none = all)
```

### CLI Grammar

```
gt doctor                              # Run all checks (existing behavior)
gt doctor <check-name>                 # Run one check
gt doctor <check> <check> ...          # Run multiple checks
gt doctor <category>                   # Run all checks in category
gt doctor <check> --fix                # Run + fix one check
gt doctor <check> --fix --dry-run      # Preview what fix would do
gt doctor list                         # Show available checks
gt doctor --fix                        # Fix all (existing behavior)
```

**Routing:** `list` is a Cobra subcommand (child of `doctorCmd`). Cobra routes
`gt doctor list` automatically before `runDoctor` is called. No special handling
in `runDoctor` needed for `list`.

**Note:** `cobra.EnablePrefixMatching` is enabled globally (root.go:257), so
`gt doctor l` will prefix-match to `list`. No current check names start with `l`
that could conflict, but future check names should avoid the `list` prefix.

**Argument resolution order (in `runDoctor`):**
1. If arg matches an exact check name (after normalization) → select that check
2. If arg matches a category name (case-insensitive, matched against display names like "Cleanup", "Core") → select all checks in category
3. Otherwise → error with "did you mean?" suggestions (Levenshtein distance ≤ 2, show up to 3 suggestions)

**Input normalization:**
- Case-insensitive: `Orphan-Sessions` → `orphan-sessions`
- Underscore/hyphen equivalence: `orphan_sessions` → `orphan-sessions`
- Canonical form is kebab-case (as defined by `check.Name()`)

### Components

#### 1. Check Filter (`internal/doctor/filter.go`)

New file. Responsible for:
- Matching user input to registered checks (by name or category)
- Input normalization (case, hyphens/underscores)
- "Did you mean?" suggestions using Levenshtein distance
- Validating that all requested names resolved to checks

```go
// FilterResult holds the outcome of filtering checks
type FilterResult struct {
    Matched   []Check   // Checks that matched the filter
    Unmatched []string  // Input names that didn't match any check
}

// FilterChecks filters registered checks by name or category
func FilterChecks(checks []Check, args []string) (*FilterResult, error)

// SuggestCheck returns up to 3 closest check names within edit distance ≤ 2
func SuggestCheck(checks []Check, input string) []string

// NormalizeName converts input to canonical kebab-case form
func NormalizeName(input string) string
```

#### 2. List Subcommand (`internal/cmd/doctor_list.go`)

New file. Implements `gt doctor list`.

**Output format (grouped by category):**
```
Core
  town-config-exists       Check mayor/town.json exists
  town-config-valid        Check mayor/town.json is valid
  rigs-registry-exists  🔧 Check mayor/rigs.json exists
  ...

Cleanup
  orphan-sessions       🔧 Detect orphaned tmux sessions
  orphan-processes      🔧 Detect orphaned Claude processes
  wisp-gc               🔧 Detect and clean abandoned wisps (>1h)
  stale-beads-redirect  🔧 Detect stale files in .beads directories
  ...

Rig (requires --rig)
  rig-is-git-repo          Verify rig is a valid git repository
  ...
```

🔧 icon indicates fixable checks (`CanFix() == true`).

**`--rig` interaction:** Without `--rig`, rig checks are shown in a separate section with
note "Requires --rig flag". With `--rig <name>`, rig checks are shown alongside other checks.

#### 3. Modified Doctor Command (`internal/cmd/doctor.go`)

Changes to `runDoctor`:
- Parse positional args as check names/categories
- Register all checks as before, then filter to selected set
- For single-check: simplified output (header + result line)
- For multi-check/category: same streaming format, adapted scope
- `--dry-run` flag: shows what fix would do without applying

#### 4. Shell Completion (in `internal/cmd/doctor.go`)

Set `ValidArgsFunction` on `doctorCmd` (no separate file needed):
- Returns all check names + category names for tab completion
- Cobra's built-in `ValidArgsFunction` mechanism
- Categories are completable alongside check names

#### 5. Dry-Run Mode

New flag `--dry-run` (only meaningful with `--fix`):
- Implemented in `cmd/doctor.go`, NOT in `doctor.go` core
- When `--fix --dry-run`: calls `RunStreaming` (not `FixStreaming`), then renders
  custom "Would fix:" output based on the check results
- This preserves the "minimal core changes" principle
- Reports what would be fixed without calling `Fix()`
- Output: "Would fix: 5 orphaned sessions" with details

### Output Formats

#### Single Check — Pass
```
Running: orphan-sessions

  ✓  orphan-sessions  no orphaned sessions found
```

#### Single Check — Warning + Fix
```
Running: orphan-sessions

  ⚠  orphan-sessions  5 orphaned sessions found (fixing)...
  🔧 orphan-sessions  5 orphaned sessions cleaned (fixed)
```

#### Single Check — Non-Fixable with --fix
```
Running: town-config-valid

  ⚠  town-config-valid  mayor/town.json has invalid JSON
     └─ This check does not support auto-fix.
```

#### Single Check — Dry Run
```
Running: orphan-sessions (dry run)

  ⚠  orphan-sessions  5 orphaned sessions found
     └─ Would fix: kill 5 orphaned tmux sessions
```

#### Category Run
```
Running: Cleanup (4 checks)

  ✓  orphan-sessions       no orphaned sessions found
  ⚠  orphan-processes      2 orphaned Claude processes
  ✓  wisp-gc               no abandoned wisps
  ✓  stale-beads-redirect  no stale redirect files

─────────────────────────────────
✓ 3 passed  ⚠ 1 warning  ✗ 0 failed
```

#### Unknown Check Name (single)
```
Error: unknown check "orphen-sessions"

  Did you mean: orphan-sessions?

  Run "gt doctor list" to see all available checks.
```

#### Unknown Check Names (multiple)
```
Error: unknown checks "foo", "bar"

  Run "gt doctor list" to see all available checks.
  Some checks require --rig. Run "gt doctor list --rig <name>".
```

#### Non-TTY Output (piped)
All ANSI codes stripped. No carriage return overwrites. Clean line-by-line output:
```
Running: orphan-sessions
PASS  orphan-sessions  no orphaned sessions found
```

#### Non-TTY Category Run (piped)
```
Running: Cleanup (4 checks)
PASS  orphan-sessions       no orphaned sessions found
WARN  orphan-processes      2 orphaned Claude processes
PASS  wisp-gc               no abandoned wisps
PASS  stale-beads-redirect  no stale redirect files
3 passed  1 warning  0 failed
```

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Unknown check name | Error + Levenshtein suggestion + "gt doctor list" |
| Rig check without `--rig` | Error: "unknown check 'X'" + did-you-mean suggestions + "Some checks require --rig. Run `gt doctor list --rig <name>`." (Rig checks are not registered without `--rig`, so they appear as unknown.) |
| `--fix` on non-fixable | Check runs normally, notes "does not support auto-fix" |
| `--dry-run` without `--fix` | Ignored (dry-run only meaningful with `--fix`) |
| `gt doctor all` | Error: "unknown check 'all'. Run `gt doctor` with no arguments." |
| `gt doctor list --fix` | `--fix` ignored, list shown normally |
| Outside workspace | Existing error: "not in a Gas Town workspace" |
| Empty/whitespace args | Treated as no args → full run |

### Exit Codes

| Scenario | Exit Code |
|----------|-----------|
| All checks pass | 0 |
| Warnings only (no errors) | 0 |
| Any check has errors | 1 |
| Unknown check name | 1 |
| `--dry-run` with issues found | 0 (no action taken) |

### Non-TTY Detection

Detect via existing `ui.IsTerminal()` (`internal/ui/terminal.go:96`, uses `golang.org/x/term`).
TTY state is determined in `cmd/doctor.go` and passed to `RunStreaming`/`FixStreaming`
via an `isTTY bool` parameter (or via a writer wrapper that strips ANSI/`\r`).

When non-TTY:
- No ANSI color codes
- No carriage return streaming (no `\r` overwrites in `RunStreaming`/`FixStreaming`)
- Status as text prefix: `PASS`, `WARN`, `FAIL`, `FIXED`
- No icons (no ✓, ⚠, ✗, 🔧, ⏳)

**Note:** This requires modifying `internal/doctor/doctor.go` to accept TTY state.
The `RunStreaming`/`FixStreaming` methods use `\r` carriage returns (doctor.go lines 91, 204)
that must be conditionally disabled.

---

## File Changes

### New Files
| File | Purpose |
|------|---------|
| `internal/doctor/filter.go` | Check filtering, name matching, suggestions |
| `internal/doctor/filter_test.go` | Tests for filtering |
| `internal/cmd/doctor_list.go` | `gt doctor list` subcommand |

### Modified Files
| File | Changes |
|------|---------|
| `internal/cmd/doctor.go` | Add arg parsing, check filtering, `--dry-run` flag, non-TTY detection, simplified output for targeted runs |
| `internal/doctor/doctor.go` | Add `isTTY` parameter to `RunStreaming`/`FixStreaming` to conditionally disable `\r` overwrites and ANSI codes when piped |
| `internal/doctor/types.go` | Add `Category() string` to `Check` interface (currently behind unexported `categoryGetter` type assertion in doctor.go:39). All checks already implement it via `BaseCheck`/`FixableCheck`. |

### Estimated Scope
- ~500 lines new code (filter.go + doctor_list.go + non-TTY + dry-run rendering)
- ~200 lines modified (cmd/doctor.go + doctor.go TTY support + types.go Category())
- ~400 lines tests
- Shell completion: ~30 lines (in cmd/doctor.go)

---

## Acceptance Criteria

These scenarios define "done" for v1:

| # | Scenario | Expected Result |
|---|----------|-----------------|
| 1 | `gt doctor orphan-sessions` | Runs only orphan-sessions check in <1s, simplified output |
| 2 | `gt doctor orphan-sessions wisp-gc` | Runs both checks, streaming output with summary |
| 3 | `gt doctor cleanup` | Runs all Cleanup-category checks with "Running: Cleanup (N checks)" header |
| 4 | `gt doctor orphan-sessions --fix` | Runs check, fixes issues, re-runs to verify |
| 5 | `gt doctor orphan-sessions --fix --dry-run` | Runs check, shows "Would fix: ..." without acting |
| 6 | `gt doctor orphen-sessions` | Error: unknown check + "Did you mean: orphan-sessions?" + "Run gt doctor list" |
| 7 | `gt doctor list` | Shows all checks grouped by category with 🔧 icons for fixable |
| 8 | `gt doctor` (no args) | Full check run (~72 checks), identical to current behavior |
| 9 | `gt doctor --fix` (no args) | Fix all, identical to current behavior |
| 10 | `gt doctor orphan_sessions` | Normalizes to `orphan-sessions`, runs check |
| 11 | `gt doctor Orphan-Sessions` | Case-insensitive match, runs check |
| 12 | `gt doctor all` | Error: unknown check "all" + redirect to `gt doctor` |
| 13 | `gt doctor <rig-check>` (no --rig) | Error: "unknown check" + did-you-mean + "Some checks require --rig" hint |
| 14 | `gt doctor orphan-sessions \| cat` | Non-TTY: no ANSI codes, text prefixes (PASS/WARN/FAIL) |
| 15 | Tab-completing `gt doctor or<TAB>` | Completes to `orphan-sessions` (and other matches) |
| 16 | `gt doctor town-config-valid --fix` | Runs check, shows "does not support auto-fix" note |
| 17 | `gt doctor cleanup \| cat` | Non-TTY category run: text prefixes, no ANSI, clean lines |
| 18 | `gt doctor orphan-sessions --dry-run` (no --fix) | Runs normally, --dry-run silently ignored |

---

## Non-Goals (v1)

- Exclusion patterns (`--except`)
- Saved check profiles
- Result caching
- Re-run failed checks from previous run
- Scheduled/cron execution
- JSON/structured output
- Drill-down mode (more detail than full run)
- Wildcard/glob matching (`orphan-*`)
- Check composition (combining checks into new checks)
- Multi-rig doctor runs
- Interactive TUI selection

---

## Future Considerations (v2+)

- **Wildcard support:** `gt doctor 'orphan-*'` for pattern matching
- **JSON output:** `gt doctor --json orphan-sessions` for programmatic consumption
- **Exclusion:** `gt doctor --except orphan-sessions` to skip specific checks
- **Profiles:** Named check combinations for repeated use
- **Drill-down:** More detail for single-check than full run shows

---

## Spec Review (Phase 3)

**Reviewed:** 2026-02-14
**Method:** Completeness check (45 P0+P1 questions) + fresh assessment (6 categories)
**Result:** 45/45 questions addressed. Added 15 acceptance criteria.

## Multi-Model Review (Phase 4)

**Reviewed:** 2026-02-14
**Models:** Opus 4.6, Sonnet 4.5, Haiku 4.5 (Codex/o4-mini failed, Haiku substituted)
**Issues Found:** 16 (2 critical, 4 high, 5 medium, 5 low)
**Issues Addressed:** 16 of 16

### Findings Addressed

| # | Issue | Resolution |
|---|-------|------------|
| 1 | Check count 47 → actual ~72/83 | Updated all references |
| 2 | Rig check error impossible | Changed to "unknown check" + --rig hint |
| 3 | Category() not on Check interface | Will add Category() to Check interface |
| 4 | Non-TTY detection underspecified | Specified mechanism: ui.IsTerminal() + isTTY parameter |
| 5 | `list` subcommand vs arg confusion | Clarified: Cobra subcommand, removed from arg resolution |
| 6 | cobra.EnablePrefixMatching risk | Documented in CLI Grammar section |
| 7 | Dry-run contradicts "no core changes" | Specified: implemented in cmd/doctor.go via RunStreaming |
| 8 | Non-fixable + --fix output missing | Added output format example |
| 9 | Scope estimate too low | Revised to ~500 new + ~200 modified + ~400 tests |
| 10 | Multiple unmatched error format | Added example for multiple unknown checks |
| 11 | List --rig interaction | Specified: separate section without --rig, inline with |
| 12 | Levenshtein threshold | Moved to spec: ≤2 distance, up to 3 suggestions |
| 13 | Shell completion file | Changed to ValidArgsFunction in doctor.go |
| 14 | One-time hint | Deferred to implementation (low stakes) |
| 15 | Non-TTY multi-check example | Added category run piped example |
| 16 | Dry-run exit code | Kept 0 (no action = no error), documented rationale |

### Ambiguities Resolved

| Topic | Decision | Rationale |
|-------|----------|-----------|
| Rig check errors | "Unknown check" + --rig hint | Simplest, no architecture change needed |
| Category access | Add Category() to Check interface | Clean API, all checks already implement via BaseCheck |

### Implementation Details (developer discretion)

| Detail | Guidance |
|--------|----------|
| One-time hint suppression | Config file flag or simple counter — low stakes |
| `--dry-run` without `--fix` | Silent ignore (spec says "only meaningful with `--fix`") |
