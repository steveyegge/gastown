# Question Triage: doctor-selectivity

**Scope selected:** P0 + P1
**Questions in scope:** 45
**Auto-answerable:** 22
**Branch points for human:** 23

---

## Auto-Answerable Questions

These questions have clear answers based on codebase evidence, best practices,
or logical inference. They are noted in the spec without asking you.

| # | Question (summary) | Proposed Answer | Source |
|---|----------|-----------------|--------|
| 1 | Does `gt doctor <name>` also fix, or is `--fix` still required? | `--fix` still required. Selective targeting controls *which* checks run, not *how* they run. | Codebase: `FixStreaming` vs `RunStreaming` are separate paths (doctor.go:44-46, cmd/doctor.go:237-241). Separating diagnosis from repair is the existing safety model. All 3 models converged. |
| 3 | Does `gt doctor` with no args continue as before? | Yes, 100% backward compatible. | Cobra positional args are optional by design. Zero args = existing behavior unchanged. |
| 10 | Do checks depend on other checks? Can isolation produce wrong results? | No. Checks are independent — each receives `CheckContext` but no shared state. | Codebase: `RunStreaming` (doctor.go:54-61) passes same `ctx` to each check independently. No check reads results from prior checks. Verified via code review. |
| 17 | Consistent exit codes for selective runs? | Yes. Non-zero on errors, same as full runs. | Codebase: `report.HasErrors()` (cmd/doctor.go:247-249) already drives exit codes. Same report mechanism applies regardless of check count. |
| 19 | What happens with `--fix` on a non-fixable check? | Check runs normally, shows result. `--fix` is silently inapplicable — `CanFix()` returns false so fix is never attempted. Output should note "this check does not support auto-fix". | Codebase: `FixStreaming` (doctor.go:136) guards with `check.CanFix()`. Non-fixable checks just run and report. |
| 20 | Auto re-run after fix succeeds? | Yes, already implemented. After fix, check re-runs to verify. | Codebase: `FixStreaming` (doctor.go:154-168) — calls `check.Run(ctx)` again after `check.Fix(ctx)` succeeds. |
| 22 | Immediate feedback before results? | Yes — streaming output already shows "○ check-name..." while running, then overwrites with result. Same pattern applies to selective. | Codebase: `RunStreaming` (doctor.go:57) prints check name before `check.Run(ctx)`. |
| 23 | All existing flags work with selective? | Yes. `--verbose`, `--slow`, `--rig`, `--fix`, `--restart-sessions` all compose naturally. No technical barrier. | Codebase: Flags are processed independently in `runDoctor` (cmd/doctor.go:99-106). Check selection is orthogonal to flag handling. |
| 29 | What if user runs `gt doctor all`? | Error: unknown check "all". Suggest: `gt doctor` with no arguments runs all checks. Include "did you mean?" if close to a real check name. | `all` is not a check name and should not be reserved. Natural-language intent is clear — error message redirects. |
| 30 | Check name vs reserved word conflicts (`list`, `help`)? | Reserve `list` as a subcommand from day 1. No existing check is named `list`, `help`, or `version`. Future check names should avoid these. | Codebase: All current check names are domain-specific (orphan-sessions, wisp-gc, etc). No collision risk with common subcommands. Proactive reservation prevents future conflicts. |
| 33 | Running outside a Gas Town workspace? | Already handled. Clear error: "not in a Gas Town workspace". | Codebase: `workspace.FindFromCwdOrError()` (cmd/doctor.go:111-113) handles this before any checks run. Same path for selective. |
| 39 | Stale/renamed check names? | Error message for unknown check should list available checks. If Levenshtein distance is small, suggest "did you mean X?". No alias system needed for v1. | Industry standard (git, kubectl, npm all use "did you mean?" for close matches). |
| 41 | Passing check output for targeted run? | Show the check result line (✓ check-name) plus the check's description. Brief but informative. No summary section for single-check pass. | Consistent with existing streaming output but enhanced for targeted context. Single "✓ orphan-sessions — no orphaned sessions found" is sufficient and satisfying. |
| 43 | Flag ordering: `gt doctor orphaned-sessions --fix` vs `gt doctor --fix orphaned-sessions`? | Both work identically. | Cobra framework: positional args and flags are order-independent by design. No custom parsing needed. Verified by Cobra's argument handling. |
| 24 | Rig check without `--rig` flag? | Clear error: "check 'beads-config-valid' requires --rig flag. Run: gt doctor --rig <rig-name> beads-config-valid". | Codebase: Rig checks only registered when `doctorRig != ""` (cmd/doctor.go:220-222). In selective mode, if check not found in registry AND it matches a known rig check name, produce targeted error. |
| 21 | Partial fix success? | Report what succeeded and what remains. This is already the behavior — fixes are per-item and results are accumulated. | Codebase: Each check's `Fix()` handles its own items. `Run()` after fix shows remaining issues. The re-run verification (doctor.go:156) catches partial success naturally. |
| 27 | Update help text for selective syntax? | Prominent, not footnote. Add `Usage: gt doctor [check-name] [flags]` and brief example to Long description. | Industry standard — new syntax should be visible in help. Users read `--help` first. |
| 28 | Hint in full doctor output? | Yes, subtle one-time hint at end of first run: "Tip: run gt doctor <check-name> to target a specific check. See gt doctor list for available checks." | Drives adoption. Can be suppressed after first viewing or in non-TTY mode. Low visual cost. |
| 31 | Ctrl+C during targeted fix? | Fixes should be atomic per-item (kill one session, clean one wisp). Interruption leaves remaining items unfixed but system in consistent state. Same as current behavior. | Codebase: Each fix operation is per-item in existing checks (e.g., kill each orphan session individually). Partial completion is safe — re-running fixes remaining. |
| 32 | Concurrent doctor runs? | No locking needed for v1. Checks are inherently idempotent (killing already-dead sessions is a no-op, GC-ing already-collected wisps is a no-op). | Codebase: Existing checks are idempotent by design. Two concurrent orphan-sessions fixes would just have the second find nothing to fix. |
| 38 | List shows fixable status? | Yes. Show 🔧 icon or "(fixable)" label next to fixable checks in listing. Essential for informed choice. | Codebase: `CanFix()` is available on every registered check (types.go:108). Easy to surface. Universal UX expectation. |
| 40 | See check description before running? | Yes. `gt doctor list` shows name + description. `Description()` already exists on every check. | Codebase: `Check` interface includes `Description() string` (types.go:98). Already implemented on every check. Just needs to be surfaced in listing. |

