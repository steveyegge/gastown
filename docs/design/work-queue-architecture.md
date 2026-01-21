# Work Queue Architecture

Technical architecture for the work queue and dispatcher system in Gas Town.

## Overview

The work queue system provides **capacity-controlled dispatch** of beads to polecats. It decouples work submission from execution, enabling you to queue many beads and dispatch them in controlled batches.

### The Problem: Resource Constraints

Each polecat is a Claude Code session running in a tmux window. Running too many polecats simultaneously creates resource pressure:

- **API rate limits** - Claude API has per-account rate limits
- **Memory usage** - Each Claude session consumes significant memory
- **CPU contention** - Multiple active sessions compete for CPU
- **Context switching** - Human operators can only effectively monitor a limited number of concurrent workers

Without capacity control, `gt sling --batch` on 50 beads would spawn 50 polecats simultaneously, likely overwhelming the system.

### The Solution: Queue-Based Dispatch

The work queue provides:

1. **Capacity limits** - Configure maximum concurrent polecats (`queue_max_polecats`)
2. **Deferred execution** - Queue beads now, dispatch when capacity is available
3. **Controlled parallelism** - Spawn N polecats at a time (`polecat_spawn_batch_size`)
4. **Persistent queue** - Queue membership survives restarts (stored as bead labels)

**Typical workflow:**
```bash
# Queue 50 beads for later dispatch
gt sling mol-review --batch --on @beads.txt gastown --queue

# Dispatch up to 10 at a time (respecting capacity)
gt queue run --queue-max-polecats 10

# Later, as polecats complete, dispatch more
gt queue run --queue-max-polecats 10
```

## Architecture Diagram

```
                              ┌───────────────────────────────────────┐
                              │             Town Root                 │
                              │                                       │
                              │  ┌─────────────────────────────────┐  │
                              │  │     settings/config.json        │  │
                              │  │  - queue_max_polecats           │  │
                              │  │  - polecat_spawn_batch_size     │  │
                              │  └─────────────────────────────────┘  │
                              └─────────────────┬─────────────────────┘
                                                │
                                                │ config
                                                ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                                 Queue                                     │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                        BeadsOps Interface                           │  │
│  │                                                                     │  │
│  │   IsTownLevelBead(beadID) bool                                     │  │
│  │   GetRigForBead(beadID) string                                     │  │
│  │   LabelAdd(beadID, label) error                                    │  │
│  │   LabelRemove(beadID, label) error                                 │  │
│  │   ListByLabelAllRigs(label) (map[string][]BeadInfo, error)         │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                    │                                      │
│                                    │ implements                           │
│                                    ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                         RealBeadsOps                                │  │
│  │                                                                     │  │
│  │   townRoot string                                                   │  │
│  │   runBd(dir, args...) error          # Shells out to `bd` CLI      │  │
│  │   listBeads(dir, status, label)      # bd list --json              │  │
│  │   getAllRigs() map[string]string     # Reads mayor/rigs.json       │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                            Queue                                    │  │
│  │                                                                     │  │
│  │   ops   BeadsOps                                                    │  │
│  │   items []QueueItem                                                 │  │
│  │                                                                     │  │
│  │   Add(beadID) error      # Validates + adds "queued" label         │  │
│  │   Load() ([]QueueItem)   # Queries all rigs for queued beads       │  │
│  │   Remove(beadID) error   # Removes "queued" label                  │  │
│  │   Clear() (int, error)   # Removes all beads from queue            │  │
│  │   All() []QueueItem      # Returns current items                   │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                                │
                                                │ queue
                                                ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                               Dispatcher                                  │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                         Dispatcher                                  │  │
│  │                                                                     │  │
│  │   queue       *Queue                                                │  │
│  │   spawner     Spawner                                               │  │
│  │   dryRun      bool                                                  │  │
│  │   limit       int          # Max items to dispatch (0 = unlimited)  │  │
│  │   parallelism int          # Concurrent dispatches (default 1)      │  │
│  │                                                                     │  │
│  │   Dispatch() (*DispatchResult, error)                               │  │
│  │   WithDryRun(bool) *Dispatcher                                      │  │
│  │   WithLimit(int) *Dispatcher                                        │  │
│  │   WithParallelism(int) *Dispatcher                                  │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                    │                                      │
│                                    │ spawner                              │
│                                    ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                       Spawner Interface                             │  │
│  │                                                                     │  │
│  │   SpawnIn(rigName, beadID string) error                             │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                    │                                      │
│                                    │ implements                           │
│                                    ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                         RealSpawner                                 │  │
│  │                                                                     │  │
│  │   SpawnInFunc func(rigName, beadID string) error                    │  │
│  │                                                                     │  │
│  │   # Actual implementation creates polecat worktree, hooks bead,     │  │
│  │   # starts tmux session, and wakes rig agents                       │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                                │
                                                │ parallel.Execute
                                                ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                            Parallel Executor                              │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                     parallel.Execute[T]                             │  │
│  │                                                                     │  │
│  │   items       []T                                                   │  │
│  │   parallelism int                                                   │  │
│  │   work        WorkFunc[T]                                           │  │
│  │                                                                     │  │
│  │   Returns []Result[T] in original order                             │  │
│  │                                                                     │  │
│  │   # Uses worker pool pattern with N goroutines                      │  │
│  │   # Job channel distributes work items by index                     │  │
│  │   # Results collected into slice preserving order                   │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                                │
                                                │ spawns
                                                ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                                Polecats                                   │
│                                                                           │
│   ~/gt/<rig>/polecats/<name>/                                             │
│       └── rig/                    # Git worktree from mayor/rig           │
│                                                                           │
│   Each polecat is a Claude Code session in a tmux window                  │
│   hooked to a bead for execution                                          │
└───────────────────────────────────────────────────────────────────────────┘
```

