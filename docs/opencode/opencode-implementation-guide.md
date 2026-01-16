# Opencode Implementation Guide

> Practical implementation details for Opencode orchestration in Gastown
> 
> **Companion to**: [opencode-orchestration.md](opencode-orchestration.md)
> **Status**: Planning
> **Updated**: 2026-01-15

## Quick Reference

### Current Status

**What Works Today**:
- ✅ Single Opencode session spawn
- ✅ Plugin-based context injection (`gt prime`)
- ✅ Basic runtime config support
- ✅ Wrapper script integration

**What's Being Added**:
- 🔨 Multi-session orchestration
- 🔨 Remote session support
- 🔨 Cross-session coordination
- 🔨 Unified orchestration API

## Code Structure

### New Packages

```
internal/orchestration/
├── orchestrator.go        # Core interface definition
├── types.go              # Shared types (SessionInfo, WorkUnit, etc.)
├── registry.go           # Session registry implementation
├── pool.go               # Session pooling
├── claude/
│   ├── orchestrator.go   # Claude backend implementation
│   └── session.go        # Claude session management
└── opencode/
    ├── orchestrator.go   # Opencode backend implementation
    ├── session.go        # Opencode session management
    └── remote.go         # Remote session support
```

### Modified Packages

```
internal/runtime/runtime.go     # Add orchestrator selection
internal/config/types.go        # Add orchestrator config
internal/cmd/sling.go          # Use orchestrator API
internal/cmd/agents.go         # Query orchestrator registry
```

## Interface Definitions

### Core Orchestrator Interface

```go
// Package orchestration provides backend-agnostic agent orchestration.
package orchestration

import (
    "context"
    "time"
)

// Orchestrator manages agent lifecycle and work distribution.
// Implementations exist for Claude Code and Opencode.
type Orchestrator interface {
    // Name returns the orchestrator backend name ("claude", "opencode").
    Name() string
    
    // SpawnSession creates a new agent session.
    SpawnSession(ctx context.Context, cfg *SessionConfig) (SessionID, error)
    
    // TerminateSession stops an agent session gracefully.
    TerminateSession(ctx context.Context, id SessionID) error
    
    // GetSession retrieves session information.
    GetSession(ctx context.Context, id SessionID) (*SessionInfo, error)
    
    // ListSessions returns all active sessions managed by this orchestrator.
    ListSessions(ctx context.Context) ([]*SessionInfo, error)
    
    // AssignWork sends a work unit to a session.
    AssignWork(ctx context.Context, id SessionID, work *WorkUnit) error
    
    // SendMessage sends a message to a session (best-effort).
    SendMessage(ctx context.Context, id SessionID, msg *Message) error
    
    // BroadcastMessage sends a message to multiple sessions.
    BroadcastMessage(ctx context.Context, ids []SessionID, msg *Message) error
    
    // HealthCheck verifies the orchestrator backend is operational.
    HealthCheck(ctx context.Context) error
}

// SessionConfig contains parameters for spawning a new session.
type SessionConfig struct {
    Role          string            // "polecat", "witness", "crew", etc.
    WorkDir       string            // Working directory for the session
    Rig           string            // Rig name
    AgentName     string            // Agent instance name (e.g., "Toast")
    InitialWork   *WorkUnit         // Optional work to assign immediately
    Env           map[string]string // Environment variables
    RemoteHost    string            // Optional: remote host for execution
    Metadata      map[string]string // Additional metadata
}

// SessionID uniquely identifies an orchestrated session.
type SessionID string

// SessionInfo contains runtime information about a session.
type SessionInfo struct {
    ID         SessionID
    Backend    string    // "claude" | "opencode"
    Role       string
    AgentName  string
    Rig        string
    State      SessionState
    StartedAt  time.Time
    Location   SessionLocation
    WorkUnits  []*WorkUnit
    PID        int    // Process ID (if local)
    RemoteHost string // Remote host (if remote)
}

// SessionState represents the current state of a session.
type SessionState string

const (
    StateStarting SessionState = "starting"
    StateRunning  SessionState = "running"
    StatePaused   SessionState = "paused"
    StateStopped  SessionState = "stopped"
    StateFailed   SessionState = "failed"
)

// SessionLocation indicates where the session is running.
type SessionLocation string

const (
    LocationLocal  SessionLocation = "local"
    LocationRemote SessionLocation = "remote"
)

// WorkUnit represents a unit of work to be executed by an agent.
type WorkUnit struct {
    ID          string
    Type        string // "issue", "formula", "task"
    Description string
    Payload     map[string]interface{}
    Priority    int
    AssignedAt  time.Time
}

// Message represents an inter-session message.
type Message struct {
    From    SessionID
    To      SessionID
    Type    string // "nudge", "mail", "broadcast"
    Subject string
    Body    string
    SentAt  time.Time
}
```

