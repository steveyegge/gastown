# Decision Points in Gas Town

Decision Points are a human-in-the-loop mechanism that enables agents to request structured decisions from humans before proceeding with work. They provide a formal way for agents to present options, gather input, and ensure alignment on important choices.

## Overview

In autonomous agent systems, there are moments where human judgment is essential:
- Architectural choices with significant tradeoffs
- Business decisions affecting scope or priority
- Ambiguous requirements needing clarification
- Risk points that are hard to reverse

Decision Points formalize these moments, ensuring agents pause and gather human input rather than making assumptions.

## Core Concepts

### What is a Decision Point?

A Decision Point is a **gate** that blocks workflow until a human responds. The agent presents:
- A clear question or prompt
- 2-4 structured options with descriptions
- Optional context or analysis
- Urgency level (high/medium/low)

The human can:
1. **Select an option** - Choose one of the presented options
2. **Select "Other"** - Enter custom text response (resolves decision directly)
3. **Provide text guidance** - Give custom instructions (triggers refinement)
4. **Accept as-is** - Proceed with agent's recommendation (after iteration 1)

### Decision Flow

```
Agent encounters decision point
        ↓
Creates decision: gt decision request --prompt "..." --option "A" --option "B"
        ↓
Human notified (mail, webhook, etc.)
        ↓
Human reviews and responds: gt decision resolve <id> --choice 1
        ↓
Agent proceeds with chosen option
```

## CLI Commands

### Creating Decisions

```bash
# Basic decision with two options
gt decision request \
  --prompt "Which caching strategy should we use?" \
  --option "Redis: Distributed, handles scaling" \
  --option "In-memory: Simple and fast"

# Decision with urgency and context
gt decision request \
  --prompt "Approve production deployment?" \
  --option "Deploy now: All tests passing" \
  --option "Wait: Schedule for maintenance window" \
  --urgency high

# Decision that blocks another bead
gt decision request \
  --prompt "Which API design?" \
  --option "REST: Standard, well-understood" \
  --option "GraphQL: Flexible queries" \
  --blocks hq-work-123
```

### Listing Decisions

```bash
# Show pending decisions
gt decision list

# Include resolved decisions
gt decision list --all

# Summary dashboard
gt decision dashboard
```

### Viewing Decision Details

```bash
gt decision show hq-abc123
```

Output:
```
🟢 Decision: gt-dec-caching_strategyabc123 [PENDING]

Question: Which caching strategy should we use?

Options:
  1. Redis (Recommended)
     Distributed, handles scaling, adds operational complexity
  2. In-memory
     Simple and fast, limited to single process

Requested by: beads/crew/decision
Requested at: 5 minutes ago
Urgency: medium

To resolve: gt decision resolve gt-abc123 --choice N --rationale "..."
```

Note: Decisions are assigned a **semantic slug** (e.g., `gt-dec-caching_strategyabc123`) derived from the prompt text. This makes decisions easy to identify in Slack notifications and logs. Use clear, descriptive prompts to generate meaningful slugs.

### Resolving Decisions

```bash
# Choose option 1
gt decision resolve hq-abc123 --choice 1

# Choose with rationale
gt decision resolve hq-abc123 --choice 2 --rationale "Simpler for MVP"
```

### Interactive Watch TUI

The `gt decision watch` command provides an interactive terminal UI for monitoring and responding to decisions in real-time.

```bash
# Launch the watch TUI
gt decision watch

# Show only high urgency decisions
gt decision watch --urgent-only

# Enable desktop notifications for new decisions
gt decision watch --notify
```

**Features:**
- Real-time view of pending decisions with auto-refresh (every 5 seconds)
- Two-pane layout: decision list (top) and detail view (bottom)
- Color-coded urgency indicators (red/orange/green)
- Keyboard-driven navigation and selection

**Keyboard Shortcuts:**

| Key | Action |
|-----|--------|
| `j` / `k` or `↑` / `↓` | Navigate between decisions |
| `1` - `4` | Select option by number |
| `Enter` | Confirm selection |
| `r` | Add rationale before confirming |
| `t` | Enter custom text response |
| `R` | Refresh immediately |
| `!` | Filter to high urgency only |
| `a` | Show all urgencies |
| `?` | Toggle help |
| `q` | Quit |

