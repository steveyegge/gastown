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
- Rich context including analysis and tradeoffs
- Urgency level (high/medium/low)

**Rich Decision Fields:**
| Field | Purpose |
|-------|---------|
| `--prompt` | The decision question (required) |
| `--context` | Brief background information |
| `--analysis` | Detailed analysis of the situation (paragraph-length encouraged) |
| `--tradeoffs` | General discussion of tradeoffs between approaches |
| `--option` | An option with "Label: Description" format (2-4 required) |
| `--pro` | Pros for an option in "N:text" format (e.g., "1:Fast implementation") |
| `--con` | Cons for an option in "N:text" format (e.g., "1:Higher maintenance") |
| `--recommend` | Mark option N as recommended |
| `--recommend-rationale` | Why the recommended option is suggested |

The human can:
1. **Select an option** - Choose one of the presented options
2. **Provide text guidance** - Give custom instructions (triggers refinement)
3. **Accept as-is** - Proceed with agent's recommendation (after iteration 1)

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

# Rich decision with analysis and tradeoffs
gt decision request \
  --prompt "Which database for the new microservice?" \
  --context "Building a read-heavy service for user profiles" \
  --analysis "Current traffic is ~1000 req/day but expected to grow 10x. \
    Query patterns are mostly key-value lookups with occasional \
    joins for admin dashboards." \
  --tradeoffs "Speed vs flexibility is the key consideration. \
    Managed services reduce ops burden but increase costs." \
  --option "PostgreSQL: Full-featured relational database" \
  --option "Redis: In-memory key-value store" \
  --option "DynamoDB: Managed NoSQL with auto-scaling" \
  --pro "1:Excellent query flexibility" \
  --pro "1:Strong ecosystem" \
  --con "1:Requires dedicated server" \
  --pro "2:Sub-millisecond reads" \
  --pro "2:Great for caching layer" \
  --con "2:No complex queries" \
  --con "2:Data persistence concerns" \
  --pro "3:Zero-ops auto-scaling" \
  --con "3:Vendor lock-in" \
  --con "3:Higher per-request cost" \
  --recommend 1 \
  --recommend-rationale "PostgreSQL balances flexibility with performance. \
    The query patterns suggest we'll eventually need joins, and the \
    read-heavy workload can be optimized with read replicas."

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
🟢 Decision: hq-abc123 [PENDING]

Question: Which caching strategy should we use?

Options:
  1. Redis (Recommended)
     Distributed, handles scaling, adds operational complexity
  2. In-memory
     Simple and fast, limited to single process

Requested by: beads/crew/decision
Requested at: 5 minutes ago
Urgency: medium

To resolve: gt decision resolve hq-abc123 --choice N --rationale "..."
```

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
2. **PostToolUse**: Detects when `gt decision request` is called, sets marker
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

● [Agent learns the API and creates a decision]

● Bash(gt decision request --prompt "What would you like to do?" ...)
  ⎿  📋 Decision requested: hq-abc123

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

### Crafting Rich Decisions (LLM Guidance)

When creating decision points, **invest in rich context**. Humans make better decisions when they understand the full picture. Use all available fields:

**Analysis**: Provide detailed background. Don't just say "we need a database" - explain the query patterns, expected load, data relationships, and constraints you've discovered. Paragraph-length analysis is encouraged.

**Tradeoffs**: Discuss the general tensions at play. Speed vs. flexibility? Simplicity vs. scalability? Short-term vs. long-term? Help the human understand the shape of the decision space.

**Per-Option Pros/Cons**: Don't just describe options - evaluate them. Each option should have concrete pros and cons based on your analysis. This shows you've thought through the implications.

**Recommendation Rationale**: If you recommend an option, explain why. What factors drove your recommendation? This helps humans either agree with your reasoning or identify where their priorities differ.

### Option Quality

**Don't:**
```
--option "Yes"
--option "No"
```

**Do:**
```bash
gt decision request \
  --prompt "Add caching layer to API?" \
  --analysis "Profiling shows 60% of API time is database queries. \
    Most queries are repeatable user lookups. P95 latency is 400ms, \
    target is <100ms." \
  --tradeoffs "Caching reduces latency but adds complexity. Must handle \
    cache invalidation on user updates." \
  --option "Redis: Distributed caching" \
  --option "In-memory: Process-local cache" \
  --option "Defer: Optimize queries first" \
  --pro "1:Handles horizontal scaling" \
  --pro "1:Persistent across restarts" \
  --con "1:Additional infrastructure" \
  --pro "2:Zero external dependencies" \
  --pro "2:Fastest possible reads" \
  --con "2:Lost on process restart" \
  --con "2:Memory pressure under load" \
  --pro "3:Simpler architecture" \
  --pro "3:May be sufficient" \
  --con "3:Delays latency improvement" \
  --recommend 2 \
  --recommend-rationale "For current scale, in-memory is sufficient. \
    Single-process deployment means no cache coherence issues. \
    Can migrate to Redis when we scale horizontally."
```

Each option should:
- Have a clear label
- Include specific pros and cons
- Enable informed choice

### Urgency Levels

| Level | When to Use |
|-------|-------------|
| `high` | Blocking critical path, needs response within hours |
| `medium` | Standard decision, response within a day (default) |
| `low` | Can wait, informational, nice-to-have input |

## Related Commands

| Command | Purpose |
|---------|---------|
| `gt decision request` | Create a decision point |
| `gt decision list` | List pending decisions |
| `gt decision show <id>` | View decision details |
| `gt decision resolve <id>` | Respond to a decision |
| `gt decision watch` | Interactive TUI for monitoring/responding |
| `gt decision dashboard` | Summary view by urgency |
| `gt decision await <id>` | Block until resolved (scripting) |
| `bd decision create` | Low-level primitive (for hooks/scripts only, not agent use) |
| `bd decision respond` | Low-level response recording |

## See Also

- [Beads Overview](overview.md) - Core beads concepts
- [Gates and Blocking](concepts/gates.md) - Gate mechanism
- [Claude Code Hooks](https://docs.anthropic.com/en/docs/claude-code/hooks) - Hook system

---

*Decision Points were introduced in Gas Town to ensure agents maintain human alignment during autonomous operation. They transform implicit assumptions into explicit choices.*
