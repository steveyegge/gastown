# Haiku Analysis: doctor-selectivity

## User Advocate Perspective

# User Advocate Analysis: `gt doctor <specific-doctor-fix>`

## Feature Summary
Users want to run specific doctor fixes (e.g., `gt doctor orphaned-sessions`) instead of running all 47 checks. This should reduce wait time and let users fix targeted problems without running unnecessary checks.

---

## 1. USER EXPECTATIONS (What users assume this does)

### 1.1 The "Obvious" Assumption
When a user types `gt doctor orphaned-sessions`, what exactly happens? Do they expect it to run the check AND auto-fix, or just run the check and show results?
- *Why it matters*: Users need immediate clarity on whether their problem gets solved or just diagnosed.

### 1.2 Naming Consistency
Users see "orphaned-sessions" in docs and error messages. When they type `gt doctor orphaned-sessions`, do they get results, or does the system say "that's not a valid check name"?
- *Why it matters*: Exact command names are critical—one typo and the feature feels broken.

### 1.3 Flag Inheritance
If I type `gt doctor orphaned-sessions --fix`, do the flags work the same way as the full `gt doctor --fix` command? Or are there weird interactions?
- *Why it matters*: Users expect flags to be orthogonal to the check selection; surprise behavior breaks trust.

### 1.4 Help Discoverability
When a user types `gt doctor --help`, will they immediately understand they can pass a specific check name? Or do they need to read docs?
- *Why it matters*: Users don't always read documentation; help text is their primary guide.

### 1.5 Similarity to Other Tools
Users are familiar with `apt install package`, `brew install package`, `npm install package`. When they see `gt doctor orphaned-sessions`, do they intuitively understand this runs that specific check?
- *Why it matters*: Mental models from other tools shape expectations; alignment reduces cognitive load.

### 1.6 Auto-Complete Expectations
Will shell auto-complete suggest "orphaned-sessions" after typing `gt doctor or`? Or do users have to type the full name?
- *Why it matters*: Modern CLI tools have auto-complete; its absence feels unpolished.

### 1.7 Case Sensitivity
Can the user type `gt doctor Orphaned-Sessions`, `gt doctor ORPHANED-SESSIONS`, or `gt doctor orphaned_sessions`? Or does it have to match exactly?
- *Why it matters*: Strict matching creates friction; users expect CLI tools to be forgiving.

### 1.8 Output Difference
When running a single check, should the output be simpler or identical to the full `gt doctor` output? What's the right mental model?
- *Why it matters*: Users need to know if they're seeing the "right" output or something abbreviated/broken.

### 1.9 Check Discovery
How do users find out what checks exist? Do they need to run `gt doctor` first, scroll through 47 results, and memorize names? Or is there a `gt doctor --list` command?
- *Why it matters*: Without discoverability, users will default to `gt doctor` and never use the feature.

### 1.10 Partial Matching
If a user types `gt doctor orphaned`, does it match `orphaned-sessions` and `orphaned-attachments`? Or does it fail?
- *Why it matters*: Fuzzy matching is expected in modern tools; exact matching feels rigid.

### 1.11 Category Selection
Can a user type `gt doctor cleanup` to run all cleanup checks at once, or only individual checks?
- *Why it matters*: This is a natural user request that extends the feature's usefulness significantly.

### 1.12 Grep-Like Patterns
Can users type `gt doctor *orphan*` or `gt doctor /orphan.*/` to match multiple checks? Or is only exact matching supported?
- *Why it matters*: Power users expect pattern matching in CLIs; its absence limits utility.

---

## 2. USER JOURNEY (What users are trying to accomplish)

### 2.1 The "Quick Fix" Scenario
A user's tmux session is acting weird. They remember reading "orphaned-sessions" can fix it. They want to run ONE fix and move on. How do they do it?
- *Why it matters*: Speed is critical—waiting for 47 checks when you need one is frustrating.

### 2.2 The Rapid Iteration Loop
A user fixes one issue, realizes another problem exists, and wants to quickly run another specific check. How many commands do they need to type?
- *Why it matters*: Friction in iteration kills adoption; users should be able to chain commands quickly.

### 2.3 The "CI/CD Integration" Scenario
A user wants to add a health check to their deployment pipeline. Should they run all 47 checks or just `gt doctor wisp-gc`?
- *Why it matters*: This unlocks a new use case; blocking it limits the feature's scope.

### 2.4 The "Collaboration" Problem
A user wants to tell a colleague: "Run this command to fix the issue." If they say `gt doctor orphaned-sessions`, will it work, or do they need to say "Run `gt doctor --fix` and hope it fixes your issue"?
- *Why it matters*: Clear, specific commands are how teams communicate about fixes.

### 2.5 The "Panic" State
A user is in crisis mode (production issue, deadline). They don't have time to read docs or explore options. They just need a command that works NOW.
- *Why it matters*: Intuitive design matters most when users are stressed; complex features backfire.

### 2.6 The "Learning" Journey
A new team member doesn't know what checks exist. They want to explore and understand the system. What's the guided path?
- *Why it matters*: If exploration is hard, users stay dependent on others to explain the system.

### 2.7 The "Verification" Mindset
After running `gt doctor orphaned-sessions --fix`, the user wants to verify the fix worked. Do they run it again to confirm? Or is there explicit "fix completed" feedback?
- *Why it matters*: Users need confidence their fix actually worked; ambiguity creates doubt.

### 2.8 The "Comparison" Use Case
A user wants to know: "What happens if I run just the cleanup checks vs. the full doctor?" How do they compare?
- *Why it matters*: Decision-making requires understanding scope; hidden comparisons cause buyer's remorse.

### 2.9 The "Oops" Moment
A user runs `gt doctor orphaned-sessions --fix`, then realizes they mistyped and meant `gt doctor orphaned-attachments`. Can they undo? What's the recovery path?
- *Why it matters*: Users fear destructive operations; clarity on reversibility reduces anxiety.

### 2.10 The "Scheduled" Fix
A user wants to run `gt doctor orphaned-sessions` every day as a cron job. Does it work in non-interactive mode? Are there flags to suppress output?
- *Why it matters*: Automation unlocks new use cases; interactive-by-default breaks these patterns.

### 2.11 The "Escalation" Path
A user runs `gt doctor orphaned-sessions` and it fails. They want to see more details. Do they need a `--verbose` flag? Where do they look?
- *Why it matters*: Clear debugging paths keep users from getting stuck or giving up.

### 2.12 The "Before and After" Narrative
A user wants to understand what the check does before running it. Where do they find a description of what `orphaned-sessions` actually removes?
- *Why it matters*: Users make informed decisions; opaque operations feel risky.

---

## 3. EDGE CASES: USER BEHAVIOR (What weird things might users try)

### 3.1 Multiple Checks in One Command
A user types `gt doctor orphaned-sessions wisp-gc stale-beads-redirect`. Should this work, or is only one check supported?
- *Why it matters*: This is a natural extension; forcing users to run three separate commands feels archaic.

### 3.2 Invalid Check Names
A user types `gt doctor orphaned-sessions123`. What error message do they get? Is it helpful, or cryptic?
- *Why it matters*: Error messages either educate or frustrate; bad messages break the experience.

### 3.3 Typo Recovery
A user types `gt doctor orphaned-session` (missing 's'). Do they get a helpful "Did you mean orphaned-sessions?" suggestion?
- *Why it matters*: Without suggestions, users feel dumb; with them, users feel supported.

### 3.4 Empty Input
A user types `gt doctor ` (space but no check name) or `gt doctor ""`. Does it default to running all checks, error out, or show help?
- *Why it matters*: Consistent fallback behavior prevents confusion about what happened.

### 3.5 Whitespace Handling
A user types `gt doctor  orphaned-sessions ` (extra spaces). Does it work, or are they strict about parsing?
- *Why it matters*: Lenient parsing feels professional; strict parsing feels pedantic.

### 3.6 Mixed Case and Underscores
A user types `gt doctor orphaned_sessions` (underscore instead of hyphen) or `gt doctor Orphaned-Sessions` (capitals). Does it match?
- *Why it matters*: Users type from memory; flexibility in matching reduces frustration.