### Session Registry

```go
// Registry tracks all active sessions across all orchestrators.
type Registry struct {
    mu          sync.RWMutex
    sessions    map[SessionID]*SessionInfo
    byBackend   map[string][]SessionID
    byRig       map[string][]SessionID
}

// NewRegistry creates a new session registry.
func NewRegistry() *Registry {
    return &Registry{
        sessions:  make(map[SessionID]*SessionInfo),
        byBackend: make(map[string][]SessionID),
        byRig:     make(map[string][]SessionID),
    }
}

// Register adds a session to the registry.
func (r *Registry) Register(info *SessionInfo) error

// Unregister removes a session from the registry.
func (r *Registry) Unregister(id SessionID) error

// Get retrieves session information.
func (r *Registry) Get(id SessionID) (*SessionInfo, error)

// List returns all sessions, optionally filtered.
func (r *Registry) List(filter *SessionFilter) ([]*SessionInfo, error)

// SessionFilter specifies criteria for filtering sessions.
type SessionFilter struct {
    Backend string
    Rig     string
    Role    string
    State   SessionState
}
```

## Backend Implementations

### Claude Orchestrator

```go
// Package claude implements the Orchestrator interface for Claude Code.
package claude

import (
    "context"
    "github.com/steveyegge/gastown/internal/orchestration"
    "github.com/steveyegge/gastown/internal/tmux"
)

// Orchestrator implements orchestration.Orchestrator for Claude Code.
type Orchestrator struct {
    tmux     *tmux.Tmux
    registry *orchestration.Registry
}

// NewOrchestrator creates a new Claude orchestrator.
func NewOrchestrator(t *tmux.Tmux, registry *orchestration.Registry) *Orchestrator {
    return &Orchestrator{
        tmux:     t,
        registry: registry,
    }
}

// Name returns "claude".
func (o *Orchestrator) Name() string {
    return "claude"
}

// SpawnSession creates a new Claude Code session via tmux.
func (o *Orchestrator) SpawnSession(ctx context.Context, cfg *orchestration.SessionConfig) (orchestration.SessionID, error) {
    // Implementation:
    // 1. Create worktree (if needed)
    // 2. Install Claude hooks
    // 3. Start tmux session with `claude --resume` or `claude`
    // 4. Wait for ready state
    // 5. Inject initial work (if provided)
    // 6. Register session
    // 7. Return session ID
}

// AssignWork sends work to a Claude session via hooks/mail.
func (o *Orchestrator) AssignWork(ctx context.Context, id orchestration.SessionID, work *orchestration.WorkUnit) error {
    // Implementation:
    // 1. Write work to mailbox
    // 2. Send nudge via tmux send-keys
    // 3. Update session's WorkUnits list
}

// (Other methods...)
```

### Opencode Orchestrator

```go
// Package opencode implements the Orchestrator interface for Opencode.
package opencode

import (
    "context"
    "os/exec"
    "github.com/steveyegge/gastown/internal/orchestration"
)

// Orchestrator implements orchestration.Orchestrator for Opencode.
type Orchestrator struct {
    registry *orchestration.Registry
}

// NewOrchestrator creates a new Opencode orchestrator.
func NewOrchestrator(registry *orchestration.Registry) *Orchestrator {
    return &Orchestrator{
        registry: registry,
    }
}

// Name returns "opencode".
func (o *Orchestrator) Name() string {
    return "opencode"
}

// SpawnSession creates a new Opencode session.
func (o *Orchestrator) SpawnSession(ctx context.Context, cfg *orchestration.SessionConfig) (orchestration.SessionID, error) {
    // Implementation:
    // 1. Create workspace directory
    // 2. Install Opencode plugin
    // 3. Start opencode process
    // 4. Wait for session.created event
    // 5. Inject initial work (if provided)
    // 6. Register session
    // 7. Return session ID
}

// AssignWork sends work to an Opencode session via file or API.
func (o *Orchestrator) AssignWork(ctx context.Context, id orchestration.SessionID, work *orchestration.WorkUnit) error {
    // Implementation:
    // 1. Write work to mailbox (Beads)
    // 2. Trigger plugin notification (if possible)
    // 3. Update session's WorkUnits list
}

// (Other methods...)
```

## Integration Points