---

## Branch Points (Human Decision Required)

These questions have no clear "right" answer — they depend on your preferences,
target users, or design philosophy.

### Round 1: Core Architecture (~6 questions)
These unlock the most downstream decisions. Answer these first.

| # | Question | Why Human Needed |
|---|----------|------------------|
| 2 | **Scoped execution vs filtered output?** Does `gt doctor <name>` run ONLY that check (fast, isolated), or run all checks but only display that one (slower, more context)? | Architecture decision with performance and safety tradeoffs. Scoped = fast but isolated. Filtered = slow but safe. Recommendation: scoped execution (run only targeted check). |
| 7 | **Positional argument or flag?** `gt doctor orphaned-sessions` (positional) vs `gt doctor --check orphaned-sessions` (flag)? | CLI grammar decision. Positional is more natural and concise. Flag is more explicit and avoids namespace collisions with subcommands like `list`. Recommendation: positional arg, with `list` as a reserved subcommand. |
| 8 | **Multiple check names?** Can users pass `gt doctor orphaned-sessions wisp-gc`? | Scope decision. Supporting multi-select is natural but complicates parsing and output. Recommendation: yes, support multiple positional args. |
| 9 | **Category filtering?** Should `gt doctor cleanup` run all Cleanup-category checks? | Scope decision. Category filtering is a middle ground between single check and full suite. Adds UX surface but provides useful batching. Recommendation: yes for v1, categories are already defined in code. |
| 34 | **Is the real problem speed?** If full doctor took 2 seconds, would anyone want selectivity? | Strategic question. If speed is the primary driver, other solutions (parallelization, caching) might be better. If focus/noise-reduction is the driver, selectivity is correct. |
| 42 | **Should cleanup ops be separate from doctor?** Is doctor doing two jobs (diagnostics + maintenance)? | Architectural question. If cleanup should be `gt fix <operation>` instead, the entire feature changes. Recommendation: keep in doctor, but this is a philosophical choice. |

### Round 2: UX and Safety (~10 questions)

| # | Question | Why Human Needed |
|---|----------|------------------|
| 4 | **Primary discovery mechanism?** How do users find check names? `gt doctor list` subcommand? `gt doctor --help`? Both? | Design decision on primary entry point. |
| 5 | **What does `gt doctor list` show?** Name only? Name + description? Name + description + category + fixable status? Table or grouped list? | Information density tradeoff. More info = more useful but harder to scan. |
| 6 | **"Did you mean?" suggestions?** Exact text and behavior for unrecognized check name. Just error? Suggestions? Full list? | Error UX design. Recommendation: error + "did you mean?" suggestion + "run gt doctor list for all checks". |
| 11 | **Confirmation prompt before fix?** Should `gt doctor orphaned-sessions --fix` prompt "Fix 5 orphaned sessions? [y/N]" or just do it? | Safety vs friction tradeoff. Prompt increases safety but breaks scriptability. `--fix` is arguably explicit consent already. |
| 12 | **Single-check output format?** Same streaming format as full run (with headers, separators, summary)? Or simplified single-line result? | Design decision. Full format feels heavy for one check. Simplified breaks consistency. |
| 13 | **Summary line for single check?** Show "1 passed, 0 warnings, 0 failed"? Or just the check result? | Related to Q12. Summary of 1 feels silly but may matter for script parsing. |
| 14 | **Performance target?** Should single-check be sub-second? Just "noticeably faster"? Is 50% of full-run time acceptable? | Product decision. Sets engineering constraints. Scoped execution makes sub-second achievable. |
| 15 | **Fuzzy matching?** `gt doctor orphan` matches `orphan-sessions`? Or exact match only with suggestions for near-misses? | UX tradeoff. Fuzzy = convenient but dangerous (could match wrong check). Exact + suggestions = safer. |
| 37 | **Visual scope signaling?** Should single-check output look visually different from full output? Header showing "Running: orphan-sessions (1 check)"? | Design decision on scope communication. |
| 45 | **Dry-run mode?** Add `--dry-run` for selective fixes? Shows "Would fix: 5 orphaned sessions" without acting? | Scope decision. Nice for safety but adds complexity. Could defer to v2. |

