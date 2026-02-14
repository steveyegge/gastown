# Opus 4.6 Analysis: doctor-selectivity

## User Advocate Perspective

# User Advocate Analysis: `gt doctor <specific-doctor-fix>`

## User Expectations

1. **What does the user expect `gt doctor orphaned-sessions` to do -- run the check, or run the fix?**
   The command name "doctor" already implies healing; users may expect it to both diagnose AND fix in one step, not just report.

2. **Do users expect `gt doctor orphaned-sessions` to behave identically to `gt doctor --fix` but scoped, or to just run a single check without fixing?**
   If the behavior differs from their mental model, they will run the wrong command and either miss a fix or trigger one unexpectedly.

3. **Would users expect partial name matching (e.g., `gt doctor orphan` matching `orphan-sessions` and `orphan-processes`)?**
   Users of tools like `kubectl`, `docker`, and `git` are accustomed to prefix/substring matching and will be frustrated if they must type exact names.

4. **Do users expect `gt doctor <name>` to also run prerequisite or dependent checks?**
   Some checks may only make sense after others pass; running one in isolation could produce misleading results.

5. **Would users expect glob or wildcard support (e.g., `gt doctor 'orphan-*'` or `gt doctor stale-*`)?**
   Power users pattern-match by reflex; if this silently matches nothing, they will lose trust in the tool.

6. **Do users expect they can pass multiple check names (e.g., `gt doctor orphan-sessions wisp-gc`)?**
   Running two targeted checks in one invocation is a natural expectation; forcing separate invocations feels needlessly restrictive.

7. **Would users expect `gt doctor cleanup` (a category name) to run all checks in that category?**
   The context doc suggests this as a possible flow, but if categories and check names live in the same argument namespace, ambiguity will confuse users.

8. **Do users expect the same streaming, categorized output format when running a single check?**
   The full doctor output with headers, separators, and summary may feel excessive for a single targeted check.

9. **Would users coming from `brew doctor`, `npm doctor`, or `flutter doctor` expect the same UX conventions?**
   Users bring habits from other CLI "doctor" commands; deviating without reason will create friction.

10. **Do users expect the `--fix` flag to still be required, or do they assume specifying a fixable check implies "fix it"?**
    This is the single biggest UX ambiguity: does naming a fix-capable check auto-trigger the fix, or is `--fix` still needed?

11. **Would users expect `gt doctor list` to be a subcommand or do they expect `gt doctor --list`?**
    Mixing positional arguments (check names) with subcommands (list) in the same argument slot creates parsing ambiguity and user confusion.

12. **Do users expect `gt doctor` (no args) to continue working exactly as before?**
    Any regression in the zero-argument default behavior will break existing workflows and muscle memory.

13. **Would users expect tab-completion for check names?**
    Without shell completion, users must memorize or look up exact check names, which defeats the convenience of selective execution.

14. **Do users expect that running a single check is faster than running all checks?**
    If the targeted run still initializes the full doctor framework and takes the same time, the feature feels pointless.

## User Journey