### 1. Runtime Selection

Update `internal/runtime/runtime.go`:

```go
// GetOrchestrator returns the appropriate orchestrator for a runtime config.
func GetOrchestrator(rc *config.RuntimeConfig) (orchestration.Orchestrator, error) {
    switch rc.Provider {
    case "claude":
        return claude.NewOrchestrator(tmux.Default(), globalRegistry)
    case "opencode":
        return opencode.NewOrchestrator(globalRegistry)
    default:
        return nil, fmt.Errorf("unknown orchestrator: %s", rc.Provider)
    }
}
```

### 2. Sling Command

Update `internal/cmd/sling.go`:

```go
func slingRun(cmd *cobra.Command, args []string) error {
    // ... existing arg parsing ...
    
    // Get runtime config
    rc, err := config.LoadRuntimeConfig(rigPath)
    if err != nil {
        return err
    }
    
    // Get orchestrator
    orch, err := runtime.GetOrchestrator(rc)
    if err != nil {
        return err
    }
    
    // Spawn session
    sessionID, err := orch.SpawnSession(ctx, &orchestration.SessionConfig{
        Role:        "polecat",
        WorkDir:     workDir,
        Rig:         rigName,
        AgentName:   agentName,
        InitialWork: workUnit,
    })
    if err != nil {
        return err
    }
    
    log.Printf("Spawned %s session %s", orch.Name(), sessionID)
    return nil
}
```

### 3. Agents Command

Update `internal/cmd/agents.go`:

```go
func agentsRun(cmd *cobra.Command, args []string) error {
    // Query all orchestrators
    claudeOrch := getClaudeOrchestrator()
    opencodeOrch := getOpencodeOrchestrator()
    
    claudeSessions, _ := claudeOrch.ListSessions(ctx)
    opencodeSessions, _ := opencodeOrch.ListSessions(ctx)
    
    // Merge and display
    allSessions := append(claudeSessions, opencodeSessions...)
    displayAgentTable(allSessions)
    return nil
}
```

## Configuration Examples

### Rig-Level Opencode Config

`<rig>/settings/config.json`:

```json
{
  "type": "rig-settings",
  "version": 1,
  "agent": "opencode",
  "runtime": {
    "provider": "opencode",
    "command": "opencode",
    "hooks": {
      "provider": "opencode",
      "dir": ".opencode/plugin",
      "settings_file": "gastown.js"
    }
  }
}
```

### Town-Level Default

`settings/config.json`:

```json
{
  "type": "town-settings",
  "version": 1,
  "default_agent": "claude",
  "role_agents": {
    "mayor": "claude",
    "deacon": "claude",
    "witness": "claude",
    "refinery": "claude",
    "polecat": "opencode"
  }
}
```

### Mixed Backend Workflow

```bash
# Mayor (Claude) coordinates
gt mayor attach

# Spawn Opencode workers for parallel tasks
gt sling issue-1 myproject --agent opencode --name worker-1
gt sling issue-2 myproject --agent opencode --name worker-2
gt sling issue-3 myproject --agent opencode --name worker-3

# Spawn Claude refinery for review
gt refinery start --agent claude

# Monitor all
gt agents
# Shows:
# BACKEND   ROLE      NAME       RIG        STATE
# claude    mayor     -          -          running
# opencode  polecat   worker-1   myproject  running
# opencode  polecat   worker-2   myproject  running
# opencode  polecat   worker-3   myproject  running
# claude    refinery  -          myproject  running
```

## Testing Strategy

### Unit Tests

```go
// orchestration/orchestrator_test.go
func TestClaudeOrchestrator(t *testing.T) {
    // Test Claude backend in isolation
}

func TestOpencodeOrchestrator(t *testing.T) {
    // Test Opencode backend in isolation
}

// orchestration/registry_test.go
func TestRegistry(t *testing.T) {
    // Test session registry operations
}
```

### Integration Tests

```go
// orchestration/integration_test.go
func TestMultiBackendWorkflow(t *testing.T) {
    // Spawn Claude session
    // Spawn Opencode session
    // Assign work to both
    // Verify both complete
}

func TestCrossSessionMessaging(t *testing.T) {
    // Spawn 2 sessions (different backends)
    // Send message from A to B
    // Verify receipt
}
```

### Manual Testing Checklist

- [ ] Spawn single Opencode session
- [ ] Spawn 3 concurrent Opencode sessions
- [ ] Assign work to Opencode session
- [ ] Verify `gt agents` shows Opencode sessions
- [ ] Test mixed Claude + Opencode workflow
- [ ] Test message passing between backends
- [ ] Test session cleanup on termination
- [ ] Test error handling (spawn failure, work failure)

