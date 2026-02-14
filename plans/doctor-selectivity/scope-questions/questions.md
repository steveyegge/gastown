# Scope Questions: Doctor Selectivity

## Synthesis Statistics
- Raw questions across 9 analyses: 731
- After deduplication: 89
- P0: 18 | P1: 27 | P2: 25 | P3: 19 (total: 89)

## Cross-Model Agreement Summary

### Where all 3 models converged (high confidence themes)
All three models strongly converged on five themes:

1. **Discoverability is foundational**: Every model, from every perspective, flagged that users cannot use selective execution if they cannot discover check names. A `gt doctor list` command, shell tab-completion, and helpful error messages were universally identified as critical.

2. **The check-vs-fix ambiguity must be resolved**: All models identified tension around whether `gt doctor <name>` runs a diagnostic or applies a fix. The brief's title ("specific-doctor-fix") leans toward fixing, but the existing `--fix` flag separates the two. This is the single most important design decision.

3. **Safety and check ordering matter**: All models recognized that selective runs bypass the deliberate ordering of the full doctor suite (fundamentals first, cleanup later). Running a check in isolation may produce misleading results or skip safety precautions.

4. **Output format must signal scope**: All models asked whether single-check output should differ from full-suite output. The consensus is that output must clearly communicate what scope is being run (single check, category, full) so users always know what they are looking at.

5. **Error handling and typo recovery are critical for adoption**: All models flagged that typos, near-misses, and invalid check names are inevitable, and that "did you mean?" suggestions are essential for a good experience.

### Where models diverged
- **Haiku** went deepest on accessibility, mental models, reliability/idempotency, and performance concerns. It raised unique questions about dry-run accuracy, audit logging, caching, and timezone handling.
- **Sonnet** went deepest on domain concepts, prior art breadth, non-goals/scope boundaries, regulatory/compliance considerations, and metrics/success validation. It uniquely raised questions about exclusion patterns, check composition, and approval workflows.
- **Opus** went deepest on information architecture, visual design details, state transitions, and product-level UX flows. It uniquely raised questions about the "doctor as two jobs" architectural concern and monitoring dashboard analogies.

### Where models disagreed
- **Verbose default for single checks**: Opus's Product Designer suggested single checks should auto-expand verbose details. Haiku and Sonnet questioned whether verbose should remain opt-in. No consensus.
- **Category filtering scope**: Opus treated category filtering as a natural part of selectivity. Sonnet questioned whether it belongs in scope at all. Haiku sat in between.
- **Dry-run as requirement vs. nice-to-have**: Haiku and Sonnet flagged dry-run as important for safety. Opus mentioned it but with less emphasis, focusing more on the `--fix` flag as sufficient consent.

---

## P0: Must Answer (Critical Design Decisions)

### Core Behavioral Contract

1. **Does `gt doctor <name>` run only the diagnostic check, or does it also apply the fix? Is `--fix` still required for selective runs?** [Opus, Haiku, Sonnet]
   *Why it matters: This is the single biggest UX ambiguity. The brief title says "specific-doctor-fix" implying fixing, but the existing system separates diagnosis from repair with `--fix`. Users will assume one or the other, and a wrong guess either misses a fix or triggers one unexpectedly.*

2. **Does `gt doctor <name>` run ONLY that specific check (scoped execution), or does it run all checks but only show/fix that one (filtered output)?** [Opus, Haiku, Sonnet]
   *Why it matters: These are fundamentally different architectures with different performance profiles and safety implications. Users running a single check expect it to be faster than the full suite.*

3. **Does `gt doctor` with no arguments continue to work exactly as before?** [Opus, Haiku, Sonnet]
   *Why it matters: Any regression in zero-argument default behavior breaks existing workflows, muscle memory, and scripts. Backward compatibility is non-negotiable.*

### Discoverability

