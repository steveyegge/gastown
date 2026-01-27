# Formula Execution Observer

> Design for observing formula/molecule execution in real-time and post-hoc

## Overview

This document describes the Formula Execution Observer system, which captures
detailed execution traces of molecule workflows. Building on the [Session as
First-Class Object](session-first-class.md) foundation, this system enables:

1. Step-by-step execution logging
2. Input/output capture per step
3. Timing metrics
4. Error/exception capture with context
5. Tool call logging

## Background

### Current State

Formula execution is implicit:
- Steps are closed via `bd close <step-id>`
- No structured capture of what happened during step execution
- Errors are only visible in tmux scrollback (ephemeral)
- No comparison across executions

### Why This Matters

- **Debugging**: Why did step X fail? What was the context?
- **Optimization**: Which steps take longest? Where are bottlenecks?
- **Learning**: What patterns lead to success vs failure?
- **Cost tracking**: How many tool calls per step? Token usage?
- **Replay**: Can we reproduce a successful execution?

## Requirements

| Requirement | Description |
|-------------|-------------|
| **Step Logging** | Record start/end of each step with timing |
| **I/O Capture** | Capture inputs (step instructions, context) and outputs (deliverables, artifacts) |
| **Timing** | Wall-clock duration, cumulative step time |
| **Error Context** | Capture error messages, stack traces, retry attempts |
| **Tool Logging** | Record tool invocations with arguments and results |
| **Session Link** | Connect traces to session objects for full context |
| **Queryability** | Find executions by formula, outcome, time range, agent |

## Data Model

### Execution Trace

The top-level object representing a complete formula execution.

```go
// ExecutionTrace captures a complete formula/molecule execution.
type ExecutionTrace struct {
    // Identity
    ID          string    `json:"id"`           // UUID for this execution
    SessionID   string    `json:"session_id"`   // Link to Session object
    ChainID     string    `json:"chain_id"`     // Work chain (from session)

    // Formula Context
    FormulaID   string    `json:"formula_id"`   // Source formula (e.g., "mol-polecat-work")
    MoleculeID  string    `json:"molecule_id"`  // Instantiated molecule bead ID
    WorkUnit    string    `json:"work_unit"`    // Root issue being worked

    // Agent Context
    Agent       string    `json:"agent"`        // e.g., "gastown/polecats/Toast"
    Rig         string    `json:"rig"`          // e.g., "gastown"

    // Timing
    StartedAt   time.Time `json:"started_at"`
    EndedAt     time.Time `json:"ended_at,omitempty"`
    DurationMs  int64     `json:"duration_ms,omitempty"`

    // Outcome
    Status      string    `json:"status"`       // "running", "completed", "failed", "abandoned"
    ExitReason  string    `json:"exit_reason,omitempty"` // Why execution ended

    // Step Summary
    TotalSteps  int       `json:"total_steps"`
    StepsDone   int       `json:"steps_done"`
    StepsFailed int       `json:"steps_failed"`

    // Cost Metrics (optional, populated if available)
    TotalTokens int64     `json:"total_tokens,omitempty"`  // Total tokens used
    ToolCalls   int       `json:"tool_calls,omitempty"`    // Total tool invocations
}
```

### Step Execution

Per-step execution data.

```go
// StepExecution captures execution of a single molecule step.
type StepExecution struct {
    // Identity
    ID          string    `json:"id"`           // UUID for this step execution
    TraceID     string    `json:"trace_id"`     // Parent execution trace
    StepID      string    `json:"step_id"`      // Bead ID of the step (e.g., "gt-abc.1")
    StepRef     string    `json:"step_ref"`     // Step reference from formula

    // Sequence
    StepNumber  int       `json:"step_number"`  // 1-indexed position in execution
    Tier        string    `json:"tier,omitempty"` // haiku/sonnet/opus if specified

    // Input Context
    Instructions string   `json:"instructions"` // Step instructions (from formula)
    Context     string    `json:"context,omitempty"` // Additional context provided

    // Timing
    StartedAt   time.Time `json:"started_at"`
    EndedAt     time.Time `json:"ended_at,omitempty"`
    DurationMs  int64     `json:"duration_ms,omitempty"`

    // Outcome
    Status      string    `json:"status"`       // "pending", "running", "completed", "failed", "skipped"
    Output      string    `json:"output,omitempty"` // Step deliverable/result summary
    ErrorMsg    string    `json:"error_msg,omitempty"`
    ErrorType   string    `json:"error_type,omitempty"` // "tool_error", "timeout", "validation", etc.
    RetryCount  int       `json:"retry_count,omitempty"`

    // Tool Usage
    ToolCalls   int       `json:"tool_calls,omitempty"`
    TokensUsed  int64     `json:"tokens_used,omitempty"`

    // Artifacts
    Commits     []string  `json:"commits,omitempty"`     // Git commits made
    FilesChanged []string `json:"files_changed,omitempty"` // Files modified
}
```