### 3.7 Combining with All Checks
A user types `gt doctor orphaned-sessions` and separately runs `gt doctor` (all). Do the two outputs look consistent, or does the single-check version feel like a different tool?
- *Why it matters*: Consistent visual language reinforces trust in the tool.

### 3.8 Exit Codes for Scripting
A user wants to use `gt doctor orphaned-sessions && echo "Success"` in a script. What exit code does it return? (0 on success/fix, 1 on failure?)
- *Why it matters*: Scripts depend on exit codes; inconsistency breaks automation.

### 3.9 Output Redirection
A user types `gt doctor orphaned-sessions > output.txt`. Is the output machine-parseable, or is it full of ANSI color codes?
- *Why it matters*: Users with scripting skills expect JSON/plain text outputs; pretty colors break parsing.

### 3.10 Combining Incompatible Flags
A user types `gt doctor orphaned-sessions --rig my-rig`. Does this make sense, or are rig-specific checks invalid for the global command?
- *Why it matters*: Unclear semantics cause feature misuse; clear docs prevent confusion.

### 3.11 Running as Non-Interactive
A user runs `gt doctor orphaned-sessions` over SSH without a terminal. Does it work, fail gracefully, or hang?
- *Why it matters*: Robust tools work in all contexts; context-dependent behavior is a trap.

### 3.12 Rapid Sequential Runs
A user runs `gt doctor orphaned-sessions --fix`, then immediately runs `gt doctor orphaned-sessions --fix` again. Does the second run find anything new, or does it report "nothing to fix"?
- *Why it matters*: Users test their fixes; idempotent operations build confidence.

### 3.13 Permission Errors
A user runs `gt doctor wisp-gc` but doesn't have permission to delete wisps. What happens? Partial success? Failure? Warning?
- *Why it matters*: Unexpected permission errors feel like bugs; clear feedback prevents confusion.

### 3.14 Interrupted Execution
A user starts `gt doctor orphaned-sessions --fix` and presses Ctrl+C halfway. Is the state consistent, or is the system in a broken state?
- *Why it matters*: Users expect graceful shutdown; crash-on-interrupt feels broken.

---

## 4. ACCESSIBILITY & INCLUSION (Who might struggle with this)

### 4.1 Users with Vision Impairments
The output currently uses icons (✓, ⚠, ✗, 🔧, ⏳). Are these accessible to screen readers, or should there be text labels?
- *Why it matters*: Icons alone exclude users; text+icons include everyone.

### 4.2 Users with Non-English Systems
Docs mention "orphaned-sessions" in English. Are check names translatable, or are they always English?
- *Why it matters*: Non-English speakers shouldn't need to memorize English names.

### 4.3 Users with Slow Connections
The output streams real-time. What if a user is on a slow SSH connection? Does the output get garbled or truncated?
- *Why it matters*: Tools should degrade gracefully; buffering helps slow connections.

### 4.4 Users Unfamiliar with CLI
A user from a GUI-only background has no idea how to run commands. Is there any on-ramp, or are they immediately stuck?
- *Why it matters*: Assuming CLI expertise excludes newcomers; tutorials help.

### 4.5 Users in Different Time Zones
Do any fixes (e.g., cleanup) use timestamps? If so, do they respect the user's local time zone?
- *Why it matters*: Time-based operations cause bugs in multi-TZ teams; clarity is critical.

### 4.6 Users with Accessibility Tools
If a user relies on a keyboard-only workflow (no mouse), is the CLI fully navigable? Or are there TUI elements that require a mouse?
- *Why it matters*: Keyboard-only workflows should be fully supported; mouse-only features exclude users.

### 4.7 Users with Dyslexia
Check names like `orphaned-sessions` vs. `orphaned-attachments` are similar. Are they easy to distinguish, or should there be better naming?
- *Why it matters*: Dyslexic users might transpose letters; clear, distinct names help.

### 4.8 Users with Memory Impairments
Should the system auto-suggest or remember recently run checks? Or is the user expected to memorize all names?
- *Why it matters*: Memory-dependent design excludes users; suggestion systems help everyone.

### 4.9 Users with Learning Disabilities
The help text needs to be clear and scannable. Is it, or is it dense walls of text?
- *Why it matters*: Clear formatting helps everyone; dense text excludes some.

### 4.10 Non-Technical Operators
Some users just need to run fixes without understanding what they do. Is the output approachable, or overwhelming?
- *Why it matters*: Operators should be able to follow instructions without being engineers.

### 4.11 Users in High-Stress Environments
In a crisis, users might panic and miss information. Is the output calm and clear, or does it add anxiety?
- *Why it matters*: Clear design reduces user errors during emergencies.

### 4.12 Neurodivergent Users
Some users process information better with consistent, predictable formats. Is the output predictable, or does it vary?
- *Why it matters*: Consistency helps neurodivergent users; variability adds cognitive load.

---

## 5. MENTAL MODELS & EXPECTATIONS (How users think about this feature)

### 5.1 "Task Launcher" Model
Users might think `gt doctor orphaned-sessions` launches a task called "orphaned-sessions" that handles everything. Should it auto-fix, or just run a diagnostic?
- *Why it matters*: Misaligned mental models create wrong expectations; clarity prevents confusion.

### 5.2 "Package Manager" Model
Users familiar with `apt install package` might expect `gt doctor orphaned-sessions` to mean "install the orphaned-sessions fix." But we mean "run the orphaned-sessions check." Are these aligned?
- *Why it matters*: Wrong analogy → wrong behavior; alignment prevents surprises.

### 5.3 "Git Subcommand" Model
Users might expect `gt doctor orphaned-sessions` to work like `git commit`, where the verb (doctor) and object (orphaned-sessions) form a complete action. Does it?
- *Why it matters*: Consistent with familiar patterns → intuitive behavior.

### 5.4 "Search" Model
Some users might expect `gt doctor orphaned` to search for all checks containing "orphaned." Does it, or do they need exact matches?
- *Why it matters*: Fuzzy matching feels smart; exact matching feels rigid.

### 5.5 "Namespace" Model
Some users might think "orphaned-sessions" is a category and expect sub-options (e.g., `gt doctor orphaned-sessions list`, `gt doctor orphaned-sessions dry-run`). Should the feature support this depth?
- *Why it matters*: Depth vs. simplicity trade-off; too much confuses; too little limits.

### 5.6 "Dry-Run" Expectations
For fixes, do users expect a `--dry-run` flag to show what would be fixed without actually fixing? Or is fix always destructive?
- *Why it matters*: Without dry-run, users hesitate to fix; with it, they feel safe.

### 5.7 "Undo" Expectations
After running `gt doctor orphaned-sessions --fix`, can users undo? Or is it permanent?
- *Why it matters*: Reversible operations feel safer; irreversible ones feel risky.

### 5.8 "Batch vs. Individual" Thinking
Does a user think of `gt doctor cleanup` (all cleanup checks) vs. `gt doctor orphaned-sessions` (one check) as fundamentally different modes?
- *Why it matters*: If both work, users feel powerful; if only one does, they feel limited.

### 5.9 "Status Check" Model
Users might run `gt doctor orphaned-sessions` expecting just status, not a fix. Does the feature align with this?
- *Why it matters*: Status checks and fixes are different operations; confusing them breaks workflows.

### 5.10 "Help-Seeking" Behavior
When stuck, users expect `gt doctor --help` or `gt doctor orphaned-sessions --help` to provide answers. Does it?
- *Why it matters*: If help is unhelpful, users give up; good help keeps them going.

---

## 6. COMMUNICATION & CLARITY (How the feature explains itself)

### 6.1 Default Behavior Documentation
The first time a user runs `gt doctor orphaned-sessions`, they need to understand: "Does this fix it, or just check?" Where's that documented?
- *Why it matters*: Assumptions → wrong commands → frustrated users.

### 6.2 List of Available Checks
Where can users see all available check names? In help? In a separate command? Buried in docs?
- *Why it matters*: Discoverability determines adoption; buried features go unused.