4. **How does a user discover which check names are available without reading source code or external documentation?** [Opus, Haiku, Sonnet]
   *Why it matters: Without a runtime discovery mechanism (list command, help text, error suggestions), users cannot use the feature. Every model flagged this as foundational to adoption.*

5. **Should there be a `gt doctor list` subcommand, a `gt doctor --list` flag, or both? What information should it show (names, descriptions, categories, fixable status)?** [Opus, Haiku, Sonnet]
   *Why it matters: The listing mechanism is the primary entry point to the feature. Its design determines whether users can find the right check quickly or give up in frustration.*

6. **What error message does the user see for an unrecognized check name, and does it include "did you mean X?" suggestions?** [Opus, Haiku, Sonnet]
   *Why it matters: Typos are the most common error. A helpful suggestion with the correct command to copy-paste is the difference between a frustrating and a delightful experience.*

### CLI Syntax and Flag Design

7. **Should the check name be a positional argument (`gt doctor orphaned-sessions`) or a flag (`gt doctor --check orphaned-sessions`)?** [Opus, Haiku, Sonnet]
   *Why it matters: Positional arguments feel more natural for targeting but create namespace conflicts with potential subcommands like `list`. Flags are more explicit and composable. This determines the entire CLI grammar.*

8. **Can users pass multiple check names in one invocation (e.g., `gt doctor orphaned-sessions wisp-gc`)?** [Opus, Haiku, Sonnet]
   *Why it matters: Running two targeted checks in one invocation is a natural expectation. Forcing separate invocations feels needlessly restrictive, but supporting multiple args complicates parsing and output.*

9. **Should `gt doctor cleanup` (a category name) run all checks in that category, or look for a check literally named "cleanup"?** [Opus, Haiku, Sonnet]
   *Why it matters: If category names and check names share the same argument namespace, ambiguity will confuse users. The resolution strategy (separate flag, distinct namespace, or priority rules) determines the feature's UX surface.*

### Safety

10. **When a check is run in isolation, does it produce different (potentially misleading) results compared to running it as part of the full suite, due to missing context from prerequisite checks?** [Opus, Haiku, Sonnet]
    *Why it matters: Full doctor runs have deliberate check ordering (fundamentals first, cleanup later) for safety. If running `orphan-sessions` alone could kill a session that another check would have identified as in-use, selective execution itself is the danger.*

11. **Should there be a confirmation prompt before applying a fix in selective mode, or is `--fix` sufficient consent?** [Opus, Haiku, Sonnet]
    *Why it matters: Targeted invocation with `--fix` is arguably double-consent, but destructive fixes (killing sessions, deleting data) might still warrant a prompt. The answer also affects scriptability.*

### Output Format

12. **Should single-check output use the same streaming, categorized format as the full doctor run, or a simplified layout?** [Opus, Haiku, Sonnet]
    *Why it matters: The full doctor output with headers, separators, and summary may feel excessive for a single check. But inconsistency between modes confuses users. The output must visually signal the scope.*

13. **Should the summary line ("1 passed, 0 warnings, 0 failed") be shown for a single-check run, or is a simpler message more appropriate?** [Opus, Haiku, Sonnet]
    *Why it matters: A summary of one item feels redundant, but omitting it breaks the mental model users have from full runs. This decision extends to scripted consumers that may parse the summary format.*

### Performance

14. **Should selective runs be meaningfully faster than full runs? What is the acceptable time for a single-check run?** [Opus, Haiku, Sonnet]
    *Why it matters: If running one check still initializes the full doctor framework and takes nearly the same time, the feature's primary value proposition is gone. Users expect targeted execution to be fast.*

### Naming and Matching

15. **Should fuzzy/prefix/substring matching be supported (e.g., `gt doctor orphan` matching `orphan-sessions`), or must names match exactly?** [Opus, Haiku, Sonnet]
    *Why it matters: Users of tools like kubectl, docker, and git are accustomed to partial matching. But fuzzy matching can lead to unintended execution. The tradeoff is convenience vs. precision.*