### Tool Call Record

Individual tool invocation within a step.

```go
// ToolCall records a single tool invocation.
type ToolCall struct {
    // Identity
    ID          string    `json:"id"`           // UUID for this call
    StepExecID  string    `json:"step_exec_id"` // Parent step execution
    TraceID     string    `json:"trace_id"`     // Root trace ID

    // Tool Info
    ToolName    string    `json:"tool_name"`    // e.g., "Read", "Edit", "Bash"
    Category    string    `json:"category"`     // "file", "search", "execute", "communicate"

    // Invocation
    Timestamp   time.Time `json:"timestamp"`
    Arguments   string    `json:"arguments,omitempty"` // JSON-encoded arguments (truncated)
    ArgSummary  string    `json:"arg_summary,omitempty"` // Human-readable summary

    // Result
    DurationMs  int64     `json:"duration_ms,omitempty"`
    Success     bool      `json:"success"`
    ResultSize  int       `json:"result_size,omitempty"` // Bytes of result
    ErrorMsg    string    `json:"error_msg,omitempty"`

    // Cost (if tracked)
    TokensInput  int64    `json:"tokens_input,omitempty"`
    TokensOutput int64    `json:"tokens_output,omitempty"`
}
```

### Execution Status States

```
    ┌───────────┐
    │  pending  │ ─── molecule attached but not started
    └─────┬─────┘
          │
          ▼
    ┌───────────┐
    │  running  │ ─── actively executing steps
    └─────┬─────┘
          │
          ├───────────────────────────────────────┐
          │                                       │
          ▼                                       ▼
    ┌───────────┐                           ┌───────────┐
    │ completed │ ─── all steps done        │  failed   │ ─── step failed
    └───────────┘                           └───────────┘
                                                  │
                                                  ▼
                                            ┌───────────┐
                                            │ abandoned │ ─── gave up/escalated
                                            └───────────┘
```

## Storage Mechanism

### Design: Events + Index (Following Session Pattern)

Execution traces follow the same pattern as sessions:

1. **Events log** - Detailed step/tool events appended to audit log
2. **Trace index** - Fast lookup for active/recent traces
3. **Trace files** - Full execution details for analysis

### File Structure

```
~/gt/
├── .events.jsonl           # Existing (extended with execution events)
├── .sessions/              # Session state (from session design)
│   └── ...
├── .traces/                # NEW: Execution traces
│   ├── index.jsonl         # Trace index (fast scan)
│   ├── active/             # Currently running traces
│   │   └── <trace-id>.json
│   ├── completed/          # Finished traces
│   │   └── <trace-id>.json
│   └── archive/            # Old traces (optional rotation)
└── ...
```

### Trace Index Format

```jsonl
{"id":"trace-123","molecule_id":"gt-abc","agent":"gastown/polecats/Toast","status":"running","started_at":"2026-01-17T01:00:00Z"}
{"id":"trace-456","molecule_id":"gt-def","agent":"gastown/polecats/Toast","status":"completed","started_at":"2026-01-17T00:00:00Z","duration_ms":300000}
```

### Event Types

New event types for the events log:

```go
const (
    // Execution trace events
    TypeTraceStarted     = "trace_started"
    TypeTraceCompleted   = "trace_completed"
    TypeTraceFailed      = "trace_failed"

    // Step execution events
    TypeStepStarted      = "step_started"
    TypeStepCompleted    = "step_completed"
    TypeStepFailed       = "step_failed"
    TypeStepSkipped      = "step_skipped"

    // Tool call events (audit-only by default)
    TypeToolInvoked      = "tool_invoked"
    TypeToolCompleted    = "tool_completed"
    TypeToolFailed       = "tool_failed"
)
```

### Why Separate from Sessions?

Sessions and traces capture different concerns:

| Sessions | Traces |
|----------|--------|
| Agent lifecycle (spawn → handoff) | Work execution (formula steps) |
| One session may span multiple traces | One trace belongs to one session |
| Links via `chain_id` across handoffs | Links via `session_id` to context |
| Tracks what agent did | Tracks how work progressed |

A session might start a molecule, hand off mid-execution, and the next session
continues. The trace captures the full execution across session boundaries via
the `chain_id`.

## API Design