**Workflow:**
1. Launch `gt decision watch`
2. Navigate to a decision with `j`/`k`
3. Review the question and options in the detail pane
4. Press `1`-`4` to select an option
5. Optionally press `r` to add a rationale
6. Press `Enter` to confirm

The TUI provides immediate visual feedback and handles the resolution automatically, making it ideal for humans monitoring multiple agent sessions.

## Per-Turn Decision Enforcement

Gas Town can enforce that agents offer a decision point before ending each turn. This ensures agents regularly check in with humans rather than running autonomously for extended periods.

### How It Works

Three Claude Code hooks work together:

1. **UserPromptSubmit**: Clears the "decision offered" marker when a new turn starts
2. **PostToolUse**: Detects when `gt decision request` or `bd decision create` is called, sets marker
3. **Stop**: Checks for marker; blocks if missing

### Enforcement Modes

| Mode | Behavior |
|------|----------|
| `strict` | Block until formal decision created (default for crew) |
| `soft` | Remind but don't block (default for polecats) |
| `off` | No enforcement |

### Configuration

Hook scripts are installed in `~/.claude/hooks/`:

```bash
~/.claude/hooks/decision-post-tool.sh  # PostToolUse handler
~/.claude/hooks/decision-stop.sh       # Stop hook
```

Settings in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [{
      "hooks": [{
        "command": "rm -f /tmp/.decision-offered-* 2>/dev/null; exit 0",
        "type": "command"
      }]
    }],
    "PostToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "command": "~/.claude/hooks/decision-post-tool.sh",
        "type": "command"
      }]
    }],
    "Stop": [{
      "hooks": [{
        "command": "~/.claude/hooks/decision-stop.sh",
        "type": "command"
      }]
    }]
  }
}
```

### What Happens When Blocked

When an agent tries to end a turn without offering a decision:

```
● Hello! Ready to help.

● Ran 1 stop hook
  ⎿  Stop hook error: You must offer a formal decision point using
     'gt decision request' before ending this turn.

     When the decision is created, it will be assigned a semantic slug
     (e.g., gt-dec-cache_strategyzfyl8) that makes it easy to identify
     in Slack and logs. Use clear, descriptive prompts so the generated
     slug is meaningful.

● [Agent creates a decision with a descriptive prompt]

● Bash(gt decision request --prompt "Which caching strategy?" ...)
  ⎿  📋 Decision requested: gt-dec-caching_strategyabc123

● Decision point created. Waiting for your choice.
```

The agent is forced to use the formal decision system, ensuring:
- Decisions are tracked in beads
- Humans are notified
- Options are structured and clear

## Data Model

Decisions are stored in the `decision_points` table:

```sql
CREATE TABLE decision_points (
    issue_id TEXT PRIMARY KEY,        -- Links to gate issue
    prompt TEXT NOT NULL,             -- The question
    options TEXT NOT NULL,            -- JSON array of options
    default_option TEXT,              -- Timeout fallback
    selected_option TEXT,             -- Human's choice
    response_text TEXT,               -- Custom text input
    responded_at DATETIME,
    responded_by TEXT,
    iteration INTEGER DEFAULT 1,      -- Refinement iteration
    max_iterations INTEGER DEFAULT 3,
    prior_id TEXT,                    -- Previous iteration
    guidance TEXT,                    -- Text that triggered iteration
    requested_by TEXT,                -- Agent that created it
    created_at DATETIME
);
```

Each option has:
- `id`: Short identifier (e.g., "a", "redis")
- `short`: 1-3 word summary for compact display
- `label`: Sentence-length description
- `description`: Optional rich markdown content

## Semantic Slugs

Decisions are assigned human-readable **semantic slugs** that make them easy to identify in Slack, logs, and CLI output.

### Format

```
<prefix>-dec-<title_slug><random>
```

Examples:
- `gt-dec-caching_strategyabc123` - From prompt "Which caching strategy?"
- `gt-dec-api_design_approachxyz789` - From prompt "API design approach?"
- `gt-dec-production_deploymentp1q2r3` - From prompt "Approve production deployment?"

### Slug Generation Rules

1. The prompt text is slugified:
   - Converted to lowercase
   - Stop words removed (a, an, the, which, should, etc.)
   - Non-alphanumeric replaced with underscores
   - Truncated to 40 characters at word boundary

2. The type code `dec` identifies it as a decision

3. The random component ensures uniqueness

### Best Practice

Use **clear, descriptive prompts** so the generated slug is meaningful:

**Good prompts (generate useful slugs):**
- "Which caching strategy for the API layer?" → `gt-dec-caching_strategy_api_layer...`
- "Database choice for user data?" → `gt-dec-database_choice_user_data...`

**Poor prompts (generate unhelpful slugs):**
- "What should we do?" → `gt-dec-xxx...` (too vague)
- "Yes or no?" → `gt-dec-yes_no...` (not descriptive)

## Iterative Refinement

When a human provides text guidance instead of selecting an option, the agent can create a refined decision:

```
Iteration 1: "Which database?"
  Options: PostgreSQL, MySQL, SQLite
  Human: "We need something that works offline"
        ↓