16. **How should the system handle common input variations: underscores vs. hyphens (`orphan_sessions`), different casing (`OrphanSessions`), close misspellings (`orphen-sessions`)?** [Opus, Haiku, Sonnet]
    *Why it matters: Kebab-case vs. snake_case and case sensitivity mistakes are extremely common. Accepting variations or suggesting corrections prevents dead-end experiences.*

### Scripting and Automation

17. **Should selective runs produce non-zero exit codes on failure, consistent with full runs, for scripting and CI/CD use?** [Opus, Haiku, Sonnet]
    *Why it matters: The exit code contract is the most important interface for automation. If selective runs have different exit code semantics, scripts will break.*

18. **Should the output detect non-TTY contexts (pipes, redirects) and produce clean, parseable output without ANSI codes and streaming updates?** [Opus, Haiku, Sonnet]
    *Why it matters: Selective runs are more likely to be scripted than full runs. Streaming output with ANSI codes and carriage returns produces garbled pipe output.*

---

## P1: Should Answer

### Fix Behavior and Feedback

19. **What happens when `--fix` is passed with a check that is not fixable?** [Opus, Haiku, Sonnet]
    *Why it matters: The user needs to understand why the fix did not happen -- is it because the check passed, or because the check cannot be auto-fixed? This is a predictable user error that the response should educate about.*

20. **After a selective fix succeeds, should the check automatically re-run to verify, or just report "fixed"?** [Opus, Haiku, Sonnet]
    *Why it matters: Auto-verification builds confidence that the fix actually worked. Skipping it leaves doubt, but adds runtime.*

21. **What happens when a fix partially succeeds (e.g., kills 3 of 5 orphaned sessions)?** [Opus, Haiku, Sonnet]
    *Why it matters: Partial success is ambiguous. The user needs to know what was fixed, what remains, and whether to retry.*

22. **What feedback does the user see immediately after invoking a targeted check, before results arrive?** [Opus, Haiku, Sonnet]
    *Why it matters: Instant feedback ("Running orphan-sessions check...") confirms the command was understood. Silence causes uncertainty, especially for slow checks.*

### Interaction with Existing Flags

23. **Should selective runs support all the same flags as full runs (`--verbose`, `--slow`, `--rig`)?** [Opus, Haiku, Sonnet]
    *Why it matters: Flag compatibility is a completeness criterion. If some flags do not work with selective runs, users will file bugs. Users expect flags to compose naturally.*

24. **What happens when a user specifies a rig-specific check without the `--rig` flag?** [Opus, Haiku, Sonnet]
    *Why it matters: Rig checks require rig context. Running one without `--rig` should either error clearly or auto-detect, but both choices have consequences. The check may not even be registered without the flag.*

25. **How should `--verbose` interact with targeted checks? Should single-check mode auto-expand details since targeting implies wanting depth?** [Opus, Haiku, Sonnet]
    *Why it matters: Running a targeted check implies the user wants depth. Hiding details behind `--verbose` may feel like unnecessary friction, but auto-expanding changes the default behavior.*

### Discoverability and Help

26. **Should tab-completion be supported for check names?** [Opus, Haiku, Sonnet]
    *Why it matters: Tab-completion dramatically improves discoverability and reduces typos, but requires shell integration work. Without it, users must memorize or look up exact check names.*

27. **How should existing `gt doctor --help` text be updated to show the selective syntax? Should it be prominent or a footnote?** [Opus, Haiku, Sonnet]
    *Why it matters: If the feature is invisible in help text, adoption will be low. Placement determines whether users discover it organically or need external training.*

28. **Should the full `gt doctor` output include a hint about selective runs (e.g., "Tip: run `gt doctor <check-name>` to target a specific check")?** [Opus, Haiku]
    *Why it matters: Users with muscle memory for `gt doctor` need a nudge to discover the new capability. A subtle hint drives adoption without cluttering.*