## Error Handling

### Common Errors

**Spawn Failure**:
```go
if err := orch.SpawnSession(ctx, cfg); err != nil {
    // Log error
    // Clean up partial state
    // Return user-friendly error
    return fmt.Errorf("failed to spawn %s session: %w", orch.Name(), err)
}
```

**Work Assignment Failure**:
```go
if err := orch.AssignWork(ctx, sessionID, work); err != nil {
    // Mark work as failed
    // Optionally reassign to different session
    // Escalate if critical
    return fmt.Errorf("failed to assign work to session %s: %w", sessionID, err)
}
```

**Backend Unavailable**:
```go
if err := orch.HealthCheck(ctx); err != nil {
    // Fall back to alternative backend (if configured)
    // Or fail gracefully with clear message
    return fmt.Errorf("backend %s unavailable: %w", orch.Name(), err)
}
```

## Performance Considerations

### Resource Limits

**Per-Backend Limits**:
```go
const (
    MaxConcurrentClaudeSessions  = 10 // Memory-limited
    MaxConcurrentOpencodeSessions = 20 // To be determined by testing
)
```

**Pooling Strategy**:
- Lazy spawn: Create sessions on-demand
- Warm pool: Keep N idle sessions ready
- Auto-scale: Spawn up to limit, then queue

### Monitoring

**Metrics to Track**:
- Session count per backend
- Session spawn time
- Work assignment latency
- Message delivery latency
- Session memory/CPU usage

**Logging**:
```go
log.Printf("[orchestration] spawned %s session %s for rig %s", backend, sessionID, rig)
log.Printf("[orchestration] assigned work %s to session %s", workID, sessionID)
log.Printf("[orchestration] session %s state: %s → %s", sessionID, oldState, newState)
```

## Migration Path

### Phase 1: Add Infrastructure (No User Impact)

1. Add `internal/orchestration/` package
2. Implement Claude orchestrator (wraps existing code)
3. Implement basic Opencode orchestrator
4. Add tests

**Result**: No user-visible changes, foundation in place.

### Phase 2: Wire Up Commands (Opt-In)

1. Update `gt sling` to use orchestrator API
2. Update `gt agents` to query orchestrators
3. Add `--backend` flag for explicit selection

**Result**: Users can opt into Opencode via config or flag.

### Phase 3: Advanced Features

1. Multi-session pooling
2. Remote session support
3. Cross-session messaging enhancements

**Result**: Full feature parity with existing Claude workflows.

## Documentation Requirements

### User Docs

- [ ] "Choosing a Backend" guide
- [ ] Opencode setup instructions
- [ ] Multi-session workflow examples
- [ ] Troubleshooting guide

### Developer Docs

- [ ] Orchestrator interface documentation
- [ ] Adding new backend guide
- [ ] Testing guide
- [ ] Performance tuning guide

## Open Issues

### High Priority

1. **Opencode Session Lifecycle**: How to detect when Opencode session is ready?
   - Option A: Poll for plugin initialization
   - Option B: Parse stdout for ready signal
   - Option C: Use Opencode API (if exists)

2. **Remote Session API**: What's the best way to spawn remote Opencode sessions?
   - Option A: SSH + opencode command
   - Option B: Opencode remote API (verify exists)
   - Option C: Agent-based approach (remote agent pulls work)

3. **State Persistence**: Where to store session state?
   - Option A: In-memory (lost on restart)
   - Option B: File in `.runtime/` (local)
   - Option C: Beads ledger (distributed)

### Medium Priority

4. **Session Resume**: Can Opencode sessions be resumed like Claude?
5. **Resource Limits**: What's the practical limit for concurrent sessions?
6. **Plugin Capabilities**: Full catalog of Opencode plugin events?

### Low Priority

7. **UI Integration**: Dashboard view for multi-backend sessions?
8. **Auto-failover**: Automatically switch backends on failure?
9. **Cost Tracking**: Track per-backend usage for optimization?

## Next Steps

1. **Review**: Get feedback on interface design
2. **Prototype**: Build minimal Opencode orchestrator
3. **Test**: Verify multi-session spawn works
4. **Iterate**: Refine based on testing
5. **Document**: Write user-facing guides
6. **Release**: Roll out gradually with feature flag

---

**Last Updated**: 2026-01-15
**Owner**: Gastown Team
**Related**: [opencode-orchestration.md](opencode-orchestration.md)