Iteration 2: "Offline-capable database?"
  Options: SQLite (local file), PouchDB (sync), Realm
  Human: Selects "SQLite"
        ↓
Decision resolved
```

Each iteration is linked via `prior_id`, creating an audit trail of the decision process.

## Integration with Beads

Decisions are first-class beads with type `gate` and `await_type = "decision"`. This means:

- They appear in `bd list` and `bd ready`
- They can block other beads via dependencies
- They're tracked in the beads sync and export
- They have full audit trail via events

## Best Practices

### When to Offer Decisions

1. **Completing research/design** - Before implementing
2. **Multiple valid approaches** - Architectural forks
3. **End of work session** - What was done, what's next
4. **Scope ambiguity** - Requirements need clarification
5. **Risk points** - Hard-to-reverse actions

### Crafting Quality Options

**Don't:**
```
--option "Yes"
--option "No"
```

**Do:**
```
--option "Redis: Distributed caching, handles scaling, adds ops complexity"
--option "In-memory: Simple, fast, single-process only"
--option "Defer: No caching until bottleneck proven"
```

Each option should:
- Have a clear label
- Explain the tradeoff
- Enable informed choice

### Urgency Levels

| Level | When to Use |
|-------|-------------|
| `high` | Blocking critical path, needs response within hours |
| `medium` | Standard decision, response within a day (default) |
| `low` | Can wait, informational, nice-to-have input |

## Decision Lifecycle

### Auto-close Stale Decisions

Decisions that go unanswered are automatically closed to prevent clutter:

```bash
# Auto-close decisions older than 10 minutes (default)
gt decision auto-close

# Preview what would be closed
gt decision auto-close --dry-run

# Custom threshold
gt decision auto-close --threshold 30m

# For hooks (outputs system-reminder)
gt decision auto-close --inject
```

The auto-close command runs as part of the `UserPromptSubmit` hook, cleaning up
stale decisions before each agent turn.

### Single Decision Rule

Each agent can have only one pending decision at a time. When a new decision is
created, any existing pending decisions from the same agent are automatically
closed as "superseded".

This enforces clean decision workflows and prevents decision pile-up.

### Custom Text Responses ("Other")

When none of the predefined options fit, users can provide a custom text response:

**Via Slack:**
1. Click the "Other" button on the decision notification
2. Enter your response in the modal
3. Submit - the decision is resolved with your custom text

**Via TUI:**
1. Navigate to the decision
2. Press `t` to enter text mode
3. Type your response and submit

**Via CLI:**
```bash
bd decision respond <id> --text="Your custom response" --accept-guidance
```

Custom text responses are marked with the `implicit:custom_text` label for tracking.

## Related Commands

| Command | Purpose |
|---------|---------|
| `gt decision request` | Create a decision point |
| `gt decision list` | List pending decisions |
| `gt decision show <id>` | View decision details |
| `gt decision resolve <id>` | Respond to a decision |
| `gt decision cancel <id>` | Cancel/dismiss a decision |
| `gt decision watch` | Interactive TUI for monitoring/responding |
| `gt decision dashboard` | Summary view by urgency |
| `gt decision await <id>` | Block until resolved (scripting) |
| `gt decision auto-close` | Clean up stale decisions (for hooks) |
| `bd decision create` | Low-level decision creation |
| `bd decision respond` | Low-level response recording |

## See Also

- [Beads Overview](overview.md) - Core beads concepts
- [Gates and Blocking](concepts/gates.md) - Gate mechanism
- [Claude Code Hooks](https://docs.anthropic.com/en/docs/claude-code/hooks) - Hook system

---

*Decision Points were introduced in Gas Town to ensure agents maintain human alignment during autonomous operation. They transform implicit assumptions into explicit choices.*
