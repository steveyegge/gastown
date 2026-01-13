# Dog Pool Architecture for Concurrent Shutdown Dances

> Design document for gt-fsld8

## Problem Statement

Boot needs to run multiple shutdown-dance molecules concurrently when multiple death
warrants are issued. The current hook design only allows one molecule per agent.

Example scenario:
- Warrant 1: Kill stuck polecat Toast (60s into interrogation)
- Warrant 2: Kill stuck polecat Shadow (just started)
- Warrant 3: Kill stuck witness (120s into interrogation)

All three need concurrent tracking, independent timeouts, and separate outcomes.

## Design Decision: Lightweight State Machines

After analyzing the options, the shutdown-dance does NOT need Claude sessions.
The dance is a deterministic state machine:

```
WARRANT -> INTERROGATE -> EVALUATE -> PARDON|EXECUTE
```

Each step is mechanical:
1. Send a tmux message (no LLM needed)
2. Wait for timeout or response (timer)
3. Check tmux output for ALIVE keyword (string match)
4. Repeat or terminate

**Decision**: Dogs are lightweight Go routines, not Claude sessions.

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────────┐
│                             BOOT                                    │
│                     (Claude session in tmux)                        │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                      Dog Manager                              │  │
│  │                                                               │  │
│  │   Pool: [Dog1, Dog2, Dog3, ...]  (goroutines + state files)  │  │
│  │                                                               │  │
│  │   allocate() → Dog                                           │  │
│  │   release(Dog)                                               │  │
│  │   status() → []DogStatus                                     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  Boot's job:                                                       │
│  - Watch for warrants (file or event)                              │
│  - Allocate dog from pool                                          │
│  - Monitor dog progress                                            │
│  - Handle dog completion/failure                                   │
│  - Report results                                                  │
└────────────────────────────────────────────────────────────────────┘
```

## Dog Structure

```go
// Dog represents a shutdown-dance executor
type Dog struct {
    ID        string            // Unique ID (e.g., "dog-1704567890123")
    Warrant   *Warrant          // The death warrant being processed
    State     ShutdownDanceState
    Attempt   int               // Current interrogation attempt (1-3)
    StartedAt time.Time
    StateFile string            // Persistent state: ~/gt/deacon/dogs/active/<id>.json
}

type ShutdownDanceState string

const (
    StateIdle          ShutdownDanceState = "idle"
    StateInterrogating ShutdownDanceState = "interrogating"  // Sent message, waiting
    StateEvaluating    ShutdownDanceState = "evaluating"     // Checking response
    StatePardoned      ShutdownDanceState = "pardoned"       // Session responded
    StateExecuting     ShutdownDanceState = "executing"      // Killing session
    StateComplete      ShutdownDanceState = "complete"       // Done, ready for cleanup
    StateFailed        ShutdownDanceState = "failed"         // Dog crashed/errored
)

type Warrant struct {
    ID        string    // Bead ID for the warrant
    Target    string    // Session to interrogate (e.g., "gt-gastown-Toast")
    Reason    string    // Why warrant was issued
    Requester string    // Who filed the warrant
    FiledAt   time.Time
}
```

## Pool Design

### Fixed Pool Size

**Decision**: Fixed pool of 5 dogs, configurable via environment.

Rationale:
- Dynamic sizing adds complexity without clear benefit
- 5 concurrent shutdown dances handles worst-case scenarios
- If pool exhausted, warrants queue (better than infinite dog spawning)
- Memory footprint is negligible (goroutines + small state files)

```go
const (
    DefaultPoolSize = 5
    MaxPoolSize     = 20
)

type DogPool struct {
    mu       sync.Mutex
    dogs     []*Dog           // All dogs in pool
    idle     chan *Dog        // Channel of available dogs
    active   map[string]*Dog  // ID -> Dog for active dogs
    stateDir string           // ~/gt/deacon/dogs/active/
}

func (p *DogPool) Allocate(warrant *Warrant) (*Dog, error) {
    select {
    case dog := <-p.idle:
        dog.Warrant = warrant
        dog.State = StateInterrogating
        dog.Attempt = 1
        dog.StartedAt = time.Now()
        p.active[dog.ID] = dog
        return dog, nil
    default:
        return nil, ErrPoolExhausted
    }
}

