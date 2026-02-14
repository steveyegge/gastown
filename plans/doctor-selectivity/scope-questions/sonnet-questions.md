# Sonnet 4.5 Analysis: doctor-selectivity

## User Advocate Perspective

### Feature Brief
Add support for `gt doctor <specific-doctor-fix>` like `gt doctor orphaned-sessions`

---

### User Expectations

#### What would users assume this feature does?

1. **Does it run ONLY that specific check or does it run all checks but only fix that one?**
   - This matters because users might be confused if they see output from 46 other checks when they asked for just one.

2. **Will `gt doctor orphaned-sessions` show me the problem first or immediately fix it?**
   - This matters because users might panic if they type a command and it instantly deletes their sessions without confirmation.

3. **Can I run multiple specific checks at once like `gt doctor orphaned-sessions wisp-gc`?**
   - This matters because users often want to fix a small subset of issues without running the full diagnostic suite.

4. **Does the check name need to match exactly or can I use partial matches?**
   - This matters because users won't memorize exact check names and will try shortcuts like "orphan" instead of "orphaned-sessions".

5. **Will this work the same way as `git <command>` or `docker <command>` where the argument is a subcommand?**
   - This matters because users bring mental models from other CLI tools and expect consistency.

6. **If I type `gt doctor cleanup` will it run all cleanup checks or look for a check literally named 'cleanup'?**
   - This matters because the codebase has categories (Cleanup, Core, etc.) and users might expect category-level selection.

7. **Will `gt doctor orphaned-sessions --fix` work or do I need `gt doctor --fix orphaned-sessions`?**
   - This matters because flag ordering varies across CLI tools and users will try both.

8. **When I run a specific check, will it still show me the summary footer or just the single check result?**
   - This matters because users might rely on that summary format for scripting or mental pattern matching.

9. **Does this replace or supplement the existing `gt doctor` behavior?**
   - This matters because users might worry they're losing the comprehensive health check they're used to.

10. **Will there be a way to discover what check names are available without reading documentation?**
    - This matters because users won't open docs for every command; they expect discoverability through the CLI itself.

11. **If I misspell a check name, will the tool suggest the correct one or just error?**
    - This matters because typos are inevitable and helpful error messages prevent frustration.

12. **Will `gt doctor orphaned-sessions` behave identically to `gt doctor --fix` when it hits that specific check?**
    - This matters because inconsistent behavior between full and selective runs will confuse users.

13. **Can I use this with other flags like `--verbose` or `--rig`?**
    - This matters because users will want detailed output or rig-specific checks for their targeted runs.

14. **Will the output indicate "running only orphaned-sessions check" or will it be silent about other checks being skipped?**
    - This matters because users need confidence that the tool understood their intent.

---

### User Journey

#### What's the user trying to accomplish? What's their emotional state?

15. **When a user knows they have orphaned sessions, why would they run this instead of just killing tmux sessions manually?**
    - This matters because if the tool doesn't provide clear value over manual fixes, users won't adopt it.