### Package: `internal/traces`

```go
package traces

// Manager handles execution trace lifecycle and querying.
type Manager struct {
    townRoot string
    sessions *sessions.Manager // Link to session manager
}

// StartTrace begins recording a new execution trace.
func (m *Manager) StartTrace(opts StartTraceOptions) (*ExecutionTrace, error)

// EndTrace completes a trace with final status.
func (m *Manager) EndTrace(traceID string, outcome Outcome) error

// StartStep begins recording a step execution.
func (m *Manager) StartStep(traceID string, opts StartStepOptions) (*StepExecution, error)

// EndStep completes a step with outcome.
func (m *Manager) EndStep(stepExecID string, outcome StepOutcome) error

// RecordToolCall logs a tool invocation.
func (m *Manager) RecordToolCall(stepExecID string, call ToolCall) error

// Get retrieves a trace by ID.
func (m *Manager) Get(traceID string) (*ExecutionTrace, error)

// GetSteps retrieves all step executions for a trace.
func (m *Manager) GetSteps(traceID string) ([]*StepExecution, error)

// Query finds traces matching criteria.
func (m *Manager) Query(q TraceQuery) ([]*ExecutionTrace, error)
```

### Query Interface

```go
type TraceQuery struct {
    FormulaID  string     // Filter by formula
    MoleculeID string     // Filter by molecule instance
    Agent      string     // Filter by agent
    WorkUnit   string     // Filter by root issue
    ChainID    string     // Filter by work chain
    Status     string     // Filter by outcome
    Since      time.Time  // Traces after this time
    Until      time.Time  // Traces before this time
    Limit      int        // Max results
}

type StepQuery struct {
    TraceID    string     // Required: parent trace
    Status     string     // Filter by outcome
    HasError   bool       // Only steps with errors
    MinDuration int64     // Steps taking longer than N ms
}
```

### CLI Commands

```bash
# List recent execution traces
gt trace list [--agent=<addr>] [--formula=<id>] [--since=1h]

# Show trace details with step summary
gt trace show <trace-id>

# Show detailed step execution
gt trace steps <trace-id> [--verbose]

# Show tool calls for a step
gt trace tools <step-exec-id>

# Analyze trace for insights
gt trace analyze <trace-id>

# Compare two traces
gt trace diff <trace-id-1> <trace-id-2>

# Find failed steps across traces
gt trace failures [--formula=<id>] [--since=24h]
```

## Integration Points

### 1. Trace Start (molecule attach)

**Location:** `cmd/molecule_attach.go`

```go
// After attaching molecule, start trace
trace, _ := traces.StartTrace(traces.StartTraceOptions{
    SessionID:  os.Getenv("GT_SESSION_UUID"),
    FormulaID:  formula.Name,
    MoleculeID: mol.ID,
    WorkUnit:   hookedBead.ID,
    Agent:      agentAddress,
})
// Set GT_TRACE_ID in environment
os.Setenv("GT_TRACE_ID", trace.ID)
```

### 2. Step Start (claiming step)

**Location:** `cmd/molecule_step.go` or wherever steps are claimed

```go
// When marking step in_progress
step, _ := traces.StartStep(os.Getenv("GT_TRACE_ID"), traces.StartStepOptions{
    StepID:       stepBead.ID,
    StepRef:      stepBead.Ref,
    StepNumber:   n,
    Instructions: stepBead.Description,
    Tier:         stepBead.Tier,
})
os.Setenv("GT_STEP_EXEC_ID", step.ID)
```

### 3. Step Completion

**Location:** `cmd/molecule_step.go` (step done command)

```go
// When closing step
traces.EndStep(os.Getenv("GT_STEP_EXEC_ID"), traces.StepOutcome{
    Status:       "completed",
    Output:       deliverableSummary,
    Commits:      getNewCommits(),
    FilesChanged: getChangedFiles(),
})
```

### 4. Tool Call Recording

**Location:** Hook or instrumentation layer (implementation detail)

Tool calls are captured via Claude Code hooks or by instrumenting the tool
execution layer. This is lower-level than the step API.

```go
// On tool invocation (pseudo-code)
traces.RecordToolCall(os.Getenv("GT_STEP_EXEC_ID"), traces.ToolCall{
    ToolName:   "Edit",
    Category:   "file",
    Arguments:  truncatedArgs,
    ArgSummary: "Edit src/main.go:42",
    Success:    true,
    DurationMs: elapsed,
})
```

### 5. Trace End (gt done or failure)

**Location:** `cmd/done.go` or error handlers