### 6.3 Check Descriptions
When a user sees "orphaned-sessions," what does it mean? Is there a one-liner description, or do they have to research?
- *Why it matters*: Quick descriptions build confidence; mystery creates hesitation.

### 6.4 Fixability Clarity
How does a user know which checks are auto-fixable? Are fixable checks marked differently?
- *Why it matters*: Users need to know if `--fix` will actually do anything.

### 6.5 Safety Documentation
For destructive fixes (e.g., orphaned-sessions kills tmux sessions), is the safety story clear? What's protected? What's not?
- *Why it matters*: Users fear data loss; clear safety docs reduce anxiety.

### 6.6 Error Message Quality
When a user types an invalid check name, does the error say "Invalid check" or does it suggest similar names?
- *Why it matters*: Helpful errors guide users; cryptic ones frustrate.

### 6.7 Success Feedback
After running `gt doctor orphaned-sessions --fix`, how does the user know it worked? Is there explicit "✓ Fixed X sessions" output?
- *Why it matters*: Users need confirmation; silent success feels wrong.

### 6.8 Example Usage
Are there clear examples in help text? (`gt doctor orphaned-sessions`, `gt doctor --fix orphaned-sessions`, etc.)
- *Why it matters*: Examples teach faster than prose; missing examples confuse.

### 6.9 Relationship to `gt doctor`
Is the relationship clear? Is `gt doctor orphaned-sessions` a new mode of the same command, or something different?
- *Why it matters*: Unified design feels coherent; disjointed design feels fragmented.

### 6.10 Warning Escalation
If a user tries to run a dangerous fix without `--fix` flag, is there a warning? Or does it run anyway?
- *Why it matters*: Warnings prevent accidents; missing warnings cause regrets.

---

## 7. PERFORMANCE & PERCEIVED SPEED (Time matters)

### 7.1 Startup Time
Should `gt doctor orphaned-sessions` start instantly, or is it OK if it takes a few seconds to initialize?
- *Why it matters*: Users judge tools partly on responsiveness; slow startup feels broken.

### 7.2 Single Check Duration
How long should `gt doctor orphaned-sessions` take? Should there be a timeout?
- *Why it matters*: Hung processes are frustrating; timeouts prevent user helplessness.

### 7.3 Progress Indication
For slow checks, is there a spinner or progress indicator? Or just silent waiting?
- *Why it matters*: Progress feedback prevents users from thinking the tool hung.

### 7.4 Comparison to Full Doctor
Should `gt doctor orphaned-sessions` be noticeably faster than `gt doctor`? If not, why use it?
- *Why it matters*: If there's no speed benefit, the feature's main value proposition is gone.

### 7.5 Parallelization
Should multiple check names run in parallel? (`gt doctor orphaned-sessions wisp-gc stale-beads-redirect`)
- *Why it matters*: Parallelization can provide a significant speed boost.

### 7.6 Caching
If a user runs `gt doctor orphaned-sessions` twice in a row, is the result cached? Or does it re-run?
- *Why it matters*: Caching speeds up rapid iteration; lack of caching feels inefficient.

### 7.7 Verbosity by Default
Should single-check output be minimal (just pass/fail) or detailed? What's the right balance?
- *Why it matters*: Too verbose → information overload; too minimal → missing details.

---

## 8. RELIABILITY & ERROR RECOVERY (Trust matters)

### 8.1 Idempotency
Can a user safely run `gt doctor orphaned-sessions --fix` multiple times? Or does the second run break something?
- *Why it matters*: Idempotent operations build confidence; side effects destroy it.

### 8.2 Partial Failures
If a fix partially succeeds (e.g., removes 5 of 8 orphaned sessions), what happens? Error? Partial success?
- *Why it matters*: Users need to know if they're 100% fixed or partially fixed.

### 8.3 Data Validation
Before running a fix, does the system validate the data? Or can it corrupt state?
- *Why it matters*: Validation prevents silent failures; lack of it creates paranoia.

### 8.4 Rollback Support
If a fix goes wrong, is there a rollback, or is the state permanently changed?
- *Why it matters*: Reversibility = confidence; irreversibility = fear.

### 8.5 Concurrent Execution
If a user runs `gt doctor orphaned-sessions --fix` while another user is doing the same, what happens? Race condition? Locking?
- *Why it matters*: In shared environments, race conditions cause data corruption; locking prevents it.

### 8.6 State Recovery
If the process crashes mid-fix, is the state consistent, or can it be corrupted?
- *Why it matters*: Crash safety is critical; inconsistent state is a nightmare.

### 8.7 Logging
Are all fixes logged so the user can audit what happened?
- *Why it matters*: Audit trails support accountability; lack of logging raises suspicion.

### 8.8 Dry-Run Accuracy
If a user runs `--dry-run`, is it 100% accurate to what `--fix` would do?
- *Why it matters*: Inaccurate dry-runs lead to surprises; accurate ones build confidence.

---

## 9. CONTEXT & PREREQUISITES (What's needed to succeed)

### 9.1 System State Assumptions
Does `gt doctor orphaned-sessions` assume the system is in a certain state? What if it's not?
- *Why it matters*: Unmet assumptions lead to mysterious failures.

### 9.2 Permissions
What permissions does a user need to run `gt doctor orphaned-sessions`? Do they need sudo for some checks?
- *Why it matters*: Permission errors are frustrating; clear requirements prevent them.

### 9.3 Config Files
Does the command depend on certain config files being present? What if they're missing?
- *Why it matters*: Hidden dependencies cause cryptic errors.

### 9.4 Environment Variables
Does the command depend on environment variables? Which ones?
- *Why it matters*: Undocumented dependencies are debugging nightmares.

### 9.5 Network Connectivity
Do any checks require network access? What happens if there's no network?
- *Why it matters*: Network-dependent features should say so upfront.

### 9.6 Disk Space
Do any fixes require free disk space? What happens if there's no space?
- *Why it matters*: Out-of-space errors should be anticipated, not mysterious.

### 9.7 Service Dependencies
Do any checks depend on specific services (e.g., tmux daemon)? What if they're not running?
- *Why it matters*: Missing services should produce helpful errors, not cryptic ones.

### 9.8 Version Compatibility
Does the command work across different Go versions, OS versions, tmux versions? Or only specific combinations?
- *Why it matters*: Version incompatibility should be clear upfront.

---

## 10. DISCOVERABILITY & SELF-SERVICE (Users shouldn't need to ask for help)

### 10.1 Built-in Check List
Is there a command to list all available checks? (`gt doctor --list`, `gt doctor list`, etc.)
- *Why it matters*: Without discovery, users can't use the feature without external docs.

### 10.2 Inline Help
Does `gt doctor --help` mention that you can pass a specific check name?
- *Why it matters*: Help text is the primary source of truth; if it's not there, users don't know.

### 10.3 Shell Completion
Do shell completions suggest valid check names when the user types `gt doctor or<TAB>`?
- *Why it matters*: Auto-complete is how modern users discover options.

### 10.4 README Updates
Is there a README or docs file that explains this feature? Or do users need to dig through code?
- *Why it matters*: Documented features are used; undocumented ones are ignored.

### 10.5 Changelog Entry
If this is a new feature, is it documented in a changelog so users know it exists?
- *Why it matters*: Users don't read code; they read release notes.

### 10.6 Tutorial or Walkthrough
Is there a guided example for new users? ("Try running `gt doctor orphaned-sessions` to see...")
- *Why it matters*: Tutorials reduce the activation energy for new users.

### 10.7 Error-Driven Discovery
When a user makes a mistake (e.g., invalid check name), does the error message suggest valid options?
- *Why it matters*: Good error messages can be mini-tutorials.

### 10.8 Man Page or Help System
Is there a detailed man page (`man gt-doctor`)? Or is `--help` the only reference?
- *Why it matters*: Comprehensive docs support curious users.

### 10.9 Web-Based Docs
Is there a website or wiki documenting all checks? Or only in-CLI docs?
- *Why it matters*: Some users prefer browsing docs; in-CLI docs alone aren't enough.

