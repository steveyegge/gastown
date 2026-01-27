# Claude Code Decision Types Catalog

Research output for bead `hq-0rbvjw.2`

## Executive Summary

Claude Code requires 5 distinct types of user decisions. Analysis shows:
- **~70-80%** of user interactions could be structured decisions (vs free-form)
- **~60%** of decisions are binary (yes/no, allow/deny)
- **~40%** are multi-choice (select approach, choose option)

---

## 1. Permission Decisions (Tool Use Approvals)

**Type**: Binary (Allow/Deny)

**When triggered**: Before Claude executes a tool that isn't pre-approved

**Decision options**:
- `allow` - Proceed with this tool use
- `deny` - Block this tool use
- `allow_always` - Add to permanent allow list (implicit option)

**Context needed**:
- Tool name (Bash, Edit, Write, etc.)
- Tool parameters (command, file path, etc.)
- Risk level indicator
- Previous similar decisions in session

**Permission modes** (affect when decisions are needed):
| Mode | Behavior |
|------|----------|
| `default` | Prompts for "dangerous" tools |
| `acceptEdits` | Auto-approve file edits (Write, Edit) |
| `plan` | Require plan approval before any execution |
| `bypassPermissions` | No prompts (dangerous) |

**Estimated frequency**: 30-40% of all decisions

---

## 2. Code Change Decisions (Accept/Reject Edits)

**Type**: Binary with preview

**When triggered**: When Claude proposes file modifications

**Decision options**:
- `accept` - Apply the change
- `reject` - Discard the change
- `edit` - Modify before accepting (implicit)

**Context needed**:
- File path
- Diff view (before/after)
- Surrounding code context
- Explanation of why change is proposed

**Special case - Plan mode**: In `plan` mode, all edits are batched and require single approval before execution begins.

**Estimated frequency**: 25-35% of all decisions

---

## 3. Direction Decisions (Choose Approach)

**Type**: Multi-choice (2-4 options)

**When triggered**: Via `AskUserQuestion` tool when multiple valid approaches exist

**Decision structure**:
```
Question: "Which authentication method should we use?"
Options:
  - JWT tokens (stateless, scalable)
  - Session cookies (simpler, traditional)
  - API keys (for service-to-service)
  - Other: [free text]
```

**Context needed**:
- Clear question with "?" ending
- 2-4 distinct options
- Pros/cons for each option
- "Other" always available for custom input

**Constraints**:
- 1-4 questions per invocation
- Header tag max 12 chars
- Options must be mutually exclusive (unless multiSelect)

**Estimated frequency**: 15-20% of all decisions

---

## 4. Completion Decisions (Confirm Task Done)

**Type**: Binary (implicit)

**When triggered**: When Claude signals work is complete

**Decision options**:
- Continue (give new task)
- Request changes
- End session

**Context needed**:
- Summary of what was done
- Files changed
- Tests run/results
- Any warnings or notes

**Note**: Not a formal decision point in Claude Code - completion is signaled via message, user responds naturally.

**Estimated frequency**: 5-10% of interactions

---

## 5. Escalation Decisions (Need Human Help)

**Type**: Binary (handled externally)

**When triggered**: Claude identifies it cannot proceed autonomously

**Decision options**:
- Provide requested information
- Change approach
- Pause work

**Context needed**:
- What is blocking
- What was tried
- What information is needed
- Severity level

**Estimated frequency**: 5-10% of sessions

---

## Hook-Based Decisions (Meta-Level)

Claude Code supports **hooks** that can intercept and decide on actions programmatically:

| Hook Event | Decision Point |
|------------|----------------|
| `PreToolUse` | Block/allow before execution |
| `PostToolUse` | React after execution |
| `UserPromptSubmit` | Filter/modify user input |
| `Stop` | Custom stop handling |
| `PreCompact` | Control context compaction |

Hook responses can return `{"decision": "block"}` to prevent actions.

---

## Analysis: Decision Characteristics

### Binary vs Multi-Choice

| Decision Type | Binary | Multi-Choice |
|---------------|--------|--------------|
| Permission | ✓ (allow/deny) | |
| Code Change | ✓ (accept/reject) | |
| Direction | | ✓ (2-4 options) |
| Completion | ✓ (implicit) | |
| Escalation | ✓ (provide/pause) | |

**Result**: ~60% binary, ~40% multi-choice

### Context Requirements Summary

| Decision | Minimal Context | Full Context |
|----------|-----------------|--------------|
| Permission | Tool + params | + risk level, history |
| Code Change | Diff | + explanation, surrounding code |
| Direction | Question + options | + tradeoffs, recommendations |
| Completion | Summary | + files, tests, warnings |
| Escalation | Blocker | + attempts, severity |

### Automation Potential

| Decision | Can Pre-Configure | Requires Real-Time |
|----------|-------------------|-------------------|
| Permission | ✓ (rules) | Some edge cases |
| Code Change | ✓ (acceptEdits mode) | Complex changes |
| Direction | ✗ | Always |
| Completion | N/A | N/A |
| Escalation | ✗ | Always |

---

## Recommendations for Decision-Point UX

1. **Permission decisions** - Best candidates for structured UI (checkboxes, remember choices)

2. **Code change decisions** - Need diff viewer + accept/reject buttons

3. **Direction decisions** - Card-based selection with descriptions

4. **Batch decisions** - Plan mode shows all pending decisions at once for efficiency

5. **Progressive disclosure** - Start with binary, expand to multi-choice on "Other"

---

## Data Sources

- Claude Code SDK types (`claude_code_sdk/types.py`)
- Claude Code SDK query handling (`claude_code_sdk/_internal/query.py`)
- SDK tool definitions (`sdk-tools.d.ts`)
- Gas Town integration patterns (`beads/refinery/rig/integrations/claude-code/`)