```go
// On completion
traces.EndTrace(os.Getenv("GT_TRACE_ID"), traces.Outcome{
    Status:     "completed",
    ExitReason: "all steps done",
})

// On failure
traces.EndTrace(os.Getenv("GT_TRACE_ID"), traces.Outcome{
    Status:     "failed",
    ExitReason: "step 3 failed: test failures",
})
```

## Analysis Capabilities

### 1. Step Failure Analysis

```bash
gt trace analyze <trace-id>
```

Output:
```
Execution Analysis: trace-abc123
Formula: mol-polecat-work (6 steps)
Duration: 12m 34s
Status: FAILED at step 4

Step Breakdown:
  ✓ Step 1: Load context (45s) - 12 tool calls
  ✓ Step 2: Branch setup (8s) - 3 tool calls
  ✓ Step 3: Implementation (8m 12s) - 87 tool calls
  ✗ Step 4: Run tests (3m 29s) - 24 tool calls
    Error: Test failures in auth_test.go
    Retries: 2

Bottleneck: Step 3 (Implementation) took 65% of total time.
Failure Pattern: Test failure after implementation - consider TDD.
```

### 2. Cross-Execution Comparison

```bash
gt trace diff trace-123 trace-456
```

Output:
```
Comparing traces for formula: mol-polecat-work

              trace-123 (failed)    trace-456 (completed)
Duration      12m 34s               9m 12s
Steps Done    3/6                   6/6
Tool Calls    126                   98

Step Differences:
  Step 3 (Implementation):
    - trace-123: 8m 12s, 87 tools
    + trace-456: 5m 45s, 62 tools
    Notable: trace-456 used Read more, Edit less (better planning?)

  Step 4 (Tests):
    - trace-123: FAILED after 3m 29s
    + trace-456: completed in 2m 1s
```

### 3. Pattern Detection

```bash
gt trace failures --formula=mol-polecat-work --since=7d
```

Output:
```
Failure Patterns for mol-polecat-work (last 7 days)

12 traces analyzed, 4 failures (33% failure rate)

Common Failure Points:
  Step 4 (Run tests): 3 failures
    - Test assertion failures (2)
    - Timeout (1)

  Step 6 (Exit decision): 1 failure
    - Missing branch push

Recommendations:
  1. Step 4 failures suggest TDD approach might reduce iterations
  2. Consider adding pre-test validation step
```

## Environment Variables

| Variable | Set By | Used By |
|----------|--------|---------|
| `GT_TRACE_ID` | `gt mol attach` | Step/tool recording |
| `GT_STEP_EXEC_ID` | Step start | Tool recording |
| `GT_SESSION_UUID` | Session start | Trace linking |

These compose: Session → Trace → Step → Tool calls

## Migration Path

### Phase 1: Core Tracing

1. Create `internal/traces` package
2. Instrument molecule attach/step commands
3. Add `gt trace` CLI commands
4. Traces recorded but minimal analysis

### Phase 2: Tool Call Capture

1. Add hook for tool invocations (Claude Code hook integration)
2. Record tool calls with timing
3. Enable per-step tool breakdown

### Phase 3: Analysis Engine

1. Implement cross-trace comparison
2. Add pattern detection
3. Generate actionable recommendations

### Phase 4: Real-Time Streaming (Optional)

1. Add trace event streaming
2. Dashboard integration
3. Live execution monitoring

## Cost/Benefit Analysis

| Cost | Benefit |
|------|---------|
| ~800 lines new code | Full execution visibility |
| Storage: ~5-10KB per trace | Debug failed executions |
| Minimal runtime overhead | Identify optimization opportunities |
| | Learn from success/failure patterns |
| | Foundation for automated improvement |

## Privacy & Size Considerations

### Data Minimization

- **Arguments**: Truncated to first 500 chars, no secrets
- **Output**: Summary only, not full content
- **Tool results**: Size recorded, not content

### Retention

- Active traces: Keep until completion
- Completed traces: 30 days default
- Archived traces: Configurable (optional export)

```bash
# Configure retention
gt config set trace.retention.days 30
gt config set trace.archive.enabled true
```

## Open Questions

1. **Tool call granularity** - Capture every tool call, or sample?
2. **Token tracking** - How to get accurate token counts from Claude Code?
3. **Cross-machine traces** - If work spans machines, how to correlate?
4. **Storage limits** - Should we auto-prune old traces?

## Related Documentation

- [Session as First-Class Object](session-first-class.md) - Session data model
- [Molecules](../concepts/molecules.md) - Formula/molecule concepts
- [Architecture](architecture.md) - Overall Gas Town architecture