### Edge Cases

29. **What if the user runs `gt doctor all` expecting it to mean "run everything"?** [Opus, Sonnet]
    *Why it matters: If `all` is not a recognized keyword, it will be treated as a check name and fail confusingly.*

30. **What if a check name conflicts with a potential subcommand or reserved word (`list`, `help`, `version`, `all`)?** [Opus, Haiku, Sonnet]
    *Why it matters: If `gt doctor list` is later added as a subcommand but a check is named `list`, the namespace collision creates ambiguity. This must be designed proactively.*

31. **What if the user hits Ctrl+C during a targeted fix mid-execution? Can fixes be interrupted cleanly?** [Opus, Haiku, Sonnet]
    *Why it matters: Interrupted fixes could leave the system in a worse state than before. Fixes should be atomic or clearly warn about partial application.*

32. **What if two targeted doctor commands run simultaneously in different terminals?** [Opus, Haiku, Sonnet]
    *Why it matters: Concurrent fixes on the same resource (e.g., orphaned sessions) could conflict. Locking or warning behavior needs to be defined.*

33. **What if the user runs `gt doctor orphaned-sessions` from outside a Gas Town workspace?** [Opus, Sonnet]
    *Why it matters: Some checks require workspace context. Running from outside one should fail clearly with guidance, not a stack trace.*

### Problem Depth

34. **Is the real problem that `gt doctor` is too slow, and selectivity is a workaround for speed?** [Opus, Haiku, Sonnet]
    *Why it matters: If the full run took 2 seconds, nobody would ask for selectivity. Understanding whether this is a performance problem disguised as a UX request changes the solution.*

35. **Is the real problem that users want to run a fix without the diagnostic noise, and a `gt fix orphan-sessions` command would be more direct?** [Opus, Haiku, Sonnet]
    *Why it matters: If users already know what is broken, the doctor framing may be the wrong abstraction. The answer determines whether we are building the right feature.*

36. **Is the real problem that doctor output is overwhelming and selectivity is a filtering mechanism, where better output formatting would suffice?** [Opus, Haiku, Sonnet]
    *Why it matters: If the real issue is signal-to-noise in a 47-check run, collapsing passing checks or showing only failures might solve the problem without adding selective execution at all.*

### Visual Design

37. **How should the output visually differ between single-check, category, and full runs so users always know the scope?** [Opus, Haiku, Sonnet]
    *Why it matters: Three scopes need distinct but related visual treatments. Without clear scope signaling, users may not realize they ran only one check or an entire category.*

38. **Should the listing of available checks show fixable vs. read-only status with a visual indicator?** [Opus, Haiku, Sonnet]
    *Why it matters: If the primary use case for targeting is fixing, making fixable checks visually pop saves scanning time and prevents wasted attempts on unfixable checks.*

### Stale Names and Version Skew

39. **What if a user runs a check name from outdated documentation, a teammate's message, or a script, but the check was renamed or removed?** [Opus, Haiku, Sonnet]
    *Why it matters: Stale instructions are inevitable. The error message for an unrecognized check name should mention if it was renamed/removed, or suggest the closest current name.*

### Check Descriptions

40. **Should users be able to see what a check does before running it (description, what it fixes, risks)?** [Opus, Haiku, Sonnet]
    *Why it matters: Users should not have to run a check to find out what it does. A description mode is essential for informed choice, especially for checks with domain-specific names like "wisp-gc".*

### Passing Check Output

41. **When a targeted check passes, what does the output look like? Is "check passed" without context sufficient?** [Opus, Haiku, Sonnet]
    *Why it matters: A bare "check passed" may not reassure users. They may need to see what was checked and why it passed. But too much output for a passing check feels like noise.*

### Architecture