16. **Is the user likely to run this during a crisis (daemon crashed, can't start work) or during routine maintenance?**
    - This matters because crisis-mode users need speed and confidence, while maintenance-mode users want thoroughness.

17. **Will users remember the exact check name when their system is broken and they're stressed?**
    - This matters because cognitive load is high during troubleshooting and every friction point compounds frustration.

18. **After running a specific fix, what should the user do next?**
    - This matters because the tool should guide users toward success, not leave them wondering if the problem is solved.

19. **If a user runs `gt doctor orphaned-sessions` and it says "OK", do they trust that or run the full suite?**
    - This matters because selective checking might create doubt about overall system health.

20. **When would a user choose `gt doctor <specific>` over `gt doctor --fix`?**
    - This matters because if the use cases overlap too much, one feature becomes redundant.

21. **Will users expect to run this command repeatedly during a debugging session?**
    - This matters because the UX should optimize for repeated runs (speed, clear state changes, etc.).

22. **If a user sees a warning in `gt doctor` output, will they know how to re-run just that check?**
    - This matters because the full doctor report should make selective re-runs discoverable and easy.

23. **What happens before the user runs this command—how did they identify which specific check they need?**
    - This matters because the workflow should be seamless from problem identification to targeted fixing.

24. **After fixing an issue with this command, does the user need to verify with `gt doctor` again?**
    - This matters because users shouldn't need multiple commands to achieve confidence in a fix.

25. **If a user is following a troubleshooting guide that says "run this specific check", will they understand the syntax immediately?**
    - This matters because documentation and user-to-user support depend on intuitive command structure.

26. **When a user has 5 minutes before a meeting and just needs to fix one thing, will this command respect that urgency?**
    - This matters because rushed users will abandon tools that waste time or create new problems.

---

### Edge Cases (User Behavior)

#### What weird things might users try? What if they use it wrong?

27. **What if a user types `gt doctor orphaned-sessions orphan-processes` and one exists but the other doesn't?**
    - This matters because partial success/failure scenarios need clear handling and feedback.

28. **What if a user types `gt doctor all` expecting to run everything?**
    - This matters because "all" is a natural word users might try as a catch-all argument.

29. **What if a user types `gt doctor fix-orphaned-sessions` using action verbs in the check name?**
    - This matters because users might conflate the check name with the action they want to take.

30. **What happens if a user runs `gt doctor orphaned-sessions` while another `gt doctor` process is already running?**
    - This matters because concurrent runs might conflict or produce confusing output.

31. **What if a user runs the same specific check 10 times in a row rapidly?**
    - This matters because repeated runs might indicate the fix isn't working, or the user is confused about the result.

32. **What if a user tries `gt doctor "orphaned sessions"` with spaces or quotes?**
    - This matters because users coming from other tools might expect quoted arguments.

33. **What if the specific check name conflicts with a future flag name?**
    - This matters because adding `gt doctor --list` later might break someone's script that does `gt doctor list`.

34. **What if a user runs `gt doctor orphaned-sessions` without the `--fix` flag and expects it to fix anyway?**
    - This matters because the command name pattern implies action, but flags control actual fixing.

35. **What if a user chains this in a script like `gt doctor orphaned-sessions && gt checkout main`?**
    - This matters because exit codes and error propagation affect automation reliability.

36. **What if a user hits Ctrl+C halfway through a selective fix?**
    - This matters because interrupted fixes might leave the system in an inconsistent state.

37. **What if a user bookmarks or scripts a check name that gets renamed in a future version?**
    - This matters because breaking changes to check names will break user workflows silently.

38. **What if a user typos the check name but the typo is one character off from a real check?**
    - This matters because auto-running the wrong check could cause unintended consequences.

39. **What if a user expects `gt doctor orphan*` to work with wildcards or glob patterns?**
    - This matters because shell users are accustomed to wildcard expansion for filtering.

40. **What if a user runs `gt doctor orphaned-sessions --rig` and that check doesn't support rig mode?**
    - This matters because incompatible flag combinations need graceful handling.

41. **What if a user runs this command from a directory that isn't a Gas Town workspace?**
    - This matters because error messages should be helpful regardless of working directory.

---

### Accessibility & Inclusion

#### Who might struggle with this? What assumptions are we making?

42. **Will users with screen readers understand the output when only one check runs?**
    - This matters because streaming output and icons might not translate well to non-visual interfaces.

43. **Are we assuming users can remember or look up exact kebab-case check names?**
    - This matters because memory requirements and discoverability affect users with cognitive differences.

44. **Will colorblind users be able to distinguish check results without color?**
    - This matters because relying solely on color for status communication excludes some users.

45. **Are we assuming users are comfortable with terminal commands and flags?**
    - This matters because CLI complexity can exclude users who are less technical or new to development.

46. **Will non-native English speakers understand check names like "orphaned-sessions" or "wisp-gc"?**
    - This matters because jargon and idioms create barriers for international users.

47. **Are we assuming users have working tab completion configured?**
    - This matters because discoverability drops significantly without autocomplete.

48. **Will users on slower terminals or SSH connections get frustrated with streaming output?**
    - This matters because network latency can make real-time updates feel sluggish or broken.

49. **Are we assuming users know what each check does from its name alone?**
    - This matters because cryptic check names force users to context-switch to documentation.

50. **Will users who rely on scripts or automation be able to parse the output programmatically?**
    - This matters because some users can't or won't read visual terminal output directly.

51. **Are we assuming users work in environments where they can safely run fix commands?**
    - This matters because production systems, shared workspaces, or CI environments might have different safety requirements.

52. **Will users with different terminal widths see broken or confusing output?**
    - This matters because narrow terminals (mobile SSH, split panes) can mangle carefully formatted output.

53. **Are we assuming users understand the consequences of fixing specific issues in isolation?**
    - This matters because fixing one thing might surface or hide other problems that the full suite would catch.

---

### Additional User-Focused Concerns

#### Discovery and Learning

54. **When users first learn about this feature, will they understand when to use it vs. the full `gt doctor`?**
    - This matters because confusing feature overlap leads to users picking the wrong tool for their situation.

55. **Will existing `gt doctor` users discover this new capability organically or need explicit training?**
    - This matters because hidden features don't get used, and explicit training creates adoption friction.

56. **If a user sees `gt doctor orphaned-sessions` in a colleague's Slack message, can they understand and reproduce it?**
    - This matters because peer-to-peer knowledge sharing is how most users actually learn tools.

#### Trust and Confidence

57. **Will users trust a selective check as much as the full diagnostic suite?**
    - This matters because perceived reliability affects whether users actually run the fix or seek alternatives.

58. **If a selective check says "OK" but the full suite later finds issues, will users lose confidence?**
    - This matters because contradictory results erode trust in the entire tool.

59. **Will users feel confident running `--fix` on a specific check without seeing the broader system context?**
    - This matters because fear of unintended consequences can paralyze users even when the tool is safe.

#### Workflow Integration

60. **Will users expect this to integrate with CI/CD pipelines or pre-commit hooks?**
    - This matters because automated workflows need stable interfaces and predictable behavior.

61. **Can users easily switch from "check-only" to "fix" mode without retyping the whole command?**
    - This matters because iterative workflows (check, review, fix) should be frictionless.

62. **Will users want to save their frequently-run selective checks as aliases or shortcuts?**
    - This matters because power users optimize their workflows and the tool should support that.

#### Error Recovery

63. **If a specific fix fails partway through, will users know how to recover or retry safely?**
    - This matters because partial failures can leave systems in worse states than before the fix.

64. **Will users understand whether a failed selective fix means "this check failed" or "the fix operation failed"?**
    - This matters because ambiguous error states lead to incorrect troubleshooting.

65. **If users run a specific check that depends on other checks passing first, will they get helpful guidance?**
    - This matters because check dependencies might not be obvious and users might run things out of order.

#### Performance and Feedback

66. **Will users become impatient if a "quick selective check" takes longer than expected?**
    - This matters because expectations set by the feature name affect perceived performance.

67. **Will users understand progress when a selective check is running?**
    - This matters because black-box operations without feedback make users anxious.

68. **If a selective check is instant, will users trust that it actually did something?**
    - This matters because too-fast operations can feel fake or incomplete.

#### Documentation and Help

69. **Will users know where to find the list of available check names without leaving the terminal?**
    - This matters because context-switching to documentation breaks flow and reduces tool usage.

70. **If a user runs `gt doctor --help` will they understand this new selective syntax?**
    - This matters because help text is the primary discovery mechanism for CLI features.

71. **Will example commands in help text cover the most common user scenarios?**
    - This matters because users copy-paste examples more than they read explanations.

#### Compatibility and Migration

72. **Will users' existing scripts with `gt doctor` continue to work unchanged?**
    - This matters because breaking backward compatibility forces migration work on users.

73. **If check names change in the future, will old commands fail gracefully with helpful messages?**
    - This matters because version upgrades shouldn't silently break user workflows.

74. **Will users understand if some checks aren't available for selective runs?**
    - This matters because inconsistent feature availability creates confusion and support burden.

---

## Product Designer Perspective

### Feature Brief
Add support for `gt doctor <specific-doctor-fix>` like `gt doctor orphaned-sessions`

### Information Architecture

#### 1. How should check names be displayed when listing available options?
Users need to quickly scan and identify the check they want to run, so consistent formatting (kebab-case) and categorization is critical for discoverability.

#### 2. What metadata should accompany each check in a list view (category, fixable status, description)?
Users need to understand what a check does and whether it can fix issues before running it, reducing trial-and-error.

#### 3. Should categories be visually separated or just labeled inline when showing available checks?
Grouping reduces cognitive load by chunking related checks together, making it easier to find relevant fixes.

#### 4. How should the system indicate which checks are currently failing vs. all available checks?
Users may want to prioritize running checks that are known to be failing, so highlighting problem areas saves time.

#### 5. Should the output show similar/related checks when a user specifies an invalid check name?
Fuzzy matching or suggestions reduce frustration when users misremember exact check names.

#### 6. What information is essential in the help text for this feature (examples, syntax, options)?
Clear examples showing common use cases help users understand the feature without reading full documentation.

#### 7. Should there be a visual distinction between running a single check vs. a category of checks?
Users need to understand the scope of what they're about to run, especially if it's broader than they intended.

#### 8. How should check aliases or shortcuts be surfaced (e.g., "orphan-sessions" vs. "orphaned-sessions")?
Inconsistent naming causes confusion, so showing accepted variations prevents repeated failed attempts.

#### 9. What should users see when they run `gt doctor` with no arguments vs. `gt doctor list`?
Distinguishing between "run all checks" and "show available checks" prevents accidental execution of all checks.

#### 10. Should recent check results be cached and displayed in list views to show status?
Showing last-run results helps users decide which checks need attention without running everything again.

#### 11. How should the system present the relationship between check names and their categories?
Users may think in terms of categories ("cleanup stuff") rather than specific check names, so showing this mapping aids discovery.

#### 12. Should there be visual indicators for checks that require additional flags (--rig, --restart-sessions)?
Users need to know upfront if a check requires additional context, avoiding incomplete runs.

#### 13. What information hierarchy makes sense when showing a single check's detailed information?
When focusing on one check, users need more depth (description, what it fixes, risks, dependencies) than in a list view.

#### 14. How should the system differentiate between check IDs (machine names) and display names?
Users may search using natural language ("orphaned sessions") but the system expects kebab-case, so showing both prevents confusion.

### Interaction Design

#### 15. What happens when a user types `gt doctor orphan` - exact match only or prefix matching?
Prefix matching reduces typing but may cause ambiguity if multiple checks start with the same word.

#### 16. Should running `gt doctor <check-name>` automatically apply fixes or require the --fix flag?
Auto-fixing without explicit consent could surprise users and cause unintended changes, violating the principle of least surprise.

#### 17. How should the feature respond to multiple check names (e.g., `gt doctor orphan-sessions wisp-gc`)?
Supporting multiple targets in one command increases efficiency for users who know what they need.

#### 18. What should happen if a user specifies both a specific check and a category?
Handling overlapping selections (explicit check + category containing it) needs clear precedence rules to avoid confusion.

#### 19. Should there be a confirmation prompt before running fixable checks, or does --fix imply consent?
Single-check runs may need less confirmation than full runs, but destructive operations still warrant explicit approval.

#### 20. How should the --verbose flag interact with single-check runs?
Users running one check may always want detailed output, making verbose the implicit default for targeted runs.

#### 21. What keyboard shortcuts or aliases would make this feature more efficient for power users?
Frequent users benefit from shortened commands (e.g., `gt d orphan` or `gt doctor -l`), reducing repetitive typing.

#### 22. Should running an invalid check name trigger interactive selection, or just show an error?
Interactive fallback is friendlier but may interrupt scripting use cases, so mode detection (TTY vs. pipe) matters.

#### 23. How should users cancel a long-running single check?
Ctrl+C should cleanly abort without leaving partial state, and users should know if cancellation is safe mid-check.

#### 24. What interaction model works best for running all checks in a category (positional arg, flag, or special syntax)?
Consistency with existing CLI patterns (Cobra conventions) ensures users can transfer knowledge from similar tools.

#### 25. Should there be a dry-run mode that shows what would be fixed without actually fixing?
Users want to preview changes before committing, especially for unfamiliar checks that might affect their workflow.

#### 26. How should the feature handle case sensitivity in check names?
Forcing exact case matching adds friction; case-insensitive matching is more forgiving and user-friendly.

#### 27. What happens if a user runs `gt doctor --fix` with a check that's not fixable?
Clear feedback that the check is informational-only prevents confusion about why --fix had no effect.

#### 28. Should there be a way to "favorite" or "pin" frequently-run checks for quick access?
Power users running the same checks repeatedly benefit from saved shortcuts or history-based suggestions.

### User Flows

#### 29. Happy path: User wants to fix orphaned sessions specifically - what are the exact steps and output?
The core use case defines the entire feature's success; every step needs to feel effortless and transparent.

#### 30. What does the output look like when running a single check vs. the full suite?
Simplified output for single checks (no category headers, focused messaging) improves signal-to-noise ratio.

#### 31. Error path: User typos the check name - how do they recover without leaving the terminal?
Fast recovery from mistakes (suggestions, fuzzy matching, interactive selection) reduces friction dramatically.

#### 32. How does a user discover what checks are available if they don't know the name?
First-time users or those exploring need an intuitive way to browse without reading documentation.

#### 33. What's the flow for running all cleanup checks at once?
Category-based execution is a natural grouping that users will expect based on the existing categorization.

#### 34. Edge case: User runs a check that has dependencies on other checks - what happens?
Dependencies should either auto-run or warn the user, preventing confusing failures due to missing prerequisites.

#### 35. How does a user move from discovering a check to running it to fixing it?
Multi-step workflow (list -> inspect -> run -> fix) should be smooth, with each step suggesting the next action.

#### 36. What's the experience when a specific check passes (nothing to fix)?
Success states need clear messaging; silence might feel like nothing happened, causing users to re-run unnecessarily.

#### 37. What happens when a check fails but can't auto-fix (requires manual intervention)?
Actionable guidance (what to do next, manual steps, related commands) prevents dead-ends.

#### 38. How does a user return to running the full suite after running specific checks?
Switching between targeted and full modes should be obvious and not require memorizing different commands.

#### 39. What's the flow when running multiple specific checks in sequence?
Users should understand whether they're running serially or in parallel, and see progress for each.

#### 40. Edge case: Empty state - what if all checks pass when running a specific one?
Positive reinforcement ("All clear!") feels better than just exiting silently, building user confidence.

#### 41. How does a user know if a check is safe to run in their current context (active sessions, dirty git state)?
Pre-flight warnings about environmental requirements prevent disruptive failures mid-check.

#### 42. What's the recovery path if a fix operation partially succeeds?
Partial success states (fixed 3 of 5 issues) need clear reporting so users can address remainders.

### Visual & Layout

#### 43. Should single-check output use the same icon system (✓, ⚠, ✗, 🔧) as the full suite?
Visual consistency with existing patterns reduces learning curve and maintains brand identity.

#### 44. How much vertical space should list output consume (one line per check, grouped blocks, paginated)?
Terminal real estate is limited; compact views work for lists, but details need breathing room.

#### 45. Should category headers appear when running all checks in a category, or just the check names?
Context headers help users understand grouping, but redundant headers in small sets add clutter.

#### 46. Where should this feature be documented in the help text hierarchy?
Discoverability depends on placement in `gt doctor --help` - should it be a top-line example or under advanced usage?

#### 47. Should the output be color-coded differently for targeted runs vs. full runs?
Visual distinction helps users quickly confirm they're in the right mode, preventing accidental full runs.

#### 48. How should progress be indicated when running a single slow check?
Spinner, percentage, elapsed time, or status messages help users know the system isn't frozen.

#### 49. What visual layout works for showing check details (description, last run, fix capability, warnings)?
Structured formatting (key: value pairs, tables, or indented lists) improves scannability over prose paragraphs.

#### 50. Should the summary line at the end adjust for single-check runs, or maintain the same format?
Saying "1 passed, 0 failed" for a single check feels verbose; a simpler message like "orphan-sessions: passed" is clearer.

#### 51. How should the terminal width be utilized when showing check names and descriptions in list mode?
Long descriptions should wrap intelligently or truncate with ellipsis, adapting to narrow terminals.

#### 52. What visual separator (if any) distinguishes output sections when running multiple specific checks?
Boundaries between checks prevent visual bleed, especially when output is lengthy or contains errors.

#### 53. Should there be ASCII art, banners, or decorative elements, or stick to minimal functional output?
Consistency with existing doctor output (functional, minimal) maintains a professional CLI aesthetic.

#### 54. How should fix hints be displayed when running a specific check that fails but isn't auto-fixable?
Highlighted, actionable suggestions (commands to run, files to edit) should stand out from diagnostic details.

### States & Transitions

#### 55. What are all possible states for a specific check run (not started, running, passed, warned, failed, fixed, errored)?
Comprehensive state modeling ensures every outcome has appropriate UI treatment and messaging.

#### 56. How should the UI represent the transition from "running" to "complete" for a single check?
In-place updates (overwriting the "running..." line) feel responsive, while appending new lines shows history.

#### 57. What happens to the UI state if a check times out or hangs?
Timeout states need visible indicators and options to cancel, prevent users from thinking the terminal froze.

#### 58. How does the system move between "check failed" and "fix applied" states when using --fix?
Sequential state transitions (failed -> fixing -> fixed) should be explicit, showing the system is taking action.

#### 59. What state does the system return to after completing a single check (exit, return to prompt, suggest next action)?
Suggesting related actions ("Run 'gt doctor list' to see all checks") keeps users engaged without being pushy.

#### 60. How should the system handle rapid state changes (check completes in milliseconds)?
Very fast checks might flash too quickly to read; minimum display time or persistent output ensures users see results.

#### 61. What state indicators are needed when running multiple specific checks (current, queued, completed)?
Queue visibility (check 2 of 5) sets expectations about wait time and progress.

#### 62. How does the feature transition from discovery mode (list) to execution mode (run)?
Clear mode boundaries (different command structure, visual changes) prevent confusion about whether checks are running.

#### 63. What's the terminal state after an error - does it show partial results or reset?
Preserving partial output on error aids debugging; clearing the screen loses valuable context.

#### 64. Should there be persistent state (history, cache) that survives between invocations?
Caching last-run results enables features like "show only failing checks" without re-running everything.

#### 65. How does the system handle the state when a check modifies resources that another check depends on?
State invalidation or re-checking dependent checks prevents showing stale results after fixes.

#### 66. What visual state changes occur when a check is running with --verbose vs. normal mode?
More detailed output should feel like enhancement (additional lines) not replacement (different structure entirely).

### Additional Cross-Cutting Questions

#### 67. How should this feature behave differently in interactive vs. non-interactive (scripted) contexts?
TTY detection should enable richer output (colors, spinners) in interactive use while staying parseable in scripts.

#### 68. What accessibility considerations matter for terminal output (color-blind users, screen readers)?
Icons and colors should have text equivalents; relying solely on color for status fails for color-blind users.

#### 69. Should there be telemetry or analytics about which checks are run most frequently?
Usage data could inform defaults, shortcuts, or UI prioritization, but privacy implications need consideration.

#### 70. How should error messages be styled - inline with output, summary at end, or both?
Error visibility determines how quickly users can react; prominent placement with clear next steps reduces support burden.

#### 71. What consistency is needed with other gt commands in terms of flags, output format, and help text?
Cross-command consistency (similar --verbose behavior, help format, success indicators) reduces overall learning curve.

#### 72. Should the feature support JSON or structured output for programmatic consumption?
Machine-readable output enables automation and integration with other tools, expanding use cases beyond interactive CLI.

#### 73. How should the feature handle localization or internationalization of check names and messages?
Even if not immediately needed, considering i18n architecture (message keys vs. hardcoded strings) prevents future rewrites.

#### 74. What feedback timing feels responsive (immediate, streamed, batched) for different check durations?
Sub-second checks can show immediate results; multi-second checks need streaming progress to avoid appearing frozen.

#### 75. Should there be visual distinction between checks that modify state vs. read-only checks?
Users need to know risk level; clearly marking destructive operations builds trust and prevents accidental changes.

#### 76. How should the feature surface check metadata like category, estimated duration, and risk level?
Rich metadata helps users make informed decisions about what to run and when, especially in time-sensitive contexts.

#### 77. What should happen if a user runs a specific check while another doctor instance is already running?
Concurrency handling (lock files, warnings, queue) prevents conflicts and data corruption from simultaneous runs.

#### 78. How should success states be celebrated - minimal acknowledgment or prominent confirmation?
Positive reinforcement for successful fixes increases user confidence and satisfaction, but shouldn't be excessive.

#### 79. Should there be visual indicators for checks that require elevated permissions or have security implications?
Security-sensitive operations warrant extra visual weight (warnings, confirmations) to prevent accidental damage.

#### 80. How should the system handle and display deprecation warnings for old check names or flag combinations?
Graceful deprecation (still works but warns) gives users time to adapt while steering them toward current patterns.

---

## Domain Expert Perspective

### Feature Brief
Add support for `gt doctor <specific-doctor-fix>` like `gt doctor orphaned-sessions`

---

### Domain Concepts

#### 1. What terminology is assumed but not defined?

**Q1: What does "doctor" mean in the mental model of Gas Town users, and is it diagnostic-only or diagnostic+treatment?**
Why it matters: If users expect "doctor" to be read-only diagnostics, adding treatment via selective execution might violate their expectations.

**Q2: What is the difference between a "check", a "fix", and a "doctor fix" in user terminology?**
Why it matters: The term "specific-doctor-fix" conflates checking and fixing, which may confuse users about whether they're running diagnostics or applying fixes.

**Q3: Does "orphaned-sessions" refer to the check name, the problem type, or the fix operation?**
Why it matters: Users need to know whether they're naming the diagnostic or the solution when they type `gt doctor orphaned-sessions`.

**Q4: What is the distinction between "running a check" and "running a check with --fix" in user understanding?**
Why it matters: Selective execution might bypass the safety model where users first see what's wrong before choosing to fix it.

**Q5: Are "fixable checks" and "cleanup operations" synonymous in the domain, or are they different concepts?**
Why it matters: The context shows most fixable checks are in the Cleanup category, but this relationship needs to be explicit for users.

**Q6: What does "selectivity" mean - running one check, multiple checks, a category, or matching a pattern?**
Why it matters: The scope of selectivity determines the entire interface design and user expectations.

**Q7: Is a "doctor fix" a noun (the thing that fixes) or a verb (the act of fixing)?**
Why it matters: Language precision affects whether users think of this as running a specific check or invoking a specific repair action.

**Q8: What is the relationship between "doctor" and "health" in the Gas Town mental model?**
Why it matters: If doctor is purely about system health, adding selective execution might be seen as a "repair tool" rather than a diagnostic tool.

**Q9: Does "orphaned-sessions" as a positional argument imply automatic fixing, or just selective checking?**
Why it matters: The interaction between selective execution and the --fix flag needs to be unambiguous.

**Q10: What does "specific" mean in "specific-doctor-fix" - one exact check, a category, or a search pattern?**
Why it matters: Users might expect fuzzy matching, exact matching, or category filtering depending on what "specific" signals.

**Q11: Are there "composite" doctor operations that users think of as single fixes but require multiple checks?**
Why it matters: Users might expect `gt doctor cleanup` to run all cleanup-related checks, not just one.

**Q12: What is the difference between "doctor" and "maintenance" in user vocabulary?**
Why it matters: If users see doctor as emergency diagnostics and maintenance as routine cleanup, selective execution needs to fit one of these categories.

---

### Prior Art

#### 2. What do existing products do?

**Q13: How do other CLI health-check tools (like brew doctor, npm doctor, flutter doctor) handle selective execution?**
Why it matters: Users will bring expectations from analogous tools they've used.

**Q14: Do any popular CLI tools offer "run just this one check" functionality, and how do they expose it?**
Why it matters: Established patterns reduce learning curve and prevent UX mistakes.

**Q15: How do database migration tools handle selective execution (running one migration vs. all pending)?**
Why it matters: Doctor checks have migration-like properties (sequential, potentially dependent).

**Q16: What do linters do when users want to run specific rules vs. all rules?**
Why it matters: Linters offer both category-based and individual rule selection, which might map well to doctor checks.

**Q17: How do system administration tools like systemctl handle selective operations (one service vs. all services)?**
Why it matters: Doctor operates on system resources like sessions and processes, similar to service managers.

**Q18: What do package managers do when users want to check/fix one package vs. all packages?**
Why it matters: The selective vs. comprehensive execution pattern is well-established in package management.

**Q19: How do CI/CD systems handle running specific tests vs. full test suites?**
Why it matters: Test selectivity patterns (tags, names, patterns) might inform check selectivity design.

**Q20: What do cloud infrastructure tools (aws, terraform) do for selective plan/apply operations?**
Why it matters: Infrastructure tools separate planning (checking) from applying (fixing), similar to doctor's --fix flag.

**Q21: How do health monitoring dashboards allow users to investigate specific alerts vs. viewing all system health?**
Why it matters: The UI patterns for selective vs. comprehensive health views are well-established in monitoring tools.

**Q22: What do developer tools like cargo check, go vet offer for running specific analyses?**
Why it matters: Developer tooling often supports both "check everything" and "check this file" modes.

**Q23: How do antivirus/malware tools handle full scans vs. selective scans of specific directories?**
Why it matters: Security scanning tools have solved the problem of comprehensive vs. targeted diagnostics.

---

### Problem Depth

#### 3. Is this the real problem or a symptom?

**Q24: Are users asking for selective execution because comprehensive doctor runs are too slow?**
Why it matters: If speed is the real problem, caching or parallel execution might be better solutions.

**Q25: Are users trying to fix specific known issues without seeing irrelevant warnings?**
Why it matters: This suggests the real problem is information overload, not lack of selectivity.

**Q26: Do users want selective execution for scripting/automation purposes rather than interactive use?**
Why it matters: Automation needs are different from interactive debugging needs.

**Q27: Are users frustrated by having to run all checks just to fix one known problem?**
Why it matters: This suggests the real problem is the coupling between diagnosis and treatment.

**Q28: Do users need selective execution because some checks have side effects they want to avoid?**
Why it matters: If checks aren't side-effect-free, the real problem might be check design, not lack of selectivity.

**Q29: Are users asking for this because they don't understand what all the checks do?**
Why it matters: Better documentation or check categorization might address the underlying confusion.

**Q30: Is the request driven by expert users who know exactly what's wrong, or novices who are overwhelmed?**
Why it matters: Different user segments need different interfaces for selective execution.

**Q31: Are users trying to avoid auto-fixes they don't trust by running only specific checks?**
Why it matters: This suggests trust issues with comprehensive doctor runs, not a genuine need for selectivity.

**Q32: Do users want selective execution because doctor output is too verbose when all checks run?**
Why it matters: Output formatting might be the real problem, not the inability to filter checks.

**Q33: Are users trying to debug doctor itself by running checks in isolation?**
Why it matters: Developer/maintainer needs might be driving this request rather than end-user needs.

---

### Related Problems

#### 4. What related problems will users expect us to solve?

**Q34: Once users can run one check, will they expect to run multiple specific checks in one command?**
Why it matters: This determines whether we need to support `gt doctor check1 check2 check3` syntax.

**Q35: Will users expect to run all checks in a category (like "cleanup") without naming each one?**
Why it matters: Category-based selection might be more useful than individual check selection.

**Q36: Will users expect to exclude specific checks from a comprehensive run?**
Why it matters: Exclusion patterns (`gt doctor --except orphaned-sessions`) are the inverse of selection.

**Q37: Will users want to save and name their own check combinations for repeated use?**
Why it matters: Power users might want profiles like `gt doctor --profile my-routine-checks`.

**Q38: Will users expect tab completion for check names?**
Why it matters: Discoverability is critical if users need to type check names manually.

**Q39: Will users expect a way to list all available checks before running selective execution?**
Why it matters: Discovery mechanism is essential before selective execution is useful.

**Q40: Will users expect search/filter functionality to find the check they want?**
Why it matters: With 47 checks, users may not remember exact names.

**Q41: Will users expect selective execution to respect dependencies between checks?**
Why it matters: Some checks might depend on others running first.

**Q42: Will users expect to run checks matching a pattern (like "orphan*" to get all orphan-related checks)?**
Why it matters: Pattern matching is more flexible than exact name matching.

**Q43: Will users expect to run all failed checks from a previous run?**
Why it matters: Repairing after diagnosis is a common workflow.

**Q44: Will users expect different output formats for selective vs. comprehensive runs?**
Why it matters: Selective runs might not need category grouping or summary statistics.

**Q45: Will users expect to schedule or automate selective check execution?**
Why it matters: Cron-style automation might be a use case for selective execution.

---

### Non-Goals

#### 5. What are we explicitly NOT solving?

**Q46: Are we solving for running checks in a specific order, or just selecting which checks run?**
Why it matters: Ordering control is a different feature from selection.

**Q47: Are we making checks configurable (like tuning thresholds), or just selectable?**
Why it matters: Configuration is a much larger scope than selection.

**Q48: Are we providing a way to create custom checks, or just run existing ones selectively?**
Why it matters: Plugin/extension architecture is beyond selective execution.

**Q49: Are we solving for distributed/remote doctor execution, or just local selective runs?**
Why it matters: Remote execution adds complexity that might not be in scope.

**Q50: Are we creating a GUI/TUI for check selection, or keeping it CLI-only?**
Why it matters: Interactive selection changes the entire UX design.

**Q51: Are we adding check scheduling/recurring execution, or just one-off selective runs?**
Why it matters: Scheduling is a separate feature from selection.

**Q52: Are we solving for check result history/tracking, or just current-state execution?**
Why it matters: Historical analysis is a different domain from selective execution.

**Q53: Are we enabling partial fixes (fix only some issues found by a check), or all-or-nothing fixing?**
Why it matters: Granular fix control is more complex than check selection.

**Q54: Are we adding check composition (combining checks into new checks), or just running existing checks?**
Why it matters: Composability is a different abstraction from selection.

**Q55: Are we solving for multi-rig doctor runs, or single-rig selective execution?**
Why it matters: Multi-rig orchestration is beyond selective execution scope.

---

### Edge Cases (Domain)

#### 6. What unusual but valid scenarios exist?

**Q56: What happens if a user specifies a check name that's ambiguous or matches multiple checks?**
Why it matters: Fuzzy matching and exact matching have different failure modes.

**Q57: What if a user wants to run a check that only makes sense in combination with other checks?**
Why it matters: Check dependencies might make selective execution invalid in some cases.

**Q58: What if a fixable check requires human confirmation for some issues but not others?**
Why it matters: Selective execution might bypass normal safety confirmations.

**Q59: What if a user specifies both a category and individual checks, and they overlap?**
Why it matters: Combination selection logic needs to be well-defined.

**Q60: What if a check is conditionally available (like rig-specific checks) and the user tries to run it selectively?**
Why it matters: Availability constraints need to interact correctly with selection.

**Q61: What if running one check would normally trigger others (like cascading checks), but selective execution prevents that?**
Why it matters: Isolation might break intended check relationships.

**Q62: What if a user runs a check selectively that would normally be skipped due to context (like being in a crew)?**
Why it matters: Selective execution might override contextual filtering.

**Q63: What if the same issue is detected by multiple checks, and the user runs only one selectively?**
Why it matters: Partial diagnosis might give incomplete information.

**Q64: What if a check's fix operation depends on state established by another check's run?**
Why it matters: Selective fixing might fail if prerequisite checks didn't run.

**Q65: What if a user tries to run "all cleanup checks" but some are unsafe to run together?**
Why it matters: Category selection might accidentally combine incompatible operations.

**Q66: What if a check name conflicts with a command-line flag or reserved keyword?**
Why it matters: Parsing ambiguity could cause unexpected behavior.

**Q67: What if a user wants to run a check that was deprecated or removed?**
Why it matters: Version migration might leave users with outdated check names in scripts.

---

### Regulatory/Compliance

#### 7. Regulatory or compliance considerations?

**Q68: If doctor operations affect shared resources (like tmux sessions), do selective runs need audit logging?**
Why it matters: Compliance requirements might mandate logging all system modifications.

**Q69: If doctor is used in production environments, do selective fixes need approval workflows?**
Why it matters: Some organizations require change approval before system modifications.

**Q70: Do selective doctor operations need to respect maintenance windows or change freezes?**
Why it matters: Production systems might have temporal constraints on when fixes can run.

**Q71: If doctor operations delete data (like old sessions or wisps), do they need retention policy compliance?**
Why it matters: Data retention regulations might require preserving artifacts before deletion.

**Q72: Do selective doctor runs need to be reproducible for compliance audits?**
Why it matters: Audit trails might require exact replay of what checks ran and what they found.

---

### Cultural/Regional

#### 8. Cultural or regional variations?

**Q73: Do different development teams within Gas Town have different "standard" doctor workflows?**
Why it matters: Team-specific conventions might drive different selective execution patterns.

**Q74: Are there cultural differences in how teams perceive "safe" vs. "risky" auto-fixes?**
Why it matters: Risk tolerance affects whether selective execution should default to fix or check-only.

**Q75: Do some teams use doctor as a learning tool (verbose, exploratory) vs. a maintenance tool (quick, targeted)?**
Why it matters: Different use cases might need different selective execution interfaces.

**Q76: Are there regional/team conventions around naming (kebab-case vs. snake_case vs. camelCase) for check names?**
Why it matters: Input parsing needs to accommodate expected naming conventions.

---

### Success Criteria

#### 9. How would we know this succeeded?

**Q77: What percentage reduction in doctor execution time would users consider successful?**
Why it matters: If selective execution doesn't save meaningful time, it adds complexity without value.

**Q78: Would success mean users run doctor more frequently because it's now targeted and fast?**
Why it matters: Increased usage frequency indicates the feature solved a real friction point.

**Q79: Would success mean fewer support requests about "how do I fix just this one thing"?**
Why it matters: Support burden reduction is a measurable success metric.

**Q80: Would success mean automated scripts using selective doctor execution instead of comprehensive runs?**
Why it matters: Automation adoption indicates the feature is reliable and useful.

**Q81: Would success mean users discover and fix issues faster because they can focus on specific problems?**
Why it matters: Time-to-resolution is a key outcome metric.

**Q82: Would success mean users run comprehensive doctor less often because selective execution handles routine issues?**
Why it matters: This could indicate either success (targeted efficiency) or failure (users avoiding thorough checks).

**Q83: Would success mean new users can understand and use selective execution without documentation?**
Why it matters: Discoverability and intuitiveness are critical for feature adoption.

**Q84: Would success mean zero increase in support questions about "why didn't doctor catch this"?**
Why it matters: Selective execution shouldn't create blind spots that comprehensive runs would catch.

---

### Quality Indicators

#### 10. What does 'good' look like in this domain?

**Q85: Does "good" selective execution feel like running a precise surgical tool rather than a blunt instrument?**
Why it matters: User perception of control and precision drives satisfaction.

**Q86: Does "good" mean check names are immediately obvious and memorable without consulting documentation?**
Why it matters: Cognitive load in remembering check names affects usability.

**Q87: Does "good" mean error messages for invalid check names suggest the correct name?**
Why it matters: Recovery from mistakes is a key quality indicator.

**Q88: Does "good" mean selective execution is safe by default (requires --fix, not automatic fixing)?**
Why it matters: Safety defaults prevent accidental system modifications.

**Q89: Does "good" mean output for selective runs is cleaner and more focused than comprehensive runs?**
Why it matters: Reduced noise is a key benefit of selective execution.

**Q90: Does "good" mean selective execution composes well with other doctor flags (--verbose, --rig, etc.)?**
Why it matters: Feature interaction quality affects overall user experience.

**Q91: Does "good" mean users can run selective doctor in muscle-memory mode (fast, automatic, reliable)?**
Why it matters: Routine operations should require minimal cognitive effort.

---

### Metrics

#### 11. What metrics matter to users?

**Q92: Do users care about execution time savings, or just perceived responsiveness?**
Why it matters: Actual vs. perceived performance might drive different optimization choices.

**Q93: Do users care about the number of checks run, or just whether their specific problem got diagnosed?**
Why it matters: Completeness vs. targeted effectiveness represent different value propositions.

**Q94: Do users track how often they run doctor with --fix vs. check-only?**
Why it matters: Fix frequency indicates trust in auto-repair capabilities.

**Q95: Do users measure doctor's false positive rate (warnings about non-issues)?**
Why it matters: False positives drive users toward selective execution to avoid noise.

**Q96: Do users track how many issues doctor finds vs. how many they knew about already?**
Why it matters: Discovery value vs. confirmation value affects usage patterns.

**Q97: Do users care about how many keystrokes it takes to run a selective check?**
Why it matters: Command brevity affects daily friction for frequent operations.

**Q98: Do users measure the success rate of auto-fixes (fixed without manual intervention)?**
Why it matters: Fix reliability determines whether selective execution is useful for automation.

**Q99: Do users care about comprehensive coverage (did we check everything), or targeted effectiveness (did we fix this problem)?**
Why it matters: Different metrics drive comprehensive vs. selective execution preferences.

---

### Problem Validation

#### 12. Additional Problem Space Questions

**Q100: Who is the primary user for selective doctor execution - developers, operators, or automation?**
Why it matters: Different user types need different interfaces and defaults.

**Q101: What is the typical frequency of doctor runs - daily, hourly, on-demand, or automated?**
Why it matters: Frequency affects whether selective execution needs to be fast or just convenient.

**Q102: What percentage of doctor runs result in using --fix vs. just checking?**
Why it matters: Fix frequency indicates whether selective execution should make fixing easier or just checking easier.

**Q103: Do users typically know what's wrong before running doctor, or do they run it to discover issues?**
Why it matters: Diagnostic vs. repair use cases need different selective execution designs.

**Q104: How often do users need to run the same specific check repeatedly during troubleshooting?**
Why it matters: Repetition frequency justifies the effort of adding selective execution.

**Q105: What is the pain point users experience with current comprehensive doctor runs?**
Why it matters: Understanding the pain helps validate whether selective execution solves it.

**Q106: Do users run doctor because they have to (automated/compliance) or because they want to (maintenance)?**
Why it matters: Mandatory vs. voluntary usage affects feature prioritization.

**Q107: How do users currently work around the lack of selective execution (scripts, manual commands)?**
Why it matters: Existing workarounds reveal the true need and expected interface.

**Q108: What would users do if selective execution existed but was slower than comprehensive runs?**
Why it matters: Speed vs. selectivity trade-offs need to align with user priorities.

**Q109: Do users trust doctor enough to run selective fixes without reviewing comprehensive diagnostics first?**
Why it matters: Trust level affects whether selective execution is used for fixes or just checks.

**Q110: What is the skill level of typical doctor users - novices who need guidance or experts who want control?**
Why it matters: Expertise level determines interface complexity and default safety levels.

---

## Cross-Perspective Themes (Sonnet)

### 1. Discovery and Discoverability
Across all three perspectives, there is strong consensus that users need effective ways to discover, remember, and understand check names. The User Advocate raises questions about exact vs. partial matching (Q4, Q22), the Product Designer focuses on information architecture and list views (Q1-14), and the Domain Expert questions prior art and discoverability mechanisms (Q13, Q38-42). This suggests that check name discoverability is a critical blocker to adoption and safe usage.

### 2. Behavioral Clarity and Scope Communication
All three perspectives emphasize uncertainty about what "selective execution" means operationally. The User Advocate asks what the command actually runs (Q1, Q9), the Product Designer asks about visual distinction between modes (Q7, Q47), and the Domain Expert questions the fundamental meaning of "selective" (Q6, Q27). There is a clear need for unambiguous communication about scope, dependencies, and what gets executed.

### 3. Safety Defaults and Risk Management
Safety concerns appear consistently across all perspectives: User Advocate questions around confirmation prompts (Q2, Q19 in Product Designer), auto-fixing behavior (Q16 in Product Designer), and safety assumptions (Q88 in Domain Expert). This suggests selective execution must have strong safety guarantees and clear risk signaling, with explicit confirmation for destructive operations.

### 4. Check Dependencies and Relationships
All three perspectives identify the problem of running checks in isolation when they have dependencies or relationships. User Advocate discusses order-of-operations (Q65 in Domain Expert), Product Designer covers dependency flows (Q34, Q41 in Domain Expert), and Domain Expert explicitly names this as a concern (Q41, Q57, Q64). This is likely a significant implementation constraint that needs upfront resolution.

### 5. Output Clarity for Selective vs. Comprehensive Runs
Across all perspectives, questions arise about whether selective runs should look different from comprehensive runs. User Advocate mentions summary footer differences (Q8), Product Designer covers output format changes (Q30, Q44, Q50), and Domain Expert questions output expectations (Q44, Q89). This suggests the UI needs explicit visual signals that communicate which mode is active and reduce cognitive friction.