### 10.10 Feedback Loop
If a user has a question about the feature, where do they ask? Is there a clear support channel?
- *Why it matters*: Users get stuck; a way to get unstuck keeps them engaged.

---

## SYNTHESIS: Key Takeaways for Design

1. **Discoverability is critical**: Users won't use a feature they don't know exists. `gt doctor --list` is essential.

2. **Mental model alignment**: The feature should feel like a natural extension of `gt doctor`, not a separate command.

3. **Speed matters**: The entire value proposition is running one check faster than 47. If it's not faster, users won't care.

4. **Safety matters more**: Users fear destructive operations. Clear dry-run, undo, and safety documentation are non-negotiable.

5. **Error messages guide behavior**: When users make mistakes, the error should educate, not confuse.

6. **Consistency wins**: The single-check output should look and feel like the full-doctor output, just smaller.

7. **Automation support**: Users want to script this; non-interactive mode and exit codes matter.

8. **Accessibility is inclusion**: Icons need text labels, help text needs clarity, error messages need specificity.

9. **Flexibility helps adoption**: Fuzzy matching, partial matching, and category selection all expand the feature's reach.

10. **Trust is earned through idempotency**: If running a fix twice is safe, users feel confident.

---

## Product Designer Perspective

# Product Designer Analysis: `gt doctor <specific-doctor-fix>`

## Feature Overview
Enable users to run individual or category-filtered health checks via `gt doctor orphaned-sessions` instead of running all 47 checks each time.

---

## 1. INFORMATION ARCHITECTURE

### Questions: What information hierarchy matters most?

**Q1.1: What should the user see first when they run `gt doctor <specific-check>`?**
*Why: Confirms the user selected the right check before any execution begins. Users need to validate intent.*

**Q1.2: Should the single check output mirror the full doctor report structure (streaming → summary → details) or be simplified?**
*Why: Users running one check may need different visual hierarchy than the full 47-check report. Streaming may feel unnecessarily verbose for one item.*

**Q1.3: When a check fails, should we show the full problem diagnosis or jump straight to the fix hint?**
*Why: The information load differs greatly—a user targeting a specific fix likely wants the solution path, not full context.*

**Q1.4: Should successful single checks still display in the same success-detail format, or can we minimize success output?**
*Why: Users may be running this in CI/scripts where verbose success output wastes screen real estate. Signal failure clearly, but success could be minimal.*

**Q1.5: When running multiple checks by category (e.g., `gt doctor cleanup`), should results be grouped differently than the current category layout?**
*Why: Current layout groups by category—if user explicitly requested a category, do we need redundant grouping?*

**Q1.6: How prominent should "related checks" be if the user-specified check has dependencies or relationships?**
*Why: Users may not know that fixing orphaned-sessions should also trigger checking for orphaned-processes. Should this be suggested but not mandatory?*

**Q1.7: For checks that can be fixed, should the fix status appear inline with the check result or in a separate section?**
*Why: Scanning efficiency—if a user runs `gt doctor orphaned-sessions --fix`, they want the "Fixed: X sessions" result to be immediately visible, not buried in a summary.*

**Q1.8: Should we show checks that were NOT run? (e.g., "Skipped 46 other checks" indicator)**
*Why: Reassurance that the user didn't accidentally disable all checks. Transparency about scope is important.*

**Q1.9: In the output, what order should details appear? (Status → Message → Details → FixHint → Elapsed time)**
*Why: Cognitive flow—what does the user need first to understand the result? Reading order affects comprehension.*

**Q1.10: Should unavailable/inapplicable checks (e.g., rig-specific checks without --rig flag) be listed as "Not applicable" or silently skipped?**
*Why: Users need to know why a check didn't run. Silent skipping creates confusion about whether the feature works.*

**Q1.11: For checks with multiple details/issues (e.g., 50 orphaned sessions), should all be listed or should we paginate/truncate with a "See more" pattern?**
*Why: Very long outputs can be overwhelming in terminal. Users need reasonable defaults that scale.*

---

## 2. INTERACTION DESIGN

### Questions: How does the user trigger and control this feature?

**Q2.1: Should `gt doctor orphaned-sessions` be a subcommand style (`gt doctor run orphaned-sessions`) or direct argument style (`gt doctor orphaned-sessions`)?**
*Why: Discoverability—does the user naturally expect a positional argument or a subcommand? One feels more natural depending on mental model.*

**Q2.2: What happens if the user runs `gt doctor orphaned-sessions` but that check doesn't exist? Should we suggest similar names?**
*Why: Typos are common (orphan vs orphaned, sessions vs session). Helpful error recovery dramatically improves UX.*

**Q2.3: Can the user pass multiple check names in one command? (`gt doctor orphaned-sessions orphaned-processes`) Or one at a time?**
*Why: Power users benefit from batch operations. Limiting to one creates friction for common multi-check workflows.*

**Q2.4: Should `gt doctor list` or `gt doctor --list` be a discoverable first-class command to show available checks?**
*Why: Users can't know what checks exist without docs. A discoverable `list` command is critical for usability.*

**Q2.5: Can users filter by category interactively (e.g., `gt doctor cleanup` to run all cleanup checks)?**
*Why: Users may remember "cleanup" more easily than specific check names. Category-based filtering is a powerful UX shorthand.*

**Q2.6: Should `--fix` work on single checks the same way as it does on full doctor runs?**
*Why: Consistency—users expect the same flags to work the same way. Breaking this mental model creates confusion.*

**Q2.7: If a user runs `gt doctor orphaned-sessions --fix` and the check passes (no orphans), should we still show output or silently succeed?**
*Why: Different use cases (human vs script) need different feedback. What's the right default?*

**Q2.8: Can users combine single-check mode with other flags like `--verbose`, `--slow`, `--rig`?**
*Why: Flexibility matters. Users should feel like they're using one cohesive tool, not mode-switching.*