42. **Should fixable cleanup operations (wisp-gc, orphan-sessions) be part of doctor at all, or should they be separate maintenance commands?** [Opus, Haiku]
    *Why it matters: Doctor may be doing two jobs -- diagnostics and cleanup maintenance. If the real problem is architectural, selective execution papers over the wrong abstraction.*

### Flag Ordering

43. **Should both `gt doctor orphaned-sessions --fix` and `gt doctor --fix orphaned-sessions` work?** [Opus, Sonnet]
    *Why it matters: Flag ordering varies across CLI tools and users will try both. If one silently does something different, trust breaks.*

### Wildcard/Glob Support

44. **Should glob or wildcard patterns be supported (e.g., `gt doctor 'orphan-*'`)?** [Opus, Sonnet]
    *Why it matters: Power users pattern-match by reflex. If this silently matches nothing, they lose trust. But glob support adds parsing complexity.*

### Dry-Run

45. **Should there be a `--dry-run` mode that previews what a fix would do without applying it?** [Opus, Haiku, Sonnet]
    *Why it matters: Dry-run reduces anxiety about destructive fixes, builds trust for new users, and is standard practice in system administration tools.*

---

## P2: Good to Have

### Output Details

46. **When running a single check, should the output include the category header for context?** [Opus, Haiku]
    *Why it matters: Category headers add context about where the check fits but feel heavy for a single targeted run. The category helps users understand the check's purpose.*

47. **Should the output include before/after metrics for fixes (e.g., "Before: 7 sessions, Now: 2 sessions")?** [Haiku]
    *Why it matters: Showing what changed makes the action feel real and measurable, giving users confidence the fix worked.*

48. **How should elapsed time be displayed for a single check vs. a batch?** [Opus, Haiku]
    *Why it matters: Timing information is useful in a full suite to spot slow checks. For a single check it may be noise, or it may be the whole point depending on the user's concern.*

49. **Should checks that were NOT run be indicated (e.g., "Skipped 46 other checks")?** [Haiku, Sonnet]
    *Why it matters: Reassurance that the user did not accidentally disable all checks. Transparency about scope builds trust.*

50. **For checks with many details (50+ orphaned sessions), should output paginate, truncate with a count, or show everything?** [Opus, Haiku]
    *Why it matters: Very long outputs overflow the terminal. Users need reasonable defaults that scale, with the ability to see full detail when wanted.*

### Accessibility

51. **Are screen reader users able to parse the output with icons and ANSI formatting? Do icons need text equivalents?** [Opus, Haiku, Sonnet]
    *Why it matters: Icons (checkmarks, wrenches, hourglasses) alone exclude screen reader users. Text equivalents are needed for accessibility.*

52. **Are check statuses distinguishable without color for users with color blindness?** [Opus, Haiku, Sonnet]
    *Why it matters: If pass/fail relies solely on green/red coloring, colorblind users cannot distinguish results. Icons help but must be sufficient alone.*

53. **Is the feature usable over slow SSH connections where streaming output may lag?** [Opus, Haiku, Sonnet]
    *Why it matters: Real-time character-by-character updates can be painful over high-latency connections. A simpler output mode would help.*

54. **Are check names meaningful to non-native English speakers? Do terms like "orphan," "wisp," "stale beads redirect" create barriers?** [Opus, Haiku, Sonnet]
    *Why it matters: Metaphorical English and internal jargon are opaque to international users and newcomers. Plain language descriptions alongside names help.*

### Edge Cases

55. **What if the user passes an empty string, whitespace, or special characters as the check name?** [Opus, Haiku]
    *Why it matters: Edge-case input should fall back to default behavior or produce a clear error, not panic.*

56. **What if `--fix` with no check name (current behavior) changes semantics when selective mode is added?** [Opus]
    *Why it matters: Adding positional arguments must not alter the semantics of existing flag-only invocations. `gt doctor --fix` must continue to fix everything.*