### Round 3: Scope Boundaries (~7 questions)

| # | Question | Why Human Needed |
|---|----------|------------------|
| 16 | **Input normalization?** Accept `orphan_sessions` (underscores), `OrphanSessions` (camelCase), or only exact `orphan-sessions`? | Strictness vs convenience. Case-insensitive + hyphen/underscore equivalence is common. |
| 18 | **Non-TTY output?** Should piped output strip ANSI codes and streaming updates automatically? | Technical scope. Good practice but adds implementation effort. Could detect `isatty()`. |
| 25 | **Verbose default for targeted?** Should single-check auto-expand verbose details since targeting implies wanting depth? | UX philosophy. Auto-verbose changes defaults. Could feel helpful or surprising. |
| 26 | **Tab completion?** Ship tab-completion for check names in v1? | Effort/priority decision. High UX value but requires shell integration (bash, zsh, fish completions). |
| 35 | **Is the real problem noise?** Would users prefer better output formatting over selective execution? | Strategic. Could inform whether to prioritize output improvements alongside or instead of selectivity. |
| 36 | **Is the real problem users want `gt fix`?** Would a separate fix command be more direct? | Strategic. May reveal that doctor framing is wrong abstraction. |
| 44 | **Wildcard/glob support?** Allow `gt doctor 'orphan-*'` or `gt doctor 'stale-*'`? | Scope decision. Power feature but adds parsing complexity. Could defer. |

---

## Question Dependencies

Answering early questions automatically resolves later ones:

**Q2 (Scoped vs Filtered execution)** unlocks:
- Q14 (Performance) — if scoped, sub-second is achievable since only one check runs
- Q10 is already auto-answered (no check deps), reinforcing that scoped is safe

**Q7 (Positional vs Flag)** unlocks:
- Q8 (Multiple names) — positional args naturally support multiple values
- Q9 (Category filtering) — positional namespace must handle both check names and category names
- Q30 is already auto-answered (reserve `list`)

**Q9 (Category filtering)** unlocks:
- Q37 (Visual signaling) — needs to distinguish single-check, category, and full runs

**Q11 (Confirmation prompt)** unlocks:
- Q45 (Dry-run) — if no confirmation, dry-run becomes more important as safety valve

**Q12 (Output format)** unlocks:
- Q13 (Summary line) — follows naturally from output format decision

**Q34 + Q35 + Q36 (Problem depth)** unlock:
- Q42 (Architecture) — if real problem is speed, architecture question is moot
- Overall feature validation — if answers suggest wrong solution, scope may change

---

## Interview Plan

**Round 1: Core Architecture** (~6 questions)
These determine the feature's fundamental shape:
1. Q34 — Is the real problem speed? (validates feature)
2. Q35 — Is the real problem noise? (validates approach)
3. Q36 — Would `gt fix` be more direct? (validates abstraction)
4. Q2 — Scoped execution vs filtered output?
5. Q7 — Positional arg or flag?
6. Q9 — Category filtering in scope?

**Round 2: Cascaded Confirmations** (~3 questions)
Based on Round 1, confirm inferred answers:
- "Since you chose scoped execution, sub-second is the target. Correct?" → Q14
- "Since positional args, multiple names supported?" → Q8
- "Cleanup stays in doctor?" → Q42

**Round 3: UX and Safety** (~8 questions)
Remaining design decisions:
1. Q4/Q5 — Discovery mechanism (list command details)
2. Q6 — Error message style
3. Q11 — Confirmation prompt
4. Q12/Q13 — Output format
5. Q15/Q16 — Matching strictness
6. Q25 — Verbose default
7. Q37 — Visual scope signaling
8. Q45 — Dry-run mode

**Round 4: Scope Boundaries** (~3 questions)
Lower priority scope decisions:
1. Q18 — Non-TTY detection
2. Q26 — Tab completion for v1?
3. Q44 — Wildcard support?

---

**Estimated dialogue:** ~23 questions for you, ~22 auto-noted
**With dependency cascading:** likely ~15-18 actual questions (some cascade from earlier answers)