## Component Descriptions

### Queue

The `Queue` struct manages work items pending dispatch. Queue membership is persisted via the `queued` label on beads, ensuring the queue survives process restarts.

```go
// internal/queue/queue.go

type QueueItem struct {
    BeadID  string  // Bead ID (e.g., "gt-abc123")
    Title   string  // Bead title for display
    RigName string  // Target rig for dispatch
}

type Queue struct {
    ops   beads.BeadsOps  // Abstraction over beads CLI
    items []QueueItem     // In-memory cache
}
```

**Key behaviors:**
- `Add(beadID)` - Validates the bead (rejects town-level beads, requires routable prefix), then adds the "queued" label
- `Load()` - Queries all rigs for beads with the "queued" label and status "open"
- `Remove(beadID)` - Removes the "queued" label from a bead
- `Clear()` - Removes all items from the queue (best-effort)

### BeadsOps Interface

The `BeadsOps` interface abstracts beads operations for testability. The real implementation shells out to the `bd` CLI; tests can provide a fake in-memory implementation.

```go
// internal/beads/beads_ops.go

type BeadsOps interface {
    IsTownLevelBead(beadID string) bool
    GetRigForBead(beadID string) string
    LabelAdd(beadID, label string) error
    LabelRemove(beadID, label string) error
    ListByLabelAllRigs(label string) (map[string][]BeadInfo, error)
}
```

The `RealBeadsOps` implementation:
- Determines rig from bead ID prefix using `routes.jsonl`
- Executes `bd update --add-label` / `--remove-label` for label operations
- Executes `bd list --json --label --status` for queries
- Iterates over all rigs from `mayor/rigs.json` for cross-rig queries

### Dispatcher

The `Dispatcher` coordinates the dispatch of queued items to polecats. It uses the builder pattern for configuration.

```go
// internal/queue/dispatcher.go

type Dispatcher struct {
    queue       *Queue
    spawner     Spawner
    dryRun      bool
    limit       int  // 0 = unlimited
    parallelism int  // 0 or 1 = sequential
}

type DispatchResult struct {
    Dispatched []string  // Bead IDs successfully dispatched
    Skipped    []string  // Bead IDs skipped due to limit
    Errors     []error   // Errors encountered
}
```

**Dispatch algorithm:**
1. Get all valid items (filter out items without rig names)
2. Apply limit if configured (remaining items go to `Skipped`)
3. If dry-run, record what would dispatch and return
4. Use `parallel.Execute` to dispatch items concurrently
5. Remove dispatched items from queue
6. Return results

### Spawner Interface

The `Spawner` interface abstracts polecat creation, allowing the dispatcher to be tested without actually spawning processes.

```go
// internal/queue/dispatcher.go

type Spawner interface {
    SpawnIn(rigName, beadID string) error
}

type RealSpawner struct {
    SpawnInFunc func(rigName, beadID string) error
}
```

The real spawner (provided by `cmd/queue.go`):
1. Gets pre-allocated polecat name from the namepool
2. Creates git worktree in `polecats/<name>/`
3. Updates bead status to "hooked" with assignee
4. Starts tmux session with Claude Code
5. Wakes the rig's witness and refinery agents