57. **What if a user expects `gt doctor orphaned-sessions --verbose` to show more detail specifically about that check?** [Opus, Sonnet]
    *Why it matters: Verbose mode scoped to a single check should feel more useful and focused, not just the same wall of text.*

58. **What if a category name collides with a check name?** [Opus, Sonnet]
    *Why it matters: If both a check and a category are named `cleanup`, which takes priority? Ambiguity must be resolved explicitly with clear rules.*

### User Journeys

59. **What triggers a user to run a specific check instead of the full doctor? Is it a known issue, a CI failure, a colleague's suggestion?** [Opus, Haiku]
    *Why it matters: Understanding the trigger shapes whether the feature should optimize for speed, clarity, or automation.*

60. **How does the user transition from seeing a warning in `gt doctor` output to running just that check?** [Opus, Haiku, Sonnet]
    *Why it matters: The full doctor output should make selective re-runs discoverable. If the warning includes the exact command to run, the transition is frictionless.*

61. **After fixing with a targeted command, does the user need to run `gt doctor` again to verify overall health?** [Opus, Haiku, Sonnet]
    *Why it matters: Users should not need multiple commands to achieve confidence. The targeted fix should either verify itself or guide the user to the next step.*

### Prior Art

62. **How do analogous tools (brew doctor, flutter doctor, npm doctor, pre-commit run) handle selective checks, and what can we learn?** [Opus, Haiku, Sonnet]
    *Why it matters: Most "doctor" commands run all checks with no selectivity. Understanding why (simplicity, safety, holistic integrity) informs our design decision.*

63. **How does `pre-commit run <hook-id>` handle selective execution, and is this the closest analogy?** [Opus]
    *Why it matters: pre-commit allows running a single hook by ID, which is structurally very similar to selective doctor checks. Its UX patterns are directly applicable.*

### Safety and Dependencies

64. **Should checks that depend on other checks auto-run their prerequisites, or warn the user about dependencies?** [Opus, Haiku, Sonnet]
    *Why it matters: Hidden dependencies can make results confusing. Surfacing them adds cognitive load but prevents misleading results.*

65. **Are there checks that are dangerous to run in isolation because they lack context from other checks?** [Opus, Sonnet]
    *Why it matters: If running `orphan-sessions` alone could kill a session that another check would have identified as in-use, isolation itself is dangerous. This must be explicitly validated.*

### Structured Output

66. **Should there be a JSON or structured output mode for programmatic consumption?** [Haiku, Sonnet]
    *Why it matters: Machine-readable output enables automation and integration with other tools, expanding use cases beyond interactive CLI.*

### Success Metrics

67. **What adoption target should we set? What percentage of doctor runs should become selective after this ships?** [Opus, Haiku, Sonnet]
    *Why it matters: If 80% of runs remain full `gt doctor`, the feature has low adoption and might not have been worth the complexity.*

68. **Should we track which checks are most commonly run selectively to inform future decisions?** [Opus, Haiku, Sonnet]
    *Why it matters: Telemetry on selective check usage would inform which checks to optimize, promote to standalone commands, or deprecate.*

### Reversibility

69. **After running `gt doctor orphaned-sessions --fix`, can users undo the fix? Are fixes reversible?** [Haiku, Sonnet]
    *Why it matters: Users fear destructive operations. Clarity on reversibility reduces anxiety. If fixes are irreversible, that must be communicated upfront.*

70. **Are fixes idempotent -- can they be safely run multiple times without causing harm?** [Haiku, Sonnet]
    *Why it matters: Idempotent operations build confidence. If running a fix twice breaks something, users will hesitate to use the feature.*

---

## P3: Parking Lot

### Advanced Features

71. **Should there be a way to exclude specific checks from a comprehensive run (e.g., `gt doctor --except orphaned-sessions`)?** [Sonnet]
    *Why it matters: Exclusion patterns are the inverse of selection and a natural extension, but may be out of scope for v1.*

