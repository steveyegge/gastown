# doctor-selectivity — Design Specification

**Created:** 2026-02-14
**Status:** Validated
**Brainstorming Mode:** With scope questions (P0+P1, 45 questions)

---

## Overview

Add support for selective doctor execution: `gt doctor <check-name>` runs only the
specified check(s) instead of the full 47-check suite. This addresses three validated
pain points: speed (full runs are slow), noise (users care about one check but see 47),
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
| 24 | Rig check without `--rig`? | Clear error with correct invocation hint | Auto: Rig checks not registered without flag. |
| 25 | Verbose default for targeted? | No, keep `--verbose` opt-in | Human choice — targeting = focus, not depth |
| 26 | Tab completion? | Yes, in v1 | Human choice — Cobra has built-in support |
| 27 | Help text update? | Prominent — update Usage line + examples | Auto: Standard practice. |
| 28 | Hint in full output? | Yes, subtle one-time tip | Auto: Drives adoption. |
| 29 | `gt doctor all`? | Error + redirect to `gt doctor` with no args | Auto |
| 30 | Reserved words? | Reserve `list` as subcommand | Auto: No check name conflicts. |
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
The existing `Doctor` framework is unchanged — filtering happens in `cmd/doctor.go` before
calling `RunStreaming` or `FixStreaming`.

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

**Argument resolution order:**
1. If arg is `list` → run list subcommand
2. If arg matches an exact check name → select that check
3. If arg matches a category name (case-insensitive) → select all checks in category
4. Otherwise → error with "did you mean?" suggestions

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

// SuggestCheck returns the closest check name for typo correction
func SuggestCheck(checks []Check, input string) string

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

#### 3. Modified Doctor Command (`internal/cmd/doctor.go`)

Changes to `runDoctor`:
- Parse positional args as check names/categories
- Register all checks as before, then filter to selected set
- For single-check: simplified output (header + result line)
- For multi-check/category: same streaming format, adapted scope
- `--dry-run` flag: shows what fix would do without applying

#### 4. Shell Completion (`internal/cmd/completion.go`)

Register completion function for doctor command:
- Returns all check names + category names for tab completion
- Cobra's built-in `ValidArgsFunction` mechanism

#### 5. Dry-Run Mode

New flag `--dry-run` (only meaningful with `--fix`):
- Runs the check's `Run()` to identify issues
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

#### Unknown Check Name
```
Error: unknown check "orphen-sessions"

  Did you mean: orphan-sessions?

  Run "gt doctor list" to see all available checks.
```

#### Non-TTY Output (piped)
All ANSI codes stripped. No carriage return overwrites. Clean line-by-line output:
```
Running: orphan-sessions
PASS  orphan-sessions  no orphaned sessions found
```

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Unknown check name | Error + Levenshtein suggestion + "gt doctor list" |
| Rig check without `--rig` | Error: "check 'X' requires --rig flag" with correct command |
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

When `os.Stdout` is not a TTY (detected via `isatty()`):
- No ANSI color codes
- No carriage return streaming (no `\r` overwrites)
- Status as text prefix: `PASS`, `WARN`, `FAIL`, `FIXED`
- No icons (no ✓, ⚠, ✗, 🔧, ⏳)

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
| `internal/doctor/doctor.go` | No changes to core — filtering is external |
| `internal/doctor/types.go` | No changes |

### Estimated Scope
- ~300 lines new code (filter.go + doctor_list.go)
- ~100 lines modified (cmd/doctor.go)
- ~200 lines tests
- Shell completion: ~30 lines

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