1. **What triggers a user to run a specific check instead of the full doctor?**
   Understanding the trigger (e.g., a known issue, a CI failure, a colleague's suggestion) shapes whether the feature should optimize for speed, clarity, or automation.

2. **Is the user typically in a rush when they want to run a single fix?**
   If they already know what is broken, they want the fastest path to resolution -- extra output, confirmations, or ceremony will feel like friction.

3. **How does the user discover which check names are available?**
   If there is no discoverability mechanism (help text, `list` command, error suggestions), users will guess wrong and give up.

4. **What happens right before the user runs `gt doctor orphaned-sessions`?**
   They likely just saw an error or warning from another `gt` command; the targeted doctor run should feel like a natural next step from that context.

5. **What does the user do immediately after running a targeted check?**
   They probably re-run whatever command failed before; the doctor output should give them confidence that the issue is resolved.

6. **Does the user expect a confirmation prompt before a fix is applied, or immediate action?**
   A prompt interrupts scripting and automation; no prompt risks unintended side effects. The stakes depend on the specific fix.

7. **Will users try to run targeted checks in CI/CD pipelines or scripts?**
   If so, the command needs stable, parseable output and predictable exit codes -- not just pretty terminal output.

8. **How does the user feel when `gt doctor` reports 47 checks but they only care about one?**
   This is likely the core motivation for the feature: information overload. The targeted mode should feel like relief, not a different kind of complexity.

9. **What if the user runs a targeted check and it passes -- do they trust the result?**
   A bare "check passed" without context may not reassure them; they may need to see what was checked and why it passed.

10. **Will users run targeted checks repeatedly as a "verify fix" loop?**
    If so, the output should be optimized for rapid re-execution: minimal, fast, and with clear pass/fail signals.

11. **Does the user know the difference between a check name and a category name?**
    If both are valid arguments, the user needs clear feedback about what scope they just selected.

12. **What if the user was told to run `gt doctor orphaned-sessions` by a teammate or docs, but the check name has changed?**
    Stale instructions are inevitable; the error message for an unrecognized check name must be helpful, not cryptic.

## Edge Cases (User Behavior)

1. **What if the user types `gt doctor orphan_sessions` (underscores instead of hyphens)?**
   Kebab-case vs snake_case mistakes are extremely common; the tool should either accept both or suggest the correction.

2. **What if the user types `gt doctor OrphanSessions` (PascalCase struct name instead of kebab-case CLI name)?**
   Developers reading source code may use the struct name; the tool should handle this gracefully.

3. **What if the user types a check name that is close but misspelled (e.g., `gt doctor orphen-sessions`)?**
   A "did you mean?" suggestion prevents a dead-end experience and is standard in modern CLIs.

4. **What if the user passes `--rig` with a non-rig-specific check name?**
   Flag combinations that make no sense should produce clear guidance, not silent misbehavior.

5. **What if the user runs `gt doctor --fix stale-binary` on a check that is not fixable?**
   The user needs to understand why the fix did not happen -- is it because the check passed, or because it cannot be auto-fixed?

6. **What if the user runs `gt doctor orphaned-sessions` from outside a Gas Town workspace?**
   Some checks require workspace context; running them outside one should fail clearly, not with a stack trace.

7. **What if the user passes an empty string or whitespace as the check name?**
   Edge-case input should fall back to default behavior or produce a clear error, not panic.

8. **What if the user passes `--` followed by a check name (e.g., `gt doctor -- orphaned-sessions`)?**
   Cobra's argument parsing may behave unexpectedly; this should be tested.

9. **What if the user tries to run a check that was removed in a newer version of `gt`?**
   Version skew between docs/memory and the installed binary is common; the error should mention the check was removed if possible.

10. **What if the user tries to Ctrl+C during a targeted fix mid-execution?**
    Interrupted fixes could leave the system in a worse state than before; the fix should be atomic or clearly warn about partial application.

11. **What if the user pipes the output (e.g., `gt doctor orphaned-sessions | grep FAIL`)?**
    The streaming output with ANSI codes and icon updates may produce garbled pipe output; the tool should detect non-TTY and simplify.

12. **What if the user runs two targeted doctor commands simultaneously in different terminals?**
    Concurrent fixes on the same resource (e.g., orphaned sessions) could conflict; is there any locking?

13. **What if the user runs `gt doctor all` expecting it to mean "run everything"?**
    If `all` is not a recognized keyword, it will be treated as a check name and fail confusingly.

14. **What if the user runs `gt doctor --fix` with no check name -- does behavior change at all?**
    Adding a new feature should not alter the semantics of existing flag-only invocations.

15. **What if the user expects `gt doctor orphaned-sessions --verbose` to show more detail about just that check?**
    Verbose mode scoped to a single check should feel more useful, not just the same wall of text.

16. **What if the user runs a category name that collides with a check name?**
    If a check is named `cleanup` and a category is also `cleanup`, which takes priority? Ambiguity must be resolved explicitly.

## Accessibility & Inclusion

1. **Who is excluded if check names are only discoverable by reading source code?**
   Users without Go expertise or source access need a runtime discovery mechanism (help text, list command, or error suggestions).

2. **Are the check names meaningful to non-native English speakers?**
   Terms like "orphan," "wisp," "stale beads redirect" use metaphorical English that may be opaque to international users.

3. **Do the check names make sense without understanding Gas Town's internal terminology?**
   "wisp-gc" or "stale-beads-redirect" require domain knowledge; users new to Gas Town will not know which check to run.

4. **Are screen reader users able to parse the streaming output with icons and ANSI formatting?**
   The icons (checkmarks, wrenches, hourglasses) need text equivalents for accessibility tools.

5. **Is the feature usable over slow SSH connections where streaming output may lag?**
   Real-time character-by-character updates can be painful over high-latency connections; a simpler output mode would help.

6. **What about users with color blindness -- are check statuses distinguishable without color?**
   If pass/fail relies solely on green/red coloring, colorblind users cannot distinguish results; icons help but must be sufficient alone.

7. **Are users with cognitive disabilities or attention difficulties served by 47 check names they need to remember?**
   Grouping, aliases, or fuzzy matching reduces the cognitive load of selecting the right check.

8. **What about users who primarily work in non-terminal environments (e.g., VS Code integrated terminal)?**
   Some terminal emulators render ANSI codes or Unicode icons differently; the output must degrade gracefully.

9. **Is there any documentation or `--help` text that explains what each check does before running it?**
   Users should not have to run a check to find out what it does; a dry-run or description mode is essential for informed choice.

10. **What about users who are new to the team and do not know the project jargon?**
    Check descriptions should use plain language, not just internal codenames, so newcomers can self-serve.

11. **Are users on older terminal emulators or Windows Terminal penalized by Unicode icon choices?**
    Emoji rendering varies across platforms; fallback characters should be available.

12. **What assumptions are we making about the user's familiarity with CLI conventions?**
    Not every user knows that `--fix` is a flag, that positional arguments exist, or that `--` separates flags from arguments; help text must be clear.

13. **Is the error output for invalid check names localized or at least written in simple, translatable English?**
    Complex error messages with idioms or jargon create barriers for non-native speakers.

14. **What about users who rely on aliases or wrapper scripts around `gt doctor`?**
    Adding positional arguments may break existing aliases that assume `gt doctor` takes no positional args.

---

## Product Designer Perspective

# Product Designer Analysis: `gt doctor <specific-doctor-fix>`

## Feature Brief
Add support for selective doctor checks, e.g. `gt doctor orphaned-sessions`, so users can run or fix individual checks rather than the full suite.

---

## Information Architecture

1. **What is the minimum information a user needs to discover which checks exist?**
   Users cannot target a specific check if they do not know the names; discoverability is the entry point to this entire feature.

2. **Should the list of available checks show descriptions, categories, or just names?**
   A name alone may be meaningless ("wisp-gc") without a short explanation, but too much detail clutters the listing.

3. **How should checks be organized in a listing: flat alphabetical, grouped by category, or grouped by fixability?**
   The grouping determines how quickly a user can scan to find what they need; the current full-doctor output already uses categories.

4. **Should the user see whether a check is fixable directly in the listing?**
   If a user's goal is to fix something, they need to quickly distinguish fixable from informational checks without trial and error.

5. **When running a single check, should the output show the category header or skip it?**
   Category headers add context but feel heavy when you are running exactly one thing; the visual weight should match the scope.

6. **Should the single-check output include the summary line ("1 passed, 0 warnings...")?**
   A summary of one item may feel redundant, but omitting it breaks the mental model users have from full runs.

7. **When a check has verbose details, should single-check mode auto-expand them instead of requiring `--verbose`?**
   Running a targeted check implies the user wants depth; hiding details behind a flag may feel like unnecessary friction.

8. **Should the fix hint always be shown for a targeted check, even when the check passes?**
   Showing the hint proactively teaches users what the fix does, but it could also create confusion ("Is something wrong?").

9. **How should elapsed time be displayed for a single check versus a batch?**
   Timing information is useful in a full suite to spot slow checks; for a single check it may be noise or it may be the whole point.

10. **Should the output distinguish between "this check was selected" and "this check was skipped" when running a category filter?**
    When a user runs `gt doctor cleanup`, they need to understand which checks that included and which it did not, to build trust.

11. **If a targeted check depends on state from another check, should the user see that dependency or just the result?**
    Hidden dependencies can make results confusing; surfacing them adds cognitive load but builds understanding.

12. **Should there be a way to see the last result of a check without re-running it?**
    Cached results would let users verify a fix worked without waiting, but stale data could be misleading.

13. **What information hierarchy should the "not found" error follow: error message, suggestion, or full listing?**
    The balance between being helpful (showing alternatives) and being noisy (dumping the whole list) affects the recovery experience.

---

## Interaction Design

1. **Should the check name be a positional argument (`gt doctor orphaned-sessions`) or a flag (`gt doctor --check orphaned-sessions`)?**
   Positional arguments feel more natural for targeting, but flags are more explicit and composable with existing flags.

2. **Can the user specify multiple checks in one invocation (e.g., `gt doctor orphan-sessions wisp-gc`)?**
   Batch targeting lets power users compose their own mini-suites, but it complicates argument parsing and output formatting.

3. **Should `--fix` be required when targeting a fixable check, or should targeting imply the intent to fix?**
   Users running `gt doctor orphan-sessions` likely want the fix, but auto-fixing without explicit consent violates fail-safe principles.

4. **How should the user trigger category-level filtering (`gt doctor cleanup` vs `gt doctor --category cleanup`)?**
   If category names and check names share the same positional argument space, collisions or ambiguity can confuse users.

5. **What feedback should the user see immediately after invoking a targeted check (before results)?**
   Instant feedback ("Running orphan-sessions check...") confirms the command was understood; silence causes uncertainty.

6. **How should success be communicated for a single targeted fix?**
   The current full-doctor uses icons and a summary; a targeted fix might deserve a more prominent, direct confirmation message.

7. **How should failure be communicated when a targeted check finds a problem but `--fix` was not passed?**
   The user needs to clearly understand: the problem exists, it is fixable, and here is the exact command to fix it.

8. **What happens when the user passes `--fix` with a check that is not fixable?**
   This is a predictable user error; the response should educate rather than just error out.

9. **Should tab-completion be supported for check names?**
   Tab-completion dramatically improves discoverability and reduces typos, but it requires shell integration work.

10. **Should fuzzy matching be supported for check names (e.g., `gt doctor orphan` matching `orphan-sessions` and `orphan-processes`)?**
    Fuzzy matching reduces the need to remember exact names but can lead to unintended matches; the tradeoff is convenience vs precision.

11. **What should happen if the user types `gt doctor --fix` with no check specified (current behavior: fix all)?**
    Backward compatibility matters; existing users expect `gt doctor --fix` to fix everything, so adding arguments must not break that flow.

12. **Should there be a confirmation prompt before fixing when a check is targeted, or is the explicit targeting sufficient consent?**
    Targeted invocation with `--fix` is arguably double-consent, but destructive fixes might still warrant a prompt.

13. **How should the `--verbose` flag interact with targeted checks?**
    If single-check mode auto-expands details, `--verbose` might become redundant or could unlock an even deeper layer of output.

14. **Should `gt doctor list` be its own subcommand or a flag (`gt doctor --list`)?**
    Subcommands feel like first-class features; flags feel like modifiers -- the choice signals how important discoverability is.

15. **How should the user learn about this feature if they have muscle memory for `gt doctor` with no arguments?**
    If the feature is invisible in the default output, adoption will be low; a subtle hint in the summary could drive discovery.

---

## User Flows

1. **Happy path: User knows the exact check name and runs it -- what does every step look like from invocation to final output?**
   Mapping the full sequence ensures no step feels jarring or missing compared to the full-doctor experience.

2. **Happy path with fix: User runs `gt doctor --fix orphan-sessions` and the fix succeeds -- what confirmation do they see?**
   The "fixed" state needs to feel conclusive so the user does not wonder whether they need to do anything else.

3. **Discovery flow: User does not know any check names -- how do they find the right one?**
   If discovery requires reading docs outside the CLI, the feature fails the "self-service" test.

4. **Typo recovery: User types `gt doctor orphan-sesions` (misspelled) -- what happens?**
   Typos are the most common error; the recovery path determines whether users retry or give up.

5. **Category flow: User wants to run all cleanup checks -- what does the invocation and output look like?**
   Category filtering is a middle ground between "one check" and "all checks" and needs its own clear UX.

6. **No-issue flow: User targets a check and it passes -- is the output satisfying or anticlimactic?**
   A passing check should still feel like useful information ("everything is fine") rather than "nothing happened."

7. **Partial failure flow: User targets multiple checks and some pass while others fail -- how is this presented?**
   Mixed results need clear visual hierarchy so failures are not buried among passes.

8. **Fix-not-available flow: User targets a non-fixable check with `--fix` -- how do they learn what they can do instead?**
   Dead ends are frustrating; the error should redirect users to a productive next action.

9. **Re-run after fix flow: User fixes a check, then re-runs it to verify -- does the experience feel natural?**
   Verify-after-fix is a universal instinct; the flow should support it without extra cognitive load.

10. **Edge case: User targets a rig-specific check without the `--rig` flag -- what happens?**
    Rig checks require context; the error message needs to explain both what went wrong and how to fix the invocation.

11. **Edge case: User targets a check that requires a slow operation but did not pass `--slow` -- what feedback do they get?**
    Slow checks are gated for a reason; the user needs to understand the tradeoff and opt in consciously.

12. **Empty state: All targeted checks pass on a healthy system -- does the output feel useful or like wasted effort?**
    Healthy systems are the common case; the output should still feel rewarding rather than hollow.

13. **Overwhelm case: User runs `gt doctor list` and sees 47 checks -- how do they navigate that?**
    A flat list of 47 items is hard to scan; grouping, filtering, or pagination may be needed.

14. **First-time user flow: Someone who has never used `gt doctor` encounters selective mode -- is it self-explanatory?**
    New users have no mental model of the check system; the feature should not assume prior knowledge.

---

## Visual & Layout

1. **Where does the check listing (`gt doctor list`) appear: inline in the terminal or piped to a pager?**
   47 checks may overflow a terminal screen; a pager preserves context but adds interaction overhead.

2. **Should the listing use columns (name + description side by side) or stacked rows (name on one line, description below)?**
   Column layout is more scannable but can truncate in narrow terminals; stacked is safer but takes more vertical space.

3. **Should category headers in the listing use the same visual treatment as in the full-doctor output?**
   Visual consistency helps users connect the listing to the full-doctor experience they already know.

4. **Should fixable checks be visually distinguished in the listing (e.g., with an icon or label)?**
   If the primary use case for targeting is fixing, making fixable checks visually pop saves scanning time.

5. **When running a single check, should the output use the same streaming pattern (icon + name, then update) or a simpler format?**
   Streaming makes sense when many checks run in sequence; for a single check, it might flash too quickly to register.

6. **Should the single-check output be more detailed than the same check's output in a full run?**
   Targeting implies wanting depth; a richer single-check view could include details that are hidden in the full suite.

7. **Should the "did you mean?" suggestion on typos use the same styling as error messages or a distinct, softer style?**
   Suggestions are helpful, not alarming; they should feel like guidance rather than error output.

8. **How should category-level runs visually differ from single-check runs and full runs?**
   Three scopes (single, category, full) need distinct but related visual treatments so users always know what scope they are in.

9. **Should the single-check output include a visual indicator of which category the check belongs to?**
   Category context helps users understand where this check fits in the larger health picture.

10. **Should the help text for `gt doctor` be updated to show the selective syntax prominently or as a footnote?**
    Placement in help text determines whether users discover the feature or overlook it.

11. **What should the terminal output look like when no arguments are passed -- should it change at all to hint at selectivity?**
    A subtle hint ("Tip: run `gt doctor <check-name>` to target a specific check") could drive adoption without cluttering the output.

12. **Should color coding differ for targeted runs to signal "focused mode"?**
    A visual cue that says "you are in targeted mode" prevents confusion about whether all checks ran or just one.

---

## States & Transitions

1. **What are the distinct states for a targeted check: loading, running, passed, warned, failed, fixed, error?**
   Enumerating all states ensures the UI accounts for each one rather than discovering gaps at runtime.

2. **How does the user move from "check failed" to "check fixed" -- is it a separate invocation or can they confirm inline?**
   Inline fixing (e.g., "Fix now? [y/N]") reduces friction but adds interactive complexity to a CLI tool.

3. **What is the transition from "invalid check name" to "running the correct check"?**
   If the error message includes the correct command, the user can copy-paste and retry; if not, they have to figure it out.

4. **What does the "loading" state look like for a targeted check -- spinner, static text, or nothing?**
   Short checks may finish before a spinner renders; the loading indicator needs to handle sub-second durations gracefully.

5. **Should there be a distinct "skipped" state for checks that are not applicable in the current context?**
   A check that cannot run (e.g., rig check without `--rig`) is different from a check that failed; the distinction matters.

6. **How does the user transition from a `gt doctor list` view to actually running a check?**
   If the listing shows copy-pasteable commands, the transition is frictionless; if not, the user has to construct the command manually.

7. **What happens if a fix partially succeeds (e.g., kills 3 of 5 orphaned sessions)?**
   Partial success is ambiguous; the user needs to know what was fixed, what remains, and whether to retry.

8. **What is the state after a successful fix -- does the check automatically re-run to verify, or does it just report "fixed"?**
   Auto-verification builds confidence; skipping it leaves doubt about whether the fix actually worked.

9. **How does the system transition when a user runs `gt doctor` (full) right after a targeted fix?**
   The full run should reflect the fix; if it shows the same issue as still broken, trust in the tool erodes.

10. **What is the state when a targeted check is running but takes longer than expected?**
    Long-running targeted checks need a progress indicator or timeout behavior so the user does not think the tool is hung.

11. **Should there be a "dry-run" state where the user can preview what a fix would do without applying it?**
    Dry-run reduces anxiety about destructive fixes and builds trust, especially for users new to the tool.

12. **How does the user transition from "I don't know what's wrong" to "I'm running the right targeted check"?**
    This is the core UX challenge: bridging the gap between a vague problem and a specific check requires guided discovery.

13. **What state is shown if the user targets a check that was recently added but their binary is outdated?**
    Version skew between known check names and the installed binary can cause confusing "not found" errors that are not the user's fault.

14. **Can a targeted check be interrupted (Ctrl+C) cleanly, and what state does it leave the system in?**
    Interrupted fixes could leave partial state; the user needs to know whether it is safe to retry or if manual cleanup is needed.

---

## Domain Expert Perspective

# Domain Expert Analysis: `gt doctor <specific-doctor-fix>`

## Domain Concepts

### What terminology is assumed but not defined?

1. **What exactly is a "fix" versus a "check" from the user's mental model?**
   Users may assume running `gt doctor orphan-sessions` means "fix it" rather than "diagnose it," and conflating these creates confusion about whether `--fix` is still required.

2. **What does "selective" mean in practice -- a single check, a category, or a pattern?**
   The brief says `<specific-doctor-fix>` but uses a check name as the example; whether this extends to categories or globs determines the entire UX surface.

3. **Is "doctor fix" the right metaphor, or is this more like "doctor check"?**
   Calling the positional argument a "specific-doctor-fix" implies the action is fixing, but the current system separates diagnosis from repair, and naming matters for user expectations.

4. **What is the relationship between a check "name" and a check "ID"?**
   The system uses kebab-case names like `orphan-sessions`, but the brief uses `orphaned-sessions` (note the 'd'); if these diverge, users will get confused by near-miss typos.

5. **What does "running a check" mean when the check depends on prior checks?**
   Some checks (like workspace-level fundamentals) may gate later checks; running a check in isolation may produce misleading results if its preconditions are not validated.

6. **What is a "category" in user-facing terms?**
   Categories like `CategoryCleanup` or `CategoryCore` are internal constants; users have no documented vocabulary for referring to groups of checks.

7. **What does "fixable" mean from the user's perspective?**
   Some checks are fixable but only under certain conditions (e.g., `orphan-sessions` skips crew-protected sessions); users may expect "fix" to mean "fully resolve" rather than "attempt conservative repair."

8. **What is the distinction between "rig checks" and "workspace checks" in the user's world?**
   Users may not realize that some checks require `--rig` and that running `gt doctor <check-name>` for a rig check without `--rig` would either fail or behave differently.

9. **What does "doctor" mean when there is nothing wrong?**
   If a user runs `gt doctor orphan-sessions` and everything is clean, should the output be a single "OK" line, a full summary, or silence? The meaning of "nothing to report" differs by context.

10. **What is the user's expectation around "safety" when selectively fixing?**
    The full doctor run has a deliberate check ordering (fundamentals first, cleanup later); selective runs bypass that safety net, and users may not know they are assuming more risk.

11. **Does "doctor" imply diagnosis, treatment, or both?**
    In medical metaphor, a doctor both examines and treats; the existing CLI separates these with `--fix`, but selective invocation blurs the line and the brief's title ("specific-doctor-fix") leans toward treatment.

12. **What does "stale" mean across different check contexts?**
    Stale beads, stale binaries, stale sessions, stale wisps -- each uses "stale" with different timeout semantics, and users might expect a uniform meaning.


## Prior Art

### What do existing products do?

1. **How does `brew doctor` handle selective checks, and what can we learn from its UX?**
   Homebrew's `brew doctor` runs all checks with no selectivity; understanding why they chose not to allow filtering (simplicity, safety) is relevant to our decision.

2. **How does `rustup check` or `cargo clippy` allow filtering to specific lints?**
   Rust's toolchain lets users allow/deny individual lints by name; this is a proven pattern for selective diagnostics and may set user expectations for CLI tool doctors.

3. **How does `npm doctor` or `yarn doctor` report on individual subsystems?**
   npm doctor runs a fixed set of checks with no filtering; if we diverge from this convention, we need a good reason.

4. **What does `git fsck` do when you want to check only specific object types?**
   Git's fsck has limited selectivity; understanding why (integrity checks are holistic) informs whether selective health checks make domain sense.

5. **How do Kubernetes liveness/readiness probes handle partial health?**
   K8s separates "alive" from "ready" as distinct selective health signals; this two-tier model may inform how we think about check subsets.

6. **What conventions do CLI tools follow for subcommand-vs-argument for filtering?**
   `docker system prune --filter` uses flags, `systemctl status <unit>` uses positional args; users have expectations about which pattern applies and the brief needs to match them.

7. **How does `pre-commit run <hook-id>` handle selective execution, and is this a closer analogy?**
   pre-commit allows running a single hook by ID, which is structurally very similar to selective doctor checks; its UX patterns (listing hooks, error on unknown ID) are directly applicable.

8. **What happens in other tools when a user asks to run a check that has dependencies?**
   Tools like Ansible's `--tags` or Make's target selection handle dependency chains; if our checks have implicit ordering, how do other tools communicate that?

9. **How do monitoring dashboards (Datadog, Grafana) handle "drill down into one check"?**
   Users accustomed to monitoring tools expect drill-down to mean "more detail on this one thing," not "only run this one thing"; the expectation gap matters.

10. **What do systemd's `systemctl is-active` and `systemctl status` teach about single-service queries?**
    systemd allows querying a single unit and returns a clear pass/fail exit code; this is the closest analogy to `gt doctor <check>` and users in the Linux ecosystem will expect similar semantics.

11. **How does ESLint handle `--rule` for running a single rule, and what UX patterns emerged?**
    ESLint allows enabling/disabling individual rules, and the community found that running a single rule is most useful for debugging false positives, not for routine use; this may inform how often selective doctor runs will actually be used.

12. **How does `flutter doctor -v` handle showing individual check details?**
    Flutter's doctor groups checks by subsystem and `-v` gives details; it does not allow running a single check, suggesting the community found holistic checks more valuable.


## Problem Depth

### Is this the real problem or a symptom?

1. **Is the real problem that `gt doctor` is too slow, and selectivity is a workaround for speed?**
   If the full doctor run took 2 seconds, nobody would ask for selectivity; understanding whether this is a performance problem disguised as a UX request changes the solution.

2. **Is the real problem that users want to run a fix without the diagnostic noise?**
   If users already know what is wrong and just want `gt fix orphan-sessions`, the doctor framing may be the wrong abstraction entirely.

3. **Is the real problem that users want scriptable, single-purpose health checks?**
   CI/CD pipelines and automation need individual exit codes; if the driver is machine consumption rather than human UX, the design changes significantly.

4. **Are users asking for selectivity because they do not trust the full doctor run?**
   If some checks are flaky or produce false positives, users may be avoiding the full run; fixing reliability might eliminate the need for selectivity.

5. **Is this a discoverability problem disguised as a selectivity problem?**
   Users may not know which checks exist or what they do; if `gt doctor list` is the real need, the positional argument for running checks is secondary.

6. **Will users expect `gt doctor <name>` to also imply `--fix`?**
   The brief title says "specific-doctor-fix" (emphasis on fix), suggesting the real ask may be "let me fix one thing quickly" rather than "let me diagnose one thing."

7. **Is the problem that cleanup checks should not be part of doctor at all?**
   Fixable cleanup operations (wisp-gc, orphan-sessions) behave more like maintenance commands than health checks; maybe the real problem is architectural (doctor is doing two jobs).

8. **What related problems will users expect us to solve once we ship this?**
   Selective runs naturally lead to requests for: check composition (`gt doctor cleanup`), scheduled runs (`gt doctor --cron orphan-sessions`), and check-specific configuration; we need to know the roadmap.

9. **Will users expect `gt doctor <name> --fix` to skip the diagnostic step and just fix?**
   Running the check first and then fixing is the current pattern, but for a targeted invocation users may want the fix to be immediate; the two-step pattern may feel redundant.

10. **Is the problem that doctor output is overwhelming and selectivity is a filtering mechanism?**
    If the real issue is signal-to-noise in a 47-check run, better output formatting (collapsing passing checks, showing only failures) might solve the problem without selectivity.

11. **Are there checks that are dangerous to run in isolation because they lack context from other checks?**
    If running `orphan-sessions` alone could kill a session that another check would have identified as in-use, the isolation itself is the problem we need to solve.

12. **Does this feature enable or conflict with future plans for automated/scheduled doctor runs?**
    If the plan is to have the daemon run doctor checks periodically, selectivity becomes an infrastructure concern (which checks to schedule) rather than a CLI convenience.


## Edge Cases (Domain)

### What unusual but valid scenarios exist?

1. **What happens when a user specifies a check name that matches multiple checks partially?**
   For example, `gt doctor orphan` could match `orphan-sessions`, `orphan-processes`, and `orphaned-attachments`; the disambiguation strategy matters.

2. **What happens when a user specifies a rig check without `--rig`?**
   Rig checks require a rig context; running `gt doctor beads-config-valid` without `--rig` should either error clearly or auto-detect the rig, but both choices have consequences.

3. **What happens when a user requests a check that is registered conditionally?**
   Rig checks are only registered when `--rig` is provided; if the user names one without the flag, the check literally does not exist in the registered set, which could produce confusing "unknown check" errors.

4. **What if a check's behavior changes depending on what other checks ran before it?**
   If `workspace-config` populates a shared context that `routes-config` depends on, running `routes-config` alone might produce a false positive or crash.

5. **What if a user specifies both a check name and a category (e.g., `gt doctor cleanup orphan-sessions`)?**
   Ambiguous inputs need a clear resolution strategy -- does the category filter further narrow the check, or is this an error?

6. **What happens when the check name does not exist but is very close to a real one?**
   Typos like `orphaned-session` (singular) vs. `orphan-sessions` (plural) or `orphaned-sessions` (brief's own example) need fuzzy matching or clear "did you mean" suggestions.

7. **What if a user runs `gt doctor --fix <check>` for a check that is not fixable?**
   The combination of selective targeting and `--fix` for a read-only check should either be a no-op, a warning, or an error; each choice signals differently.

8. **What if a user specifies multiple check names (e.g., `gt doctor orphan-sessions wisp-gc`)?**
   Multi-select via multiple positional arguments is a natural extension; if we do not support it initially, users will try it and the failure mode matters.

9. **What if a selective check takes a long time and the user expected it to be fast?**
   The full doctor run sets expectations via streaming; a single check that hangs for 30 seconds without streaming output feels broken.

10. **What if the user runs a selective check from a directory where the workspace context is ambiguous?**
    The current code uses `workspace.FindFromCwdOrError()`; if selective checks are used in scripts from arbitrary directories, the workspace detection might surprise users.

11. **What if a check's name collides with a Cobra subcommand or reserved word (like `list`, `help`, `version`)?**
    If we later add `gt doctor list` to show available checks, and a check is named `list`, the namespace collision is a real problem.

12. **What if a user expects `gt doctor <name>` to give more detail than the full run?**
    Running a single check might imply "deep dive mode" with verbose output by default; if it produces the same terse one-liner, users may feel it adds no value over the full run.

13. **What if the user pipes the output or runs in a non-TTY context?**
    Selective doctor output may be consumed by scripts; the streaming/overwrite behavior (carriage returns) produces garbled output in non-TTY contexts and selective runs are more likely to be scripted.

14. **What about checks that have side effects even during the diagnostic phase (not just the fix phase)?**
    If a check's `Run()` method writes logs, touches files, or queries external services, running it selectively might have unintended consequences the user did not anticipate.


## Success Criteria

### How would we know this succeeded?

1. **What percentage of doctor runs should become selective after this ships?**
   If 80% of runs remain full `gt doctor`, the feature has low adoption and might not have been worth the complexity; defining an adoption target helps evaluate success.

2. **Should selective runs be faster than full runs?**
   Users will measure success partly by speed improvement; if running one check is not meaningfully faster (because workspace setup dominates), the feature feels hollow.

3. **Should selective runs produce a non-zero exit code on failure, matching full runs?**
   For scripting and CI use cases, the exit code contract is the most important success criterion; if it is inconsistent with full runs, automation breaks.

4. **How do we measure whether users can discover and remember check names?**
   If users constantly mistype check names or cannot find the right one, discoverability is failing; tab-completion and `gt doctor list` become critical success factors.

5. **What does the error message look like for an unknown check name?**
   A good "did you mean X?" suggestion with the list of available checks is the difference between a frustrating and a delightful experience.

6. **Should selective runs support all the same flags as full runs (`--verbose`, `--slow`, `--rig`)?**
   Flag compatibility is a completeness criterion; if some flags do not work with selective runs, users will file bugs.

7. **How should the summary output change for a single-check run?**
   Printing "1 passed, 0 warnings, 0 failed" is technically correct but feels silly for a single check; the output format is a success criterion.

8. **Should `gt doctor list` show fixable vs. read-only status?**
   Users selecting a check to fix need to know which checks support `--fix`; if this metadata is hidden, users will waste time targeting unfixable checks.

9. **How do we measure that selective runs do not produce false results due to missing context?**
   If isolated checks produce different results than the same check in a full run, the feature is unreliable and will erode trust.

10. **Should tab completion work for check names?**
    Shell completion for check names is a strong signal of polish and usability; without it, users must consult `gt doctor list` every time.

11. **What is the acceptable time for a selective single-check run?**
    If the full run takes 10 seconds and a single check takes 8 seconds (due to workspace setup overhead), the feature is not delivering its promise; sub-second single-check runs should be the target.

12. **Should we track which checks are most commonly run selectively?**
    Telemetry on selective check usage would inform future decisions about which checks to optimize, promote to standalone commands, or deprecate from doctor.

13. **How should the feature interact with future changes to the check registry?**
    If a check is renamed or removed, users with scripts referencing the old name need clear deprecation warnings; forward compatibility is a success criterion.

14. **Does this feature reduce the support burden around doctor?**
    If users currently ask "how do I just fix orphan sessions?" in support channels, selective doctor should eliminate that question; measuring support ticket reduction is a concrete success metric.

---

## Cross-Perspective Themes (Opus)

### Theme 1: Discoverability and Check Naming
Multiple perspectives raised concerns about how users discover check names and remember them. The User Advocate worried about tab-completion and fuzzy matching. The Product Designer questioned information architecture and visualization of available checks. The Domain Expert highlighted terminology issues and naming conventions (kebab-case vs. other formats). All three converged on the fact that a `gt doctor list` command or discoverable mechanism is essential. Without it, users cannot use the feature effectively.

### Theme 2: The Ambiguity of "Fix" vs. "Check"
All three perspectives identified tension around whether `gt doctor <name>` runs a check or applies a fix. The User Advocate called this "the single biggest UX ambiguity." The Product Designer questioned whether `--fix` should still be required. The Domain Expert noted that the brief's title "specific-doctor-fix" emphasizes fixing over diagnosis. This ambiguity is fundamental to the feature's UX contract and must be resolved explicitly.

### Theme 3: Safety and Ordering of Selective Runs
The Domain Expert noted that full doctor runs have deliberate check ordering (fundamentals first, cleanup later) for safety. The User Advocate worried about checks with prerequisites. The Product Designer asked whether dependency information should be surfaced. All three recognized that selective runs may bypass safety nets, and users might not know they are assuming more risk. Either the feature must maintain safety guarantees or communicate the tradeoff clearly.

### Theme 4: Output Format and Scope Signaling
The Product Designer extensively discussed visual design and output formatting changes needed for targeted runs. The User Advocate worried about streaming output in non-TTY contexts and accessibility. The Domain Expert asked whether single-check runs should produce different output than the same check in a full run. All three converged on the need for output to visually signal the scope (single check vs. category vs. full run) so users always know what they are looking at.

### Theme 5: Edge Cases and Error Handling
All three perspectives identified a rich set of edge cases: typos in check names, wrong flags for conditional checks, partial success on fixes, interrupted operations, concurrent execution, and version skew between documentation and binary. The User Advocate provided specific user behavior scenarios. The Product Designer designed recovery flows. The Domain Expert questioned whether certain edge cases indicate deeper architectural problems. The convergence is clear: excellent error messages and suggestions (especially "did you mean?" for typos) are crucial for adoption.

