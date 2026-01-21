# Rule of 5 Branch Review Prompt

## Purpose

This prompt launches 5 independent review agents to analyze the `refactor/agents-clean` branch from different angles. Each agent provides a unique perspective, and results are synthesized into a comprehensive correctness assessment.

## How to Use

Run each of these 5 prompts as parallel Task agents, then synthesize the outputs.

---

## Agent 1: Behavioral Equivalence Review

```
You are reviewing the refactor/agents-clean branch for BEHAVIORAL EQUIVALENCE.

Goal: Verify that user-facing behavior is unchanged by the refactor.

Tasks:
1. Identify all public CLI commands that use agent lifecycle (start/stop/status)
2. Trace the code path for each command before and after refactor
3. Verify error messages, exit codes, and output formats are preserved
4. Check that all flags and options still work

Focus areas:
- gt up / gt down
- gt daemon start / stop
- gt witness/refinery/polecat start/stop
- gt status output format
- gt agents list

Output: List any behavioral differences found, categorized as:
- BREAKING: User-visible behavior changed
- IMPROVED: Better behavior, backwards compatible
- NEUTRAL: Internal change only
```

---

## Agent 2: Error Handling Review

```
You are reviewing the refactor/agents-clean branch for ERROR HANDLING correctness.

Goal: Verify that all error cases are properly handled after the refactor.

Tasks:
1. Find all error returns in factory.Start(), factory.Agents().Stop(), etc.
2. Verify callers handle these errors appropriately
3. Check that error messages are user-friendly and actionable
4. Verify no panics can occur from nil pointer dereferences

Focus areas:
- Session startup failures
- Session not found errors
- Timeout during WaitReady
- Zombie session detection and cleanup

Output: List any error handling gaps found:
- CRITICAL: Unhandled error could cause crash or data loss
- WARNING: Poor error message or swallowed error
- OK: Properly handled
```

---

## Agent 3: Race Condition Review

```
You are reviewing the refactor/agents-clean branch for RACE CONDITIONS.

Goal: Verify that concurrent access patterns are safe.

Tasks:
1. Identify all shared state in the factory/agent packages
2. Check for proper mutex usage around shared maps and counters
3. Verify tmux session operations are serialized where needed
4. Check for TOCTOU (time-of-check-time-of-use) bugs in Exists()->Start() patterns

Focus areas:
- factory.agents singleton access
- session.nudgeMu per-session mutex
- Multiple agents starting simultaneously (gt up parallelization)
- Daemon tick racing with manual gt commands

Output: List any race conditions found:
- CRITICAL: Data corruption or crash possible
- WARNING: Potential race but low impact
- MITIGATED: Race exists but handled correctly
```

---

## Agent 4: Resource Leak Review

```
You are reviewing the refactor/agents-clean branch for RESOURCE LEAKS.

Goal: Verify that all resources are properly cleaned up.

Tasks:
1. Find all places where tmux sessions are created
2. Verify each has a corresponding cleanup path
3. Check for orphaned process cleanup on errors
4. Verify file handles, locks, and markers are released

Focus areas:
- factory.Start() error paths - does zombie get cleaned?
- Deferred lock releases in boot package
- Marker files (.boot-running, etc.)
- Process tree cleanup on Stop()

Output: List any resource leaks found:
- CRITICAL: Persistent leak that grows over time
- WARNING: Leak on error path only
- OK: Properly cleaned up
```

---

## Agent 5: Test Coverage Review

```
You are reviewing the refactor/agents-clean branch for TEST COVERAGE.

Goal: Verify that the refactored code has adequate test coverage.

Tasks:
1. List all new files created by the refactor
2. Check each has corresponding test file
3. Verify test doubles (agent.Double, session.Double) match real implementations
4. Run conformance tests to verify doubles behave correctly

Focus areas:
- internal/factory/factory_test.go coverage
- internal/agent/conformance_test.go passing
- internal/session/double.go matches session.go behavior
- Manager test files use doubles correctly

Output: Coverage assessment:
- CRITICAL: Core logic untested
- WARNING: Edge case untested
- GOOD: Adequate coverage
- EXCELLENT: Comprehensive coverage with conformance tests
```

---

## Synthesis Prompt

After running all 5 agents, use this prompt to synthesize:

```
You have received 5 independent review reports for the refactor/agents-clean branch:

1. Behavioral Equivalence Review
2. Error Handling Review
3. Race Condition Review
4. Resource Leak Review
5. Test Coverage Review

Synthesize these into a final assessment:

1. CRITICAL ISSUES (must fix before merge):
   - List any CRITICAL findings from any review

2. WARNINGS (should address):
   - List any WARNING findings

3. IMPROVEMENTS NOTED:
   - List any positive changes noted

4. OVERALL ASSESSMENT:
   - READY TO MERGE: No critical issues, warnings are acceptable
   - NEEDS WORK: Has critical issues that must be addressed
   - RECOMMEND CHANGES: No critical issues but significant warnings

5. RECOMMENDED ACTIONS:
   - Prioritized list of fixes/improvements
```

---

## Running the Review

To execute this review:

```bash
# Option 1: Run sequentially
claude "Run Agent 1 prompt from BRANCH_REVIEW_PROMPT.md"
claude "Run Agent 2 prompt from BRANCH_REVIEW_PROMPT.md"
# ... etc

# Option 2: Run in parallel (recommended)
# Use Claude Code's Task tool to launch all 5 agents simultaneously
```
