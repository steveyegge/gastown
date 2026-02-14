# Spec Review: doctor-selectivity

## Review Configuration

- **Spec:** plans/doctor-selectivity/spec.md
- **Models Used:** Opus 4.6 (succeeded), Sonnet 4.5 (succeeded), Haiku 4.5 (succeeded)
- **Models Failed:** Codex/o4-mini (unsupported with ChatGPT account) — substituted Haiku
- **Categories:** All (Codebase Match, Design Quality, Security, TDD, Architecture, etc.)
- **Context Source:** Existing context.md + direct codebase reads by all reviewers

## Model Comparison

| # | Issue | Opus 4.6 | Sonnet 4.5 | Haiku 4.5 | Agree? |
|---|-------|----------|------------|-----------|--------|
| 1 | Check count wrong (47 vs 78) | CRITICAL: 5 from WorkspaceChecks + 65 Register calls = 70, +11 rig = 81 | LOW: Count appears to be 72, not 47 | CRITICAL: 67 non-rig + 11 rig = 78 | Yes (all found it) |
| 2 | Category() not on Check interface | HIGH: Unexported categoryGetter requires type assertion in filter | HIGH: Filter needs knowledge of category constants | - | Yes (Opus+Sonnet) |
| 3 | Rig check error impossible with current architecture | MEDIUM: Filter needs separate knowledge of rig check names | CRITICAL: Rig checks not registered without --rig, error can't occur | - | Yes (Opus+Sonnet) |
| 4 | Non-TTY detection underspecified | MEDIUM: Current doctor has no TTY awareness | HIGH: Doesn't specify how TTY state passes to Doctor | HIGH: Mechanism unclear | Yes (all found it) |
| 5 | `list` subcommand vs positional arg confusion | HIGH: Spec inconsistent — line 131 implies arg-based, line 169 implies Cobra subcommand | - | - | Opus only |
| 6 | cobra.EnablePrefixMatching risk | HIGH: `gt doctor l` prefix-matches to `list` subcommand | - | - | Opus only |
| 7 | Dry-run contradicts "no core changes" | MEDIUM: FixStreaming needs modification for dry-run | - | - | Opus only |
| 8 | Dry-run exit code semantics | - | HIGH: Should exit 1 to enable scripting | - | Sonnet only |
| 9 | Scope estimate too low | MEDIUM: ~500-600 lines more realistic | LOW: Check count affects scope | - | Partial (Opus+Sonnet) |
| 10 | Non-fixable check with --fix output missing | - | - | HIGH: No output format example for this case | Haiku only |
| 11 | Levenshtein threshold unspecified | LOW: Left to developer discretion | - | MEDIUM: Critical for consistency | Partial (Opus+Haiku) |
| 12 | One-time hint underspecified | LOW: No detail on text/placement | LOW: Help text example missing | - | Partial (Opus+Sonnet) |
| 13 | Shell completion file location | LOW: Should be in doctor.go not completion.go | HIGH: Implementation location unclear | LOW: Lacks detail | Yes (all mentioned) |
| 14 | Multiple unmatched error format | - | MEDIUM: What if 3 names are wrong? | - | Sonnet only |
| 15 | List --rig interaction | - | MEDIUM: What does list show with/without --rig? | - | Sonnet only |
| 16 | Check within-check state pattern | MEDIUM: Run/Fix state not documented as assumption | - | - | Opus only |
| 17 | Non-TTY multi-check example missing | LOW: Only single-check example shown | - | - | Opus only |

## All Issues by Severity

### CRITICAL (2 issues)

**1. Check count is wrong (47 vs actual 78)**
- **What:** The spec states "full 47-check suite" in the overview (line 12) and acceptance criteria (line 341). The actual codebase registers 67 non-rig checks + 11 rig-conditional checks = 78 total.
- **Where:** Spec lines 12, 341. Code: `internal/cmd/doctor.go` lines 128-222.
- **Evidence:** All three reviewers independently counted the checks. Opus: 70+11=81. Sonnet: ~72. Haiku: 67+11=78. The number 47 is outdated.
- **Recommendation:** Replace "47" with actual count throughout spec. Use "78 checks" or "all registered checks" to be future-proof.