func (p *DogPool) Release(dog *Dog) {
    p.mu.Lock()
    defer p.mu.Unlock()
    delete(p.active, dog.ID)
    dog.Reset()
    p.idle <- dog
}
```

### Why Not Dynamic Pool?

Considered but rejected:
- Adding dogs on demand increases complexity
- No clear benefit - warrants rarely exceed 5 concurrent
- If needed, raise DefaultPoolSize
- Simpler to reason about fixed resources

## Communication: State Files + Events

### State Persistence

Each active dog writes state to `~/gt/deacon/dogs/active/<id>.json`:

```json
{
  "id": "dog-1704567890123",
  "warrant": {
    "id": "gt-abc123",
    "target": "gt-gastown-Toast",
    "reason": "no_response_health_check",
    "requester": "deacon",
    "filed_at": "2026-01-07T20:15:00Z"
  },
  "state": "interrogating",
  "attempt": 2,
  "started_at": "2026-01-07T20:15:00Z",
  "last_message_at": "2026-01-07T20:16:00Z",
  "next_timeout": "2026-01-07T20:18:00Z"
}
```

### Boot Monitoring

Boot monitors dogs via:
1. **Polling**: `gt dog status --active` every tick
2. **Completion files**: Dogs write `<id>.done` when complete

```go
type DogResult struct {
    DogID    string
    Warrant  *Warrant
    Outcome  DogOutcome  // pardoned | executed | failed
    Duration time.Duration
    Details  string
}

type DogOutcome string

const (
    OutcomePardoned DogOutcome = "pardoned"  // Session responded
    OutcomeExecuted DogOutcome = "executed"  // Session killed
    OutcomeFailed   DogOutcome = "failed"    // Dog crashed
)
```

### Why Not Mail?

Considered but rejected for dog<->boot communication:
- Mail is async, poll-based - adds latency
- State files are simpler for local coordination
- Dogs don't need complex inter-agent communication
- Keep mail for external coordination (Witness, Mayor)

## Shutdown Dance State Machine

Each dog executes this state machine:

```
                    ┌─────────────────────────────────────────┐
                    │                                         │
                    ▼                                         │
    ┌───────────────────────────┐                            │
    │     INTERROGATING         │                            │
    │                           │                            │
    │  1. Send health check     │                            │
    │  2. Start timeout timer   │                            │
    └───────────┬───────────────┘                            │
                │                                             │
                │ timeout or response                         │
                ▼                                             │
    ┌───────────────────────────┐                            │
    │      EVALUATING           │                            │
    │                           │                            │
    │  Check tmux output for    │                            │
    │  ALIVE keyword            │                            │
    └───────────┬───────────────┘                            │
                │                                             │
        ┌───────┴───────┐                                    │
        │               │                                    │
        ▼               ▼                                    │
   [ALIVE found]   [No ALIVE]                               │
        │               │                                    │
        │               │ attempt < 3?                       │
        │               ├──────────────────────────────────→─┘
        │               │ yes: attempt++, longer timeout
        │               │
        │               │ no: attempt == 3
        ▼               ▼
    ┌─────────┐    ┌─────────────┐
    │ PARDONED│    │  EXECUTING  │
    │         │    │             │
    │ Cancel  │    │ Kill tmux   │
    │ warrant │    │ session     │
    └────┬────┘    └──────┬──────┘
         │                │
         └────────┬───────┘
                  │
                  ▼
         ┌────────────────┐
         │    COMPLETE    │
         │                │
         │  Write result  │
         │  Release dog   │
         └────────────────┘