### Parallel Executor

The generic parallel executor (`internal/parallel/executor.go`) provides a reusable worker pool pattern with these features:

```go
// internal/parallel/executor.go

func Execute[T any](items []T, parallelism int, work WorkFunc[T]) []Result[T]

type Result[T any] struct {
    Index   int    // Original position in input slice
    Input   T      // The input item
    Success bool   // Whether processing succeeded
    Error   error  // Error if failed
}
```

**Worker pool implementation:**
- Creates N worker goroutines based on `parallelism`
- Uses a job channel to distribute work items by index
- Results are written directly to the result slice at the original index
- Preserves input order in output (important for correlating results)

## Data Flow

### Adding a Bead to the Queue

```
User: gt sling gt-abc gastown --queue
              │
              ▼
┌─────────────────────────────────┐
│  Validate bead exists           │
│  (bd show gt-abc)               │
└───────────────┬─────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  Queue.Add("gt-abc")            │
│                                 │
│  1. Check not town-level        │
│     (rejects hq-* beads)        │
│                                 │
│  2. Get rig from prefix         │
│     (routes.jsonl lookup)       │
│                                 │
│  3. Add "queued" label          │
│     (bd update --add-label)     │
└─────────────────────────────────┘
                │
                ▼
       Bead now has "queued" label
       (persisted in beads DB)
```

### Dispatching Queued Beads

```
User: gt queue run --queue-max-polecats 10 --spawn-batch-size 3
                │
                ▼
┌─────────────────────────────────┐
│  Queue.Load()                   │
│                                 │
│  For each rig in rigs.json:    │
│    bd list --label=queued       │
│           --status=open         │
│           --json                │
│                                 │
│  Filter to open beads only      │
└───────────────┬─────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  Pre-allocate polecat names     │
│                                 │
│  Group items by rig, then       │
│  allocate N names at once       │
│  (single namepool lock)         │
└───────────────┬─────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  Count running polecats         │
│  Calculate available slots:     │
│  slots = capacity - running     │
│                                 │
│  If slots <= 0: exit            │
└───────────────┬─────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  Dispatcher.Dispatch()          │
│                                 │
│  .WithLimit(slots)              │
│  .WithParallelism(3)            │
└───────────────┬─────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  parallel.Execute()             │
│                                 │
│  3 worker goroutines            │
│  Each calls:                    │
│    spawner.SpawnIn(rig, id)     │
└───────────────┬─────────────────┘
                │
                ├──────────────────┬──────────────────┐
                ▼                  ▼                  ▼
           Worker 1           Worker 2           Worker 3
                │                  │                  │
                ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────┐
│  SpawnPolecatForSlingWithName()                         │
│                                                         │
│  1. Create git worktree (polecats/<name>/)              │
│  2. Update bead: status=hooked, assignee=<agent>        │
│  3. Start tmux session with claude --resume             │
│  4. Wake witness and refinery                           │
└─────────────────────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────┐
│  Queue.Remove(beadID)           │
│                                 │
│  (bd update --remove-label)     │
└─────────────────────────────────┘
                │
                ▼
       Polecat running with hooked bead
```

## Configuration

Queue behavior is configured in `settings/config.json` at the town level:

```json
{
  "type": "town-settings",
  "version": 1,
  "queue_max_polecats": 10,
  "polecat_spawn_batch_size": 5
}
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `queue_max_polecats` | int | 0 | **Capacity limit.** Maximum concurrent polecats across all rigs. 0 means unlimited. Set this based on your API rate limits and system resources. A good starting point is 5-10 for a single machine. |
| `polecat_spawn_batch_size` | int | 5 | **Spawn parallelism.** How many polecats to spawn concurrently when dispatching. Higher values speed up dispatch but create burst load. Keep this ≤ `queue_max_polecats`. |

### Choosing Capacity Limits

Consider these factors when setting `queue_max_polecats`:

- **Claude API tier** - Higher tiers allow more concurrent sessions
- **Available RAM** - Each Claude session uses ~500MB-1GB
- **Human bandwidth** - Can you effectively monitor N concurrent workers?
- **Task duration** - Short tasks (minutes) can tolerate higher concurrency than long tasks (hours)

**Example configurations:**

```json
// Conservative: single developer, limited API tier
{ "queue_max_polecats": 5, "polecat_spawn_batch_size": 2 }

// Moderate: team usage, standard API tier
{ "queue_max_polecats": 10, "polecat_spawn_batch_size": 5 }