**2. Rig check error message impossible with current architecture**
- **What:** The spec says running a rig check without `--rig` produces: "check 'X' requires --rig flag." But rig checks are only registered when `doctorRig != ""` (line 220 of cmd/doctor.go). Without `--rig`, the check isn't in the registry, so the filter will produce "unknown check" instead.
- **Where:** Error handling table line 276, AC #13 line 346.
- **Evidence:** `cmd/doctor.go:220`: `if doctorRig != "" { d.RegisterAll(doctor.RigChecks()...) }`. Checks are never visible to the filter without the flag.
- **Recommendation:** Either: (a) always register rig checks but skip them during Run, (b) maintain a separate "known rig check names" list for error messages, or (c) change AC #13 to expect "unknown check" + suggestion to use `--rig`.
- **Ambiguity:** Should rig checks be always-registered (cleaner error messages) or conditionally-registered (current pattern)?

### HIGH (4 issues)

**3. Category() not on Check interface — requires type assertion**
- **What:** The spec implies `Category()` is easily accessible on all checks. However, `Category()` is NOT part of the `Check` interface (types.go:93-109). It's accessed via an unexported `categoryGetter` interface (doctor.go:39-41) using type assertion.
- **Where:** Spec "Components" section, filter.go design.
- **Evidence:** `Check` interface: `Name()`, `Description()`, `Run()`, `Fix()`, `CanFix()`. `categoryGetter` at doctor.go:39 is unexported.
- **Recommendation:** Either: (a) add `Category()` to the `Check` interface, (b) export `categoryGetter`, or (c) document the type assertion pattern in the spec. Option (a) is cleanest.
- **Ambiguity:** Should `Category()` be added to the `Check` interface (breaking change but proper) or use type assertion pattern?

**4. Non-TTY detection underspecified in implementation plan**
- **What:** The spec describes non-TTY behavior but doesn't specify how TTY state is passed from cmd/doctor.go to RunStreaming/FixStreaming, or how existing streaming output (with `\r` carriage returns) is conditionally disabled.
- **Where:** Lines 264-301, Modified Files section.
- **Evidence:** Codebase already has `ui.IsTerminal()` in `internal/ui/terminal.go:96`. Current doctor command has no TTY awareness. RunStreaming uses `\r` overwrites that break in pipes.
- **Recommendation:** Add to spec: "TTY detection via existing `ui.IsTerminal()`. Pass `isTTY bool` parameter to `RunStreaming`/`FixStreaming`, or add to the existing `io.Writer` wrapper. When non-TTY: no `\r`, no ANSI, text status prefixes." This means `doctor.go` WILL need modification (update Modified Files section).

**5. `list` subcommand vs positional arg — inconsistent spec**
- **What:** The argument resolution order (line 131) says "If arg is `list` -> run list subcommand" as if handled within `runDoctor`. But Component 2 (line 169) puts `list` in a separate file `doctor_list.go` as a Cobra subcommand. These are different implementation approaches.
- **Where:** Lines 131-135 (argument resolution), lines 169-194 (list component).
- **Evidence:** Cobra subcommands route automatically before reaching `RunE`. If `list` is a subcommand, it shouldn't appear in the argument resolution order.
- **Recommendation:** Clarify: `list` should be a Cobra subcommand in `doctor_list.go`. Remove `list` from the argument resolution order in section "CLI Grammar" — Cobra handles it automatically.

**6. cobra.EnablePrefixMatching creates risk**
- **What:** `root.go:257` enables `cobra.EnablePrefixMatching = true`. This means `gt doctor l` would prefix-match to the `list` subcommand. If a check name starts with `l`, there's ambiguity.
- **Where:** Spec doesn't mention this interaction.
- **Evidence:** `cobra.EnablePrefixMatching = true` in root.go.
- **Recommendation:** Document this in the spec. No check currently starts with `l` (verified), but add a note that check names starting with `list` prefix should be avoided, or disable prefix matching for the doctor subcommand.

### MEDIUM (5 issues)

**7. Dry-run mode contradicts "no changes to core"**
- **What:** Spec says `internal/doctor/doctor.go` needs "No changes to core" but dry-run needs to prevent `Fix()` from being called in `FixStreaming`.
- **Where:** Dry-run section (line 212), Modified Files (line 317).
- **Evidence:** `FixStreaming` at doctor.go:136-153 unconditionally calls `Fix()` when conditions are met.
- **Recommendation:** Implement dry-run in cmd/doctor.go by calling `RunStreaming` (not `FixStreaming`) and rendering custom "Would fix:" output. This preserves "no core changes" claim. Spell this out in spec.