72. **Should users be able to save and name their own check combinations as profiles (e.g., `gt doctor --profile my-routine-checks`)?** [Sonnet]
    *Why it matters: Power users might want custom check sets, but this adds significant complexity beyond basic selective execution.*

73. **Should selective doctor output be cached so users can see the last result without re-running?** [Opus, Haiku]
    *Why it matters: Cached results let users verify state without waiting, but stale data could be misleading. This is an optimization for later.*

74. **Should there be a "getting started" interactive guide mode (e.g., `gt doctor --guide`)?** [Haiku]
    *Why it matters: Some users benefit from interactive guidance, but this is a separate feature that can be added after the core selectivity ships.*

75. **Should checks have difficulty, risk, or "experimental" labels displayed in listings?** [Haiku, Sonnet]
    *Why it matters: Risk labels help users make informed decisions, but designing a labeling system is additional scope.*

76. **Should there be a way to run all failed checks from a previous run (e.g., `gt doctor --rerun-failed`)?** [Sonnet]
    *Why it matters: Repairing after diagnosis is a common workflow, but this requires state persistence between runs.*

77. **Should there be support for check scheduling or cron-like recurring selective execution?** [Opus, Haiku, Sonnet]
    *Why it matters: Automation is a natural extension but is a separate feature from one-off selective runs.*

### Infrastructure and Operations

78. **What permissions does a user need to run specific checks? Do some require elevated privileges?** [Haiku, Sonnet]
    *Why it matters: Permission errors should produce clear feedback, but the permission model is orthogonal to selective execution.*

79. **Do any checks require network access, and what happens offline?** [Haiku]
    *Why it matters: Network-dependent features should declare their dependencies, but this is a check-level concern, not a selectivity concern.*

80. **Do any checks depend on specific config files or environment variables being present?** [Haiku]
    *Why it matters: Hidden dependencies cause cryptic errors, but documenting check prerequisites is a broader effort.*

81. **Should selective doctor operations be logged differently than full runs for audit purposes?** [Haiku, Sonnet]
    *Why it matters: Audit trails support accountability, but logging infrastructure is a separate concern.*

82. **Are there regulatory or compliance considerations for auto-fixing system state (approval workflows, change freezes, data retention)?** [Sonnet]
    *Why it matters: Certain environments may have constraints on automated actions, but these are enterprise/compliance concerns beyond the feature's core design.*

### Advanced UX

83. **Should the list view show currently-failing checks vs. all available checks?** [Sonnet]
    *Why it matters: Highlighting problem areas saves time, but requires caching state from previous runs.*

84. **Should there be visual indicators for checks requiring additional flags (`--rig`, `--slow`) in the listing?** [Sonnet]
    *Why it matters: Users need to know upfront if a check requires additional context, but this is a listing enhancement.*

85. **Should running `gt doctor list` show copy-pasteable commands for each check?** [Opus]
    *Why it matters: Copy-pasteable commands make the transition from listing to execution frictionless, but this is a polish concern.*

86. **Should the feature support JSON or structured output for the check listing?** [Sonnet]
    *Why it matters: Programmatic consumption of the check listing enables tooling integration, but is an extension beyond core selectivity.*

87. **Should different development teams or roles see different default check sets?** [Sonnet]
    *Why it matters: Role-based or profile-based configurations are a deeper need than basic selectivity, suitable for a future iteration.*

88. **Should the feature handle localization or internationalization of check names and messages?** [Sonnet]
    *Why it matters: Even if not immediately needed, considering i18n architecture prevents future rewrites. But this is a long-term concern.*

89. **Would users expect `gt doctor orphaned-sessions` to work like a deeper "drill-down" into that check, showing more detail than the full run?** [Opus, Haiku, Sonnet]
    *Why it matters: Users from monitoring dashboards expect drill-down to mean "more detail on this one thing." If single-check output is identical to the full-run view of the same check, users may question the value.*