// Aggressive: dedicated machine, high API tier
{ "queue_max_polecats": 20, "polecat_spawn_batch_size": 10 }
```

### Configuration Resolution

```go
// internal/config/loader.go

func GetQueueMaxPolecats(townRoot string) int {
    settings, err := LoadOrCreateTownSettings(TownSettingsPath(townRoot))
    if err != nil {
        return 0  // Default to unlimited on error
    }
    return settings.QueueMaxPolecats
}

func GetPolecatSpawnBatchSize(townRoot string) int {
    settings, err := LoadOrCreateTownSettings(TownSettingsPath(townRoot))
    if err != nil {
        return 5  // Default batch size
    }
    if settings.PolecatSpawnBatchSize <= 0 {
        return 5
    }
    return settings.PolecatSpawnBatchSize
}
```

### Command-Line Overrides

Both settings can be overridden via command-line flags:

```bash
# Override max polecats (capacity)
gt queue run --queue-max-polecats 20

# Override parallelism
gt queue run --spawn-batch-size 8

# Both
gt queue run --queue-max-polecats 20 --spawn-batch-size 8
```

Flag precedence: flag > config > default.

## Extension Points

### Adding a New Spawner Implementation

To add a new spawner (e.g., for remote execution or testing):

1. Implement the `Spawner` interface:

```go
type RemoteSpawner struct {
    sshHost string
    // ...
}

func (s *RemoteSpawner) SpawnIn(rigName, beadID string) error {
    // SSH to remote host and spawn polecat there
    cmd := exec.Command("ssh", s.sshHost, "gt", "sling", beadID, rigName)
    return cmd.Run()
}
```

2. Wire it up in the command handler:

```go
var spawner queue.Spawner
if remoteHost != "" {
    spawner = &RemoteSpawner{sshHost: remoteHost}
} else {
    spawner = &queue.RealSpawner{SpawnInFunc: localSpawnFunc}
}

dispatcher := queue.NewDispatcher(q, spawner)
```

### Adding a New BeadsOps Implementation

For testing or alternative storage backends:

```go
// internal/beads/beads_fake.go

type FakeBeadsOps struct {
    Beads map[string]*BeadInfo
}

func (f *FakeBeadsOps) IsTownLevelBead(beadID string) bool {
    return strings.HasPrefix(beadID, "hq-")
}

func (f *FakeBeadsOps) LabelAdd(beadID, label string) error {
    if bead, ok := f.Beads[beadID]; ok {
        bead.Labels = append(bead.Labels, label)
        return nil
    }
    return fmt.Errorf("bead not found: %s", beadID)
}

// ... other methods
```

### Customizing Dispatch Behavior

The dispatcher accepts configuration via builder methods:

```go
dispatcher := queue.NewDispatcher(q, spawner).
    WithDryRun(true).           // Don't actually spawn
    WithLimit(5).               // Only dispatch 5 items
    WithParallelism(3)          // 3 concurrent spawns
```

To add new dispatch options (e.g., priority ordering):

1. Add field to `Dispatcher` struct
2. Add builder method (`WithPriority(func(a, b QueueItem) bool)`)
3. Apply in `Dispatch()` method before processing items

## CLI Commands

### gt queue (status)

Shows queue status including pending and running counts.

```bash
$ gt queue
Queue Status:

  Pending:  12 beads
  Running:  3 polecats
  Capacity: 3/10

  By rig:
    gastown: 8
    beads: 4
```

### gt queue list

Lists all queued beads grouped by target rig.

```bash
$ gt queue list
Queued beads: 12

gastown (8):
  gt-abc123  Add user authentication
  gt-def456  Fix login redirect
  ...

beads (4):
  bd-xyz789  Improve error messages
  ...
```

### gt queue run

Dispatches queued beads to polecats.

```bash
$ gt queue run --queue-max-polecats 10 --spawn-batch-size 3
Found 12 queued bead(s)
Capacity: 3/10 polecats running, 7 slots available
Dispatched: 7
Skipped (capacity): 5
```

### gt queue clear

Removes all beads from the queue without dispatching.

```bash
$ gt queue clear
Cleared 5 bead(s) from queue
```

## See Also

- [work-queue-design.md](work-queue-design.md) - Original design exploration document
- [architecture.md](architecture.md) - Overall Gas Town architecture
- [polecat-lifecycle.md](../concepts/polecat-lifecycle.md) - Polecat lifecycle management