**8. Non-fixable check with --fix — no output example**
- **What:** Q19 says the check runs and notes "does not support auto-fix" but no output format example shows what this looks like.
- **Where:** Q19 (line 60), Output Formats section (lines 217-270).
- **Recommendation:** Add output example showing this case.

**9. Scope estimate too low**
- **What:** ~300 new lines + ~100 modified is optimistic given: non-TTY is new, dry-run, Levenshtein, list formatting, tab completion.
- **Recommendation:** Revise to ~500 new + ~200 modified + ~400 tests.

**10. Multiple unmatched names — error format unclear**
- **What:** Error example (line 256) shows singular "unknown check" but what if `gt doctor foo bar baz` is run?
- **Recommendation:** Add example: "Error: unknown checks 'foo', 'bar', 'baz'. Run 'gt doctor list'."

**11. List subcommand --rig interaction unspecified**
- **What:** Does `gt doctor list` show rig checks? Does `gt doctor list --rig <name>` show them?
- **Where:** Component 2 (line 169). The example (line 188) shows "Rig (requires --rig)" section.
- **Recommendation:** Specify: "Without --rig, rig checks shown in separate section with note. With --rig, shown normally."

### LOW (5 issues)

**12. Levenshtein threshold should be in spec, not developer discretion**
- Recommendation: Move to spec: "max edit distance 2, show up to 3 suggestions."

**13. Shell completion in doctor.go, not separate file**
- Recommendation: Set `ValidArgsFunction` on doctorCmd in cmd/doctor.go. No separate completion.go needed.

**14. One-time hint underspecified**
- Recommendation: Either specify or defer to v2 (YAGNI).

**15. Non-TTY multi-check output example missing**
- Recommendation: Add example showing category run in non-TTY mode.

**16. Dry-run exit code semantics**
- Spec says 0 for dry-run with issues. Sonnet argued for non-zero. Current spec choice is reasonable (no action = no error), but document the rationale.

## Reasoning

### Check count (Issue 1)
All 3 models flagged this independently. Minor variation in counts (Opus 81, Haiku 78, Sonnet 72) due to counting methodology — some counted `WorkspaceChecks()` as 1 call vs 5 checks. The exact count is 67 individual `d.Register()` calls + `RegisterAll(WorkspaceChecks()...)` which adds 5 = 72 base checks. With `--rig`, 11 more from `RigChecks()` = 83 total. Either way, 47 is significantly wrong.

### Rig check error (Issue 2)
Sonnet classified this as CRITICAL, Opus as MEDIUM. I agree with CRITICAL — AC #13 describes an impossible scenario that would fail immediately in testing. The simplest fix is option (c): change the error message to "unknown check" with a hint about `--rig`, rather than restructuring check registration.

### `list` routing (Issue 5)
Only Opus caught this inconsistency. It's valid — the spec describes `list` in two conflicting ways. The Cobra subcommand approach (separate file) is clearly the right one, and the argument resolution order should drop `list` from step 1.

### Prefix matching (Issue 6)
Only Opus caught this. It's a real risk but low probability since no check starts with `l`. Worth documenting but not blocking.

## Ambiguities Summary

| # | Issue | Ambiguity | Options |
|---|-------|-----------|---------|
| 1 | Rig check error (Issue 2) | How to produce correct error? | (a) Always register rig checks (b) Separate known-names list (c) Show "unknown check" + --rig hint |
| 2 | Category access (Issue 3) | How does filter access categories? | (a) Add Category() to Check interface (b) Export categoryGetter (c) Type assertion in filter |

## Summary

- **Total Issues:** 16 (2 critical, 4 high, 5 medium, 5 low)
- **Ambiguities Requiring Decision:** 2
- **Model Agreement Rate:** 53% (9 of 17 raw issues flagged by 2+ models)
- **Models That Failed:** Codex/o4-mini (substituted Haiku 4.5)
- **Strongest Review:** Opus (most unique valid findings, caught architectural issues others missed)
- **Most Thorough:** Sonnet (23 issues, most detail, but some inflation)