```

### Timeout Gates

| Attempt | Timeout | Cumulative Wait |
|---------|---------|-----------------|
| 1       | 60s     | 60s             |
| 2       | 120s    | 180s (3 min)    |
| 3       | 240s    | 420s (7 min)    |

### Health Check Message

```
[DOG] HEALTH CHECK: Session {target}, respond ALIVE within {timeout}s or face termination.
Warrant reason: {reason}
Filed by: {requester}
Attempt: {attempt}/3
```

### Response Detection

```go
func (d *Dog) CheckForResponse() bool {
    tm := tmux.NewTmux()
    output, err := tm.CapturePane(d.Warrant.Target, 50) // Last 50 lines
    if err != nil {
        return false
    }

    // Any output after our health check counts as alive
    // Specifically look for ALIVE keyword for explicit response
    return strings.Contains(output, "ALIVE")
}
```

## Dog Implementation

### Not Reusing Polecat Infrastructure

**Decision**: Dogs do NOT reuse polecat infrastructure.

Rationale:
- Polecats are Claude sessions with molecules, hooks, sandboxes
- Dogs are simple state machine executors
- Polecats have 3-layer lifecycle (session/sandbox/slot)
- Dogs have single-layer lifecycle (just state)
- Different resource profiles, different management

What dogs DO share:
- tmux utilities for message sending/capture
- State file patterns
- Name slot allocation pattern (pool of names, not instances)

### Dog Execution Loop

```go
func (d *Dog) Run(ctx context.Context) DogResult {
    d.State = StateInterrogating
    d.saveState()

    for d.Attempt <= 3 {
        // Send interrogation message
        if err := d.sendHealthCheck(); err != nil {
            return d.fail(err)
        }

        // Wait for timeout or context cancellation
        timeout := d.timeoutForAttempt(d.Attempt)
        select {
        case <-ctx.Done():
            return d.fail(ctx.Err())
        case <-time.After(timeout):
            // Timeout reached
        }

        // Evaluate response
        d.State = StateEvaluating
        d.saveState()

        if d.CheckForResponse() {
            // Session is alive
            return d.pardon()
        }

        // No response - try again or execute
        d.Attempt++
        if d.Attempt <= 3 {
            d.State = StateInterrogating
            d.saveState()
        }
    }

    // All attempts exhausted - execute warrant
    return d.execute()
}
```

## Failure Handling

### Dog Crashes Mid-Dance

If a dog crashes (Boot process restarts, system crash):

1. State files persist in `~/gt/deacon/dogs/active/`
2. On Boot restart, scan for orphaned state files
3. Resume or restart based on state:

| State            | Recovery Action                    |
|------------------|------------------------------------|
| interrogating    | Restart from current attempt       |
| evaluating       | Check response, continue           |
| executing        | Verify kill, mark complete         |
| pardoned/complete| Already done, clean up             |

```go
func (p *DogPool) RecoverOrphans() error {
    files, _ := filepath.Glob(p.stateDir + "/*.json")
    for _, f := range files {
        state := loadDogState(f)
        if state.State != StateComplete && state.State != StatePardoned {
            dog := p.allocateForRecovery(state)
            go dog.Resume()
        }
    }
    return nil
}
```

### Handling Pool Exhaustion

If all dogs are busy when new warrant arrives:

```go
func (b *Boot) HandleWarrant(warrant *Warrant) error {
    dog, err := b.pool.Allocate(warrant)
    if err == ErrPoolExhausted {
        // Queue the warrant for later processing
        b.warrantQueue.Push(warrant)
        b.log("Warrant %s queued (pool exhausted)", warrant.ID)
        return nil
    }

    go func() {
        result := dog.Run(b.ctx)
        b.handleResult(result)
        b.pool.Release(dog)

        // Check queue for pending warrants
        if next := b.warrantQueue.Pop(); next != nil {
            b.HandleWarrant(next)
        }
    }()

    return nil
}
```

## Directory Structure

```
~/gt/deacon/dogs/
├── boot/                    # Boot's working directory
│   ├── CLAUDE.md            # Boot context
│   └── .boot-status.json    # Boot execution status
├── active/                  # Active dog state files
│   ├── dog-123.json         # Dog 1 state
│   ├── dog-456.json         # Dog 2 state
│   └── ...
├── completed/               # Completed dance records (for audit)
│   ├── dog-789.json         # Historical record
│   └── ...
└── warrants/                # Pending warrant queue
    ├── warrant-abc.json
    └── ...
```

## Command Interface

```bash
# Pool status
gt dog pool status
# Output:
# Dog Pool: 3/5 active
#   dog-123: interrogating Toast (attempt 2, 45s remaining)
#   dog-456: executing Shadow
#   dog-789: idle

# Manual dog operations (for debugging)
gt dog pool allocate <warrant-id>
gt dog pool release <dog-id>

# View active dances
gt dog dances
# Output:
# Active Shutdown Dances:
#   dog-123 → Toast: Interrogating (2/3), timeout in 45s
#   dog-456 → Shadow: Executing warrant

# View warrant queue
gt dog warrants
# Output:
# Pending Warrants: 2
#   1. gt-abc: witness-gastown (stuck_no_progress)
#   2. gt-def: polecat-Copper (crash_loop)
```

## Integration with Existing Dogs

The existing `dog` package (`internal/dog/`) manages Deacon's multi-rig helper dogs.
Those are different from shutdown-dance dogs:

| Aspect          | Helper Dogs (existing)      | Dance Dogs (new)           |
|-----------------|-----------------------------|-----------------------------|
| Purpose         | Cross-rig infrastructure    | Shutdown dance execution    |
| Sessions        | Claude sessions             | Goroutines (no Claude)      |
| Worktrees       | One per rig                 | None                        |
| Lifecycle       | Long-lived, reusable        | Ephemeral per warrant       |
| State           | idle/working                | Dance state machine         |

**Recommendation**: Use different package to avoid confusion:
- `internal/dog/` - existing helper dogs
- `internal/shutdown/` - shutdown dance pool

## Summary: Answers to Design Questions

| Question | Answer |
|----------|--------|
| How many Dogs in pool? | Fixed: 5 (configurable via GT_DOG_POOL_SIZE) |
| How do Dogs communicate with Boot? | State files + completion markers |
| Are Dogs tmux sessions? | No - goroutines with state machine |
| Reuse polecat infrastructure? | No - too heavyweight, different model |
| What if Dog dies mid-dance? | State file recovery on Boot restart |

## Acceptance Criteria

- [x] Architecture document for Dog pool
- [x] Clear allocation/deallocation protocol
- [x] Failure handling for Dog crashes