**Q2.9: What's the user's feedback if they pass invalid flags for a check? (e.g., `gt doctor orphaned-sessions --restart-sessions` when that check doesn't use that flag)**
*Why: Flags are powerful but error-prone. Clear messaging prevents user frustration with "why isn't this working?"*

**Q2.10: Should running `gt doctor <specific>` exit with different status codes for different outcomes? (0=pass, 1=fail, 2=fixed-something)?**
*Why: Scripts depend on exit codes. Different codes let users branch logic based on outcome.*

**Q2.11: If a user abbreviates a check name (e.g., `gt doctor orphan` for `orphaned-sessions`), should fuzzy matching help them or require exact names?**
*Why: Usability vs strictness tradeoff. Fuzzy matching is friendlier but may lead to unintended executions.*

**Q2.12: For checks with interactive decision-making (fix/don't fix), should single-check mode prompt interactively or auto-apply based on flags?**
*Why: Users in CI may have already set `--fix` but want confirmation. Others want automation. What's the right default UX?*

---

## 3. USER FLOWS

### Questions: What are the happy path, error paths, and edge cases?

**Q3.1: What's the step-by-step happy path?**
1. User runs `gt doctor orphaned-sessions`
2. System identifies the check and shows what it will do
3. Check runs and completes
4. Result is displayed with clear outcome
5. If issues found, user sees fix hint
6. If `--fix` used, changes applied and user sees what was fixed

*Why: Happy path defines baseline UX. Every step should feel natural and discoverable.*

**Q3.2: When a check is not found, what should the error recovery look like?**
- System suggests similar check names?
- Shows helpful categories?
- Links to documentation?
- Shows 3-5 closest matches (fuzzy)?

*Why: Error paths determine whether users trust the tool. Good error recovery builds confidence.*

**Q3.3: If the user asks for `gt doctor cleanup` (category), and there are 8 cleanup checks, should they all run or should the user confirm first?**
*Why: Auto-running many checks may be surprising. Do users expect a preview/confirm step?*

**Q3.4: What if a single check takes a very long time (marked as "slow")? Should we warn the user before running?**
*Why: User expectations—if they run `gt doctor stale-binary` expecting 2 seconds but it takes 30, they feel lied to.*

**Q3.5: If the user runs `gt doctor orphaned-sessions` and there are no orphans, what's the emotional tone of the output?**
- "All clear" (positive, celebrates good state)?
- "No issues found" (neutral)?
- Just the checkmark (minimal)?

*Why: Single-check runs feel more personal/targeted. Tone should reflect the user's intent (confirmation vs discovery).*

**Q3.6: What if the user runs `gt doctor orphaned-sessions --fix` but the fix fails partway through? (e.g., can't kill 1 of 5 sessions due to permissions)**
*Why: Partial success is a common edge case. Users need to see what succeeded vs failed.*

**Q3.7: If a check has prerequisites that failed (e.g., requires daemon running), how should that be presented?**
*Why: Users need to know why a check was skipped or blocked, and what to do next.*

**Q3.8: When running `gt doctor list`, should it show all checks or only "user-runnable" checks (excluding internal validation checks)?**
*Why: Overwhelming users with 47 checks they can't understand defeats the purpose.*

**Q3.9: If a user runs the same check twice in a row, should output be identical or should it show "nothing changed since last run"?**
*Why: Users running checks in debugging loops benefit from "delta" output, not full repetition.*

**Q3.10: What if a check's fix can be applied multiple ways (conservative vs aggressive)? Should users be prompted or is `--fix` just one flavor?**
*Why: Some users want to safely remove a few sessions, others want aggressive cleanup. One-size-fits-all may not work.*

**Q3.11: For checks with verbose details (50+ orphaned sessions), should we offer pagination in the terminal output or truncate with a count?**
*Why: Long outputs scroll off screen. Users need a way to navigate potentially large results.*

---

## 4. VISUAL & LAYOUT

### Questions: Where does this live and how should it look?

**Q4.1: Should `gt doctor <specific>` output use the same visual style as full doctor runs (streaming icons, categories) or a simplified layout?**
*Why: Visual consistency creates a cohesive product feel. But single-check output may benefit from being less "busy."*

**Q4.2: For a single check that passes, should we show the full check name, description, and elapsed time? Or just an icon + "✓ Check passed"?**
*Why: Information density—what's the right signal-to-noise ratio for targeted checks?*

**Q4.3: Should the output for `gt doctor orphaned-sessions` look similar to running all checks and seeing the results, just filtered?**
*Why: Consistency in UI reduces cognitive load. Users expect the same check to look the same regardless of context.*

**Q4.4: When showing a list of issues (e.g., 5 orphaned sessions), should each be clickable/selectable (TUI interaction) or just viewable text?**
*Why: Some users want to interactively pick which sessions to kill. Others want fire-and-forget automation.*

**Q4.5: Should fix hints always be visible in the output, or only when a problem is detected?**
*Why: Noise vs helpfulness. Fix hints are distracting when everything passes.*

**Q4.6: For checks with many details, should we show a "See all X results in full" link that opens a pager view?**
*Why: Terminal real estate is limited. Pagers are good for exploring, but should they be automatic or opt-in?*

**Q4.7: Should the command respect terminal width and wrap/truncate output appropriately, or just let it overflow?**
*Why: UX quality—professional tools adapt to the environment. Hard-coded widths feel brittle.*

**Q4.8: When displaying timestamps (elapsed time), should they be human-readable ("2.3 seconds") or technical ("2300ms")?**
*Why: Audience matters. Developers understand milliseconds, but users scanning results prefer human-friendly format.*

**Q4.9: Should error messages in single-check mode use the same styled format (emoji + color) as the full doctor output?**
*Why: Consistency in error presentation builds trust. Inconsistent styling feels unprofessional.*

**Q4.10: For checks that are marked as "experimental" or "beta," should there be a visual indicator before running them?**
*Why: Setting expectations—users should know if a check is still being refined.*

---

## 5. STATES & TRANSITIONS

### Questions: What states can this feature be in, and how do users move between them?

**Q5.1: What are all the possible states a single-check execution can be in? (Loading → Running → Complete → Error → Fixed)**
*Why: State clarity determines output design. Each state may need different visual/informational treatment.*

**Q5.2: For a "Loading" state (check setup), should we show a spinner or just jump to "Running"?**
*Why: Users want feedback that something is happening. Even brief waits feel long without indication.*

**Q5.3: During execution, should we update the output in-place (clearing and rewriting) or append new lines?**
*Why: Scrollback history—users may want to see what happened. In-place updates are faster visually but lose history.*

**Q5.4: After `--fix` is applied, should the output automatically transition to re-running the check to confirm success, or just report what was fixed?**
*Why: Confirmation that the fix actually worked is valuable but creates double runtime. What's the right UX?*

**Q5.5: If a user runs `gt doctor orphaned-sessions`, sees results, and then runs it again, should the output indicate "no new issues since last run"?**
*Why: Repeated checks in debugging loops benefit from delta output, not seeing the same output twice.*

**Q5.6: For a check in "Warning" state (issue found but not critical), should we automatically suggest the next action (run with --fix) or let user decide?**
*Why: Guidance level—some users want a hand-held experience, others want minimal suggestions.*

**Q5.7: When a check transitions from "Running" to "Complete," should the output change dramatically or incrementally append the result?**
*Why: Visual continuity—jarring transitions feel like bugs. Smooth transitions feel intentional.*

**Q5.8: If a user interrupts a check mid-run (Ctrl+C), what state should we show? Should we offer cleanup/rollback options?**
*Why: Graceful interruption is crucial for user control. Leaving the system in a broken state erodes trust.*

**Q5.9: For checks that fix things, should "Fixed" be a celebratory success state or treated differently from "All clear" (nothing was wrong)?**
*Why: Different emotional outcomes—fixing something feels different from confirming nothing is broken. UX should reflect this.*

**Q5.10: Should a check that skips due to unmet prerequisites be visually distinct (warning color) or just noted in text?**
*Why: Scannability—users should see at a glance that a check didn't run, not have to read all text.*

**Q5.11: If the user runs `gt doctor cleanup` and it shows "Running 8 checks," should we show progress (1/8, 2/8) as they complete?**
*Why: Progress feedback is motivating and manages expectations for slow operations.*

---

## 6. EDGE CASES & SPECIAL SCENARIOS

### Questions: What unique situations might users encounter?

**Q6.1: What if a check name is ambiguous? (e.g., "orphan" could match "orphaned-sessions" or "orphaned-processes")**
*Why: Clarity—the system should never guess wrong or silently run the wrong check.*

**Q6.2: What if the user has multiple Gas Town workspaces and runs `gt doctor orphaned-sessions` in one, then switches to another? Should the output look the same?**
*Why: Context matters. Different workspaces may have different issues. Consistency in UI doesn't mean identical results.*

**Q6.3: If a check requires special permissions (e.g., killing other sessions), and the user lacks permissions, how should this be communicated?**
*Why: Users need clear explanations of why operations fail. "Permission denied" without context is unhelpful.*

**Q6.4: What if a user runs `gt doctor orphaned-sessions` in CI/automation where interactive prompts are impossible?**
*Why: Different contexts (human vs automation) need different UX. Blocking on prompts breaks CI.*

**Q6.5: If a fix modifies user data (e.g., kills sessions they might care about), should we require explicit confirmation or trust the `--fix` flag?**
*Why: Safety vs convenience. Too many prompts are annoying; too few can be dangerous.*

**Q6.6: What if a check discovers that another check should also be run? (e.g., orphaned-sessions finds evidence of the daemon being down)**
*Why: Cascading issues need UX handling. Do we recommend, suggest, or auto-run related checks?*

**Q6.7: For very fast checks (< 100ms), should we still show streaming output or batch them together?**
*Why: Noise reduction—showing each check updating is great for slow checks but annoying for fast ones.*

**Q6.8: If the user runs a check that requires network (API calls, remote checks), how should timeout/failure be handled?**
*Why: Network is unreliable. Users need to know whether the check failed due to environment or actual issues.*

**Q6.9: What if documentation/help for a specific check is very long? How should we surface it—inline, in pager, or via help subcommand?**
*Why: Users running targeted checks may need more context about that check specifically.*

**Q6.10: If a user repeatedly runs the same check (e.g., in a debugging loop), should we cache results briefly or always recompute?**
*Why: Caching is faster but may show stale info. Always recomputing is slower but more accurate.*

---

## 7. DISCOVERY & ONBOARDING

### Questions: How do new users discover and understand this feature?

**Q7.1: How does a user even know they can run `gt doctor orphaned-sessions`? Should this be documented, suggested, or discoverable via help?**
*Why: Feature discoverability is critical. Users won't use features they don't know exist.*

**Q7.2: Should `gt doctor --help` or `gt doctor -h` show the full list of available checks, or just explain the feature?**
*Why: Help text length tradeoff—comprehensive help is useful but overwhelming. What's the right level?*

**Q7.3: Should there be shell completion/autocomplete for check names (bash/zsh)?**
*Why: Discoverability through autocomplete is modern UX. Users can explore available checks as they type.*

**Q7.4: If a user runs an invalid check name, should the error suggest the `list` command or show examples?**
*Why: Error-driven discovery—good error messages are often better teaching tools than docs.*

**Q7.5: Should the feature have a "getting started" mode (e.g., `gt doctor --guide`) that walks through how to use selectivity?**
*Why: Some users benefit from interactive guidance. Others find it annoying. What's the right default?*

**Q7.6: Should checks have "difficulty" or "risk" levels displayed (Safe/Moderate/Advanced) when listing them?**
*Why: Users need to understand consequences. A "low risk" label on orphaned-sessions tells users it's safe to run.*

**Q7.7: For checks that have been recently added, should there be a "new" indicator in the list?**
*Why: Users might not know about newly available targeted checks.*

---

## 8. FEEDBACK & COMMUNICATION

### Questions: How does the system communicate with the user?

**Q8.1: If a check takes longer than expected, should we proactively show a message like "This check is taking longer than usual"?**
*Why: Managing expectations prevents user frustration. Early warning feels helpful, not like the system is broken.*

**Q8.2: For checks with side effects (deletions, restarts), should we always show a "What will be changed" preview before applying --fix?**
*Why: Safety first—users should see what's about to happen, especially for destructive operations.*

**Q8.3: After a fix is applied, should we show before/after metrics? (e.g., "Killed 5 sessions | Before: 7 sessions, Now: 2 sessions")**
*Why: Impact visibility—showing what changed makes the action feel real and measurable.*

**Q8.4: If a check generates warnings or secondary issues during fix, should we highlight these separately or include in the main output?**
*Why: Signal-to-noise—important warnings shouldn't get lost among success messages.*

**Q8.5: Should the output include a timestamp of when the check was run? Useful for logs but might clutter terminal output.**
*Why: Traceability vs noise. What's the right level?*

**Q8.6: For multi-item results (e.g., 50 orphaned sessions to kill), should we show a count first ("Found 50 orphaned sessions") before listing them?**
*Why: Scanning efficiency—users can decide whether they want to see all items based on the count.*

**Q8.7: When a check has details/context that might be important, should we use callout boxes, colors, or just plain text?**
*Why: Visual hierarchy—important information should stand out from noise.*

---

## 9. PERFORMANCE & RESPONSIVENESS

### Questions: How should the feature feel in terms of speed and responsiveness?

**Q9.1: If a user runs `gt doctor cleanup` (8 checks) vs `gt doctor orphaned-sessions` (1 check), what's the acceptable speed difference?**
*Why: Users have different expectations for batch vs single. Too slow feels broken.*

**Q9.2: Should the output start appearing immediately (streaming) even for fast checks, or batch output for very fast checks?**
*Why: Perceived responsiveness—instant visual feedback feels faster than instant-but-quiet execution.*

**Q9.3: For checks marked as "slow," should we show a spinner/progress indicator while running?**
*Why: Feedback prevents user anxiety. Without indication, a 30-second wait feels endless.*

**Q9.4: Should we show elapsed time for each check, or only if it exceeds a threshold (e.g., > 1 second)?**
*Why: Noise vs useful data—quick checks don't need timing shown, but slow ones do for context.*

**Q9.5: If running multiple checks with `--fix`, should we apply fixes serially (one by one) or in parallel?**
*Why: Speed vs safety. Parallel is faster but might create interdependencies/conflicts.*

---

## 10. CONSISTENCY & MENTAL MODELS

### Questions: How should this fit into the broader `gt doctor` experience?

**Q10.1: Should a targeted check show a message like "Running: orphaned-sessions (1 of 47 available checks)"?**
*Why: Context—users should understand they're running 1 of many, not that other checks are disabled.*

**Q10.2: If a user runs `gt doctor orphaned-sessions --fix` and something goes wrong, should the error style match other doctor errors?**
*Why: Consistency in error presentation. Users trust tools that feel cohesive.*

**Q10.3: When viewing results, should the category (Cleanup, Config, etc.) still be shown for single checks?**
*Why: Metadata helps users understand what kind of check this is, but may be redundant for targeted runs.*

**Q10.4: Should `gt doctor list` show which checks are fixable (have --fix support) and which are read-only?**
*Why: Users planning their workflow need to know which checks can be fixed.*

**Q10.5: If combining single-check mode with `--verbose`, should it show more details about internal execution or just more context about the issue?**
*Why: Verbose is ambiguous. Different users expect different things from verbose mode.*

**Q10.6: For checks that can run in different "modes" (e.g., `--rig` for rig-specific mode), should single-check mode be mode-aware?**
*Why: Users expect flags to compose naturally. Breaking this feels like inconsistent design.*

---

## SUMMARY OF KEY UX DECISIONS TO MAKE

1. **Display Style**: Does single-check output look like filtered full output, or simplified?
2. **Interaction Model**: Positional argument or subcommand? Single check or multiple?
3. **Discoverability**: How do users find available checks? (list command, help, autocomplete?)
4. **Error Recovery**: How helpful are suggestions for typos/invalid checks?
5. **Fix Confirmation**: Auto-apply with --fix or require confirmation for destructive operations?
6. **Output Verbosity**: Default to minimal success messages or mirror full doctor output?
7. **Batch Behavior**: How should `gt doctor cleanup` work for multiple checks?
8. **Error Communication**: How clearly are prerequisites, permissions, and failures explained?
9. **Performance Feedback**: How do we indicate slow operations without overwhelming output?
10. **Mental Model Alignment**: Does this feel like one tool (gt doctor) or multiple modes?

---

## Domain Expert Perspective

# Domain Expert Analysis: Adding Selective Doctor Fixes

**Feature Brief**: Add support for `gt doctor <specific-doctor-fix>` like `gt doctor orphaned-sessions`

---

## DOMAIN CONCEPTS & MISSING DEFINITIONS

1. **What is a "doctor check" in the context of developer workflows?**
   Understanding whether users think of this as a diagnostic tool, a repair tool, or both will shape expectations about what information should be surfaced and when actions should be automatic.

2. **Is an "orphaned session" a type of system resource leak unique to this workspace, or a general tmux concept?**
   Clarifying whether orphaned sessions are specific to Gas Town's architecture versus a general problem developers encounter will help us identify what other similar issues might exist.

3. **What makes a check "fixable" versus "informational" in the user's mental model?**
   Users need to understand the boundary between checks that are safe to auto-fix versus those that require human judgment, which directly impacts trust in selective fixes.

4. **Are "cleanup" operations idempotent—can they be safely run multiple times without data loss?**
   If cleanup operations have side effects or can fail partway through, users need guarantees about rollback or partial-success visibility.

5. **Does "selective doctor" imply users want to run specific checks without running the full suite, or do they want fine-grained control within a category?**
   The frequency and reasons users might skip checks will determine if we need category-based filtering, individual check selection, or both.

6. **What is the relationship between "doctor checks" and the broader Gas Town maintenance/health ecosystem?**
   If doctor is part of a larger health monitoring strategy, understanding what happens when selective checks conflict with other systems matters.

7. **Are there interdependencies between checks that we should prevent users from bypassing?**
   Some checks might be prerequisites for others (e.g., check A must pass before running fix for check B), which affects the mental model of independence.

8. **How do users currently know which check to run when they encounter an issue?**
   If users have to remember check names or search documentation, the discoverability problem shapes feature scope and UX design.

9. **What does "doctor" mean in the Gas Town jargon—is it a metaphor that breaks down, or is it consistently understood?**
   If developers think of doctor as a metaphor (like Docker's health checks), they may expect different behavior than if they think of it as a literal repair system.

10. **Are there checks that should NEVER be auto-fixed, even with the `--fix` flag?**
    Certain checks might have too much risk or require decisions that only humans should make, which affects the design of selective fixes.

11. **Do users expect selective fixes to output the same report format as full-suite runs, or something simpler?**
    If users run `gt doctor orphaned-sessions`, should they still see a summary of all 47 checks, or just the one they asked for?

12. **Is there a use case for running multiple selective checks in one command (e.g., `gt doctor cleanup orphaned-sessions wisp-gc`)?**
    Understanding whether users want batching will affect the CLI syntax and mental model.

---

## PRIOR ART & USER CONVENTIONS

1. **How do other diagnostic tools in developer ecosystems handle selective checks (e.g., linters, health checks, package managers)?**
   Understanding conventions from npm audit, Go vet, Docker health, or Kubernetes liveness probes will shape user expectations about syntax and behavior.

2. **What do developers already expect from the `gt doctor` command based on their experience with similar tools?**
   If they've used Docker health checks, system health diagnostics, or code linters, they have mental models we should either meet or explicitly violate.

3. **Do developers expect a "dry-run" or "preview" mode for fixes before they're applied?**
   Many system administration tools offer `--dry-run` flags; understanding if users expect this for selective fixes matters for safety perception.

4. **What naming conventions do similar tools use for selective filtering (e.g., by name, by category, by tag)?**
   Whether we use `gt doctor orphaned-sessions` (name), `gt doctor --category cleanup` (category), or `gt doctor --tag=session` (tag) should align with user intuition.

5. **How do other CLI tools handle ambiguous inputs (e.g., what if "orphan" could match multiple checks)?**
   Understanding fuzzy matching, substring matching, or exact-match expectations will shape error handling and UX.

6. **What do developers expect when they mistype a check name—do they want suggestions, a list of available checks, or an error?**
   Error recovery experience is a critical part of the user journey for selective commands.

7. **Have other Gas Town features used selective subcommands, and what patterns did they establish?**
   Consistency with existing Gas Town conventions matters more than external tools, as users already have mental models from using this workspace.

8. **What's the convention for whether selective operations should exit with an error code if the check they run finds a problem?**
   Understanding failure semantics for CI/CD integration and script usage will affect the contract of this feature.

9. **Do developers expect selective fixes to be faster/more efficient than full-suite runs?**
   If performance is a driver for selective execution, that's different from the use case of "I only care about this one thing right now."

10. **Are there examples in Gas Town documentation or existing features that set user expectations about what "doctor" does?**
    Users may have formed mental models from tutorials, help text, or past experiences with the tool.

11. **Should selective doctor operations be logged differently than full-suite runs?**
    Understanding logging and audit expectations will shape integration with event systems and history tracking.

12. **What do developers expect to happen to the output/report if they run a selective check and it has no issues?**
    Should it output "everything is fine" or stay silent? This affects perceived workflow friction.

---

## PROBLEM DEPTH & ROOT CAUSES

1. **Is the real problem that running all 47 checks is slow, or that users only care about specific issues at specific times?**
   If it's a speed problem, the solution might be parallelization or caching; if it's a focus problem, selective execution is the right answer.

2. **Are developers running full doctor suites repeatedly, looking for a specific fix to succeed?**
   If so, selective fixes reduce noise and time-to-success, which is a strong use case justification.

3. **Do users avoid running doctor entirely because the full output is overwhelming?**
   If so, selective execution increases adoption (by reducing cognitive load) versus just being a convenience feature—shapes priority.

4. **What workflows trigger the need for selective fixes—is it incident response, CI/CD integration, daily maintenance, or something else?**
   The frequency and context of usage will determine UX priorities and where in the workflow this feature matters.

5. **Are there checks that are "noisy" and users want to suppress them without modifying doctor configuration?**
   If so, selective execution might be solving the wrong problem—we might need muting/silencing instead.

6. **Is the problem that some developers don't understand what each check does and want to run only the ones they trust?**
   This suggests a discovery/documentation problem might be more fundamental than the feature request.

7. **Do different team members or roles care about different checks (e.g., platform team cares about infrastructure, developers care about sessions)?**
   This would indicate a deeper need for role-based or profile-based doctor configurations.

8. **Is selective doctor fixing an attempt to avoid accidental data loss from aggressive auto-fixes?**
   If safety/consent is the issue, we need to think about warnings and rollback capabilities, not just selectivity.

9. **Would developers actually use selective fixes, or is this a feature that sounds good in theory but won't be adopted?**
   Understanding the actual problem developers face (through usage patterns or support requests) matters more than feature requests.

10. **Are there checks that should only run in certain contexts (e.g., only in CI/CD, only on a dev machine, only in a specific rig)?**
    If contextual filtering is needed, selective execution might not be the complete solution.

11. **Is the problem that doctor output is too verbose for users who only care about one thing?**
    If so, the issue is output formatting, not check selection—fixing one might solve the other.

12. **Are there hidden dependencies between checks that would break if users run selective fixes?**
    Understanding the graph of check interdependencies will reveal whether selective execution is even safe.

13. **Do users want to run selective fixes to speed up development iteration (e.g., fixing one issue, running that check again to verify)?**
    This would indicate a significant workflow improvement opportunity.

14. **Is there a pattern of users having to run full doctor, finding one issue, fixing it, and running full doctor again?**
    This feedback loop would strongly justify selective execution as a quality-of-life improvement.

---

## EDGE CASES & UNUSUAL SCENARIOS

1. **What happens if a user runs `gt doctor orphaned-sessions --fix` but the fix partially fails (e.g., kills 5 of 10 orphaned sessions)?**
   Partial success visibility and recovery paths are critical for user trust in selective fixes.

2. **Can a user intentionally run checks in a specific order, or must checks always run in the order defined by the doctor?**
   If order matters for safety, selective execution needs to preserve those constraints.

3. **What if a selective fix introduces a new issue that would be caught by a different check?**
   Should we warn users that they should run related checks, or is that outside the scope of this feature?

4. **Can selective fixes be used in automated scripts/CI without human verification, or are they only for interactive use?**
   Understanding the trust/safety model for automation shapes constraints and defaults.

5. **What if a check is registered multiple times under different names (e.g., `orphaned-sessions` and `orphans`)?**
   Handling aliases and name variations will affect the mental model of which names are "canonical."

6. **How should the feature behave if a check name is ambiguous (e.g., both `orphan-sessions` and `orphan-processes` match a substring)?**
   Error recovery and disambiguation strategies need to be clear.

7. **Can users run the same selective check multiple times in one session (e.g., fix it, then run again to verify)?**
   If checks are stateful or have side effects, re-running might behave differently.

8. **What if a rig-specific check is requested but the user isn't in a rig context?**
   Should we error, skip it, or require `--rig` flag awareness?

9. **Can selective fixes be combined with global flags like `--verbose` or `--slow` in meaningful ways?**
   Understanding flag combinations and their semantics will shape the CLI design.

10. **What if a user runs `gt doctor --fix` (full suite) while another process is running `gt doctor orphaned-sessions --fix`?**
    Concurrency and locking behavior needs to be defined, especially for fixes that modify system state.

11. **Should selective fixes be reversible or have undo capability?**
    Some fixes might be destructive (e.g., killing sessions); understanding rollback expectations shapes the feature's risk profile.

12. **What if a selective fix needs to fetch external state (e.g., list of valid sessions) that changes between invocations?**
    Cache invalidation and freshness guarantees will affect consistency and correctness.

13. **Are there regulatory, compliance, or audit considerations for auto-fixing system state via doctor?**
    Certain environments (finance, healthcare, regulated industries) might have constraints on automated actions.

14. **What if a user tries to run a check that exists but is context-dependent (e.g., a rig check when not in a rig)?**
    Understanding graceful degradation or error handling for context mismatches shapes user experience.

---

## SUCCESS CRITERIA & METRICS

1. **How do we measure whether selective doctor fixes actually reduce friction in developers' workflows?**
   Metrics like "time to resolve a known issue" or "frequency of doctor usage" would validate the feature's value.

2. **What does "good" look like for discoverability—do developers know which check to run without documentation?**
   Understanding whether users can intuit check names (`orphaned-sessions` vs. `orphans` vs. `stale-sessions`) is a success metric.

3. **Should we track which selective checks are most commonly run to understand developer priorities?**
   Usage patterns might reveal whether certain checks are confusing or whether some use cases aren't being addressed.

4. **How do we ensure selective fixes maintain the same safety guarantees as the full doctor suite?**
   Testing coverage, regression detection, and user confidence in selective operations are success metrics.

5. **Is adoption of selective doctor a success metric, or is the goal to reduce unnecessary doctor runs?**
   Different goals lead to different success measures (usage increase vs. full-suite reduction).

6. **How should we measure the confidence users have in selective auto-fixes—do they trust the feature?**
   Support tickets, GitHub issues, or surveys about fix safety would indicate whether confidence is sufficient.

7. **What's the acceptable error rate for selective fixes (e.g., is 1 unexpected behavior per 1000 runs acceptable)?**
   Understanding the tolerance for edge cases will shape testing and rollout strategies.

8. **Should we measure whether selective fixes catch issues faster than waiting for full doctor to complete?**
   Speed improvements might be a secondary benefit, but worth tracking if they matter to users.

9. **Do we want selective doctor to reduce the "cognitive load" of understanding 47 different checks?**
   If so, metrics around learning curve, documentation questions, or error messages might indicate success.

10. **How would we know if selective fixes are preventing important context from being visible to users?**
    If a focused fix masks a related issue, that's a failure that metrics should catch.

11. **Is there a target adoption rate for selective doctor usage, and how does it compare to full-suite runs?**
    Understanding the expected usage distribution will help validate whether the feature is being adopted as intended.

12. **Should selective fixes have different SLAs or performance targets than full doctor runs?**
    Understanding expectations for speed, reliability, and resource consumption of selective operations shapes success criteria.

13. **How do we measure whether selective doctor helps specific workflows (CI/CD, incident response, local development)?**
    Different use cases might have different success definitions and metrics.

14. **Would we consider it a success if selective doctor reduces the frequency of support questions about a particular check?**
    This would indicate that giving users more control reduces confusion and support burden.

---

## EXPLICIT NON-GOALS & BOUNDARIES

1. **Is this feature ONLY for interactive/manual use, or should it support automation/scripting?**
   Clarifying the intended audience (humans vs. scripts) shapes safety guarantees and output formatting.

2. **Should selective doctor support filtering by category (e.g., `gt doctor cleanup`) or only by individual check name?**
   Understanding whether category filtering is in or out of scope affects API design.

3. **Should this feature enable users to disable/skip certain checks permanently, or is it only for selective one-off runs?**
   If permanent configuration is needed, that's beyond selectivity and requires a different feature.

4. **Is selective doctor meant to replace or supplement the existing full-suite doctor command?**
   Understanding whether this is additive or transformative shapes backward compatibility considerations.

5. **Should selective fixes support composition or piping (e.g., `gt doctor | doctor --fix`)?**
   Understanding whether advanced workflow integration is expected shapes CLI design.

6. **Are we committing to supporting selective fixes for ALL 47 checks, or just the "fixable" ones?**
   Clarifying scope prevents user confusion when selective fixes fail for read-only checks.

7. **Should selective doctor output be machine-parseable (JSON, structured format) or human-readable?**
   If automation is a non-goal, simpler output is acceptable; if it's a goal, structure matters.

8. **Is selective doctor meant to make it easier to run doctor in CI/CD pipelines, or is that out of scope?**
   Understanding whether this feature is solving CI/CD pain points shapes what we optimize for.

9. **Should selective fixes support conditional logic or constraints (e.g., "only fix if X is true")?**
   If advanced conditional execution is out of scope, that simplifies the feature.

10. **Are we responsible for explaining WHY each check exists, or just enabling users to run them?**
    Understanding whether educational value is in or out of scope affects documentation and discoverability.

11. **Should we provide a discoverable list of available checks (e.g., `gt doctor list`)?**
    If discovery is out of scope, users must rely on documentation or command-line help.

12. **Is partial-run optimization (e.g., skip expensive checks if only running selective) in scope?**
    If so, this shapes the internal architecture; if not, we always pay the full cost.

---

## SYNTHESIS & CRITICAL QUESTIONS

1. **Is this a feature request solving a known, validated problem, or a guess about what developers want?**
   Understanding whether the demand is organic or speculative will shape investment and timeline decisions.

2. **Who would use selective doctor and why—what's their specific workflow or pain point?**
   Without understanding the actual user, we risk building a feature nobody uses.

3. **Would developers prefer selective filtering (run fewer checks) or better output (same checks, better presentation)?**
   Understanding the root need prevents solving the wrong problem.

4. **How does selective doctor interact with other Gas Town features like configuration, automation, or monitoring?**
   Understanding the broader ecosystem prevents creating isolated features that don't integrate.

5. **What's the relationship between doctor selectivity and other pain points developers have mentioned or are experiencing?**
   Understanding whether this is the priority problem or a symptom of something deeper shapes prioritization.

6. **Do we have data on how frequently developers run `gt doctor` currently, and for what reasons?**
   Understanding actual usage patterns would validate or challenge the underlying assumption that selective execution is needed.

7. **Have any users explicitly asked for this feature, or is it a proactive design decision?**
   Direct user feedback is more reliable than assumed needs.

8. **What would prevent a user from just running the full doctor suite if they only care about one check?**
   Understanding the actual friction point (time, noise, complexity, risk) shapes the solution.

---

## Cross-Perspective Themes (Haiku)

### Theme 1: Discoverability as a Foundation for Adoption
All three perspectives emphasize that users cannot use a feature they don't know exists. The User Advocate raises the need for `gt doctor --list` and shell auto-complete, the Product Designer asks how to make the feature discoverable through help text and error recovery, and the Domain Expert questions whether clear discoverability is even in scope. This theme is foundational—without solving discoverability, the entire feature fails to reach users. Implementation must include: a built-in list command, shell completion, prominent help text documentation, and error messages that suggest valid options.

### Theme 2: Mental Model Alignment with User Expectations
All three perspectives circle around whether users will intuitively understand what `gt doctor orphaned-sessions` does. The User Advocate explores whether users expect auto-fix or just diagnostics, the Product Designer questions output format consistency, and the Domain Expert asks what "doctor" means in Gas Town's jargon. The core issue is that misaligned mental models create wrong expectations and frustration. Success requires consistent, predictable behavior that maps to existing patterns users know (like `apt install package` or `git commit`).

### Theme 3: Safety and Risk Management for Destructive Operations
All three perspectives acknowledge that selective fixes are potentially risky because they modify system state. The User Advocate discusses dry-run, undo, and idempotency expectations, the Product Designer asks about confirmation flows and partial-success visibility, and the Domain Expert questions whether certain checks should never be auto-fixed. This theme shapes critical UX decisions: must users confirm destructive operations? Can fixes be undone? How do we communicate what will be changed before applying fixes?

### Theme 4: Output and Feedback Optimization for Context
All three perspectives recognize that single-check output differs from full-doctor output in subtle ways. The User Advocate questions whether output should be simpler, the Product Designer asks about visual hierarchy and information density, and the Domain Expert wonders whether to show all checks or just the one requested. The common thread is that different contexts (human debugging vs. CI/CD scripting vs. rapid iteration) need different levels of verbosity and confirmation.

### Theme 5: Performance and Speed as a Value Proposition
All three perspectives note that the primary justification for selective execution is speed. The User Advocate emphasizes that waiting for 47 checks when you need one is frustrating, the Product Designer asks about acceptable speed differences, and the Domain Expert questions whether parallelization or caching is the real solution. This suggests that if selective fixes aren't noticeably faster than full-suite runs, users won't adopt them—making performance a non-negotiable success metric.
