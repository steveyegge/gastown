# Opencode Orchestration Integration Plan

> Design document for adding Opencode as a primary orchestration layer in Gastown
> 
> **Status**: Planning
> **Created**: 2026-01-15
> **Related**: [architecture.md](architecture.md), [plugin-system.md](plugin-system.md)

## Executive Summary

This document outlines a plan to extend Gastown's orchestration capabilities beyond Claude Code to support Opencode as an alternative primary orchestration layer. This will enable Gastown to orchestrate work across multiple concurrent Opencode sessions, including those running in other agents or on remote hosts, providing flexibility for collaborative agent workflows.

## Current State Analysis

### Existing Runtime Architecture

Gastown currently supports multiple AI runtimes through a pluggable configuration system:

**Supported Runtimes** (as of current codebase):
- **Claude Code** (default): Full hook support, native integration
- **Codex**: Partial support via fallback mechanisms
- **Gemini, Cursor, Auggie, AMP**: Basic support with agent presets
- **Opencode**: Plugin exists, but limited orchestration capabilities

**Key Infrastructure**:
```
internal/runtime/runtime.go      - Runtime abstraction layer
internal/config/types.go         - RuntimeConfig with provider support
internal/config/agents.go        - Agent preset registry
internal/opencode/plugin.go      - Opencode plugin management
internal/opencode/plugin/gastown.js - Event hooks for Opencode
```

### Current Opencode Integration

**What Exists**:
1. **Plugin system**: `gastown.js` plugin hooks into Opencode's `session.created` event
2. **Wrapper script**: `gt-opencode` primes context before launching Opencode
3. **Runtime config**: Opencode recognized as a provider with default settings
4. **Hook installation**: `EnsurePluginAt()` installs the Gastown plugin

**What's Missing**:
1. **Multi-session coordination**: No mechanism to spawn/manage multiple Opencode instances
2. **Remote session support**: No API for distributed Opencode execution
3. **Agent-to-agent communication**: Limited cross-session messaging
4. **State synchronization**: No distributed state tracking
5. **Session lifecycle management**: No coordinated startup/shutdown

## Design Goals

### Primary Objectives

1. **Feature Parity**: Replicate all Claude Code orchestration patterns via Opencode
2. **Pluggable Architecture**: Clean abstraction allowing runtime swapping
3. **Multi-Session Orchestration**: Coordinate multiple concurrent Opencode sessions
4. **Remote Execution**: Support Opencode sessions on remote hosts
5. **Forward Compatibility**: Easy adoption of new Opencode features

### Non-Goals

- Replace Claude Code (both should coexist)
- Perfect feature compatibility (document divergences instead)
- Implement missing Opencode features (work with what exists)

## Architecture Overview

### Orchestration Abstraction Layer

```
┌─────────────────────────────────────────────────────────┐
│  Gastown Orchestration API                              │
│  (gt sling, gt agents, gt convoy)                       │
└────────────────┬────────────────────────────────────────┘
                 │
    ┌────────────┴────────────┐
    │                         │
┌───▼──────────┐    ┌────────▼──────────┐
│  Claude Code │    │  Opencode         │
│  Backend     │    │  Backend          │
│              │    │                   │
│ • Hooks      │    │ • Plugins/Skills  │
│ • Sessions   │    │ • API/Commands    │
│ • Resume     │    │ • Remote Sessions │
└──────────────┘    └───────────────────┘
```

### Key Components

#### 1. Orchestrator Interface

New abstraction in `internal/orchestration/`:

```go
// Orchestrator manages agent lifecycle and work distribution
type Orchestrator interface {
    // Session Management
    SpawnAgent(config AgentConfig) (SessionID, error)
    TerminateAgent(SessionID) error
    ListActiveSessions() ([]SessionInfo, error)
    
    // Work Distribution
    AssignWork(SessionID, WorkUnit) error
    GetSessionStatus(SessionID) (Status, error)
    
    // Communication
    SendMessage(SessionID, Message) error
    BroadcastMessage([]SessionID, Message) error
    
    // Hooks/Events
    OnSessionStart(SessionID, Callback) error
    OnSessionEnd(SessionID, Callback) error
    OnWorkComplete(SessionID, Callback) error
}
```

#### 2. Backend Implementations

**Claude Backend** (`internal/orchestration/claude/`):
- Wraps existing Claude Code integration
- Uses hooks for context injection
- Manages sessions via tmux

**Opencode Backend** (`internal/orchestration/opencode/`):
- Uses Opencode plugin system
- Manages sessions via Opencode API/CLI
- Handles remote session coordination

#### 3. Session Registry

Track all active sessions regardless of backend:

```go
// SessionRegistry maintains state of all orchestrated agents
type SessionRegistry struct {
    sessions map[SessionID]*SessionInfo
    backend  Orchestrator
}

type SessionInfo struct {
    ID         SessionID
    Backend    string // "claude" | "opencode"
    Role       string // "polecat" | "witness" | etc.
    State      State  // "running" | "paused" | "stopped"
    WorkUnits  []WorkUnit
    Location   Location // local | remote host
}
```

## Integration Points

### 1. Agent Spawning (gt sling)

**Current Flow (Claude)**:
```
gt sling issue-123 myproject
  → Create worktree
  → Write CLAUDE.md
  → Start tmux session with claude --resume
  → Inject context via hooks
```

**Proposed Flow (Opencode)**:
```
gt sling issue-123 myproject --agent opencode
  → Create workspace directory
  → Write AGENTS.md
  → Start opencode session
  → Plugin auto-runs gt prime
  → Load work from mailbox
```

**Decision Point**: Where to route based on agent selection?
- **Option A**: Choose backend at config load time based on `agent` field
- **Option B**: Explicit backend selection via `--backend` flag
- **Option C**: Infer from agent name (claude→Claude backend, opencode→Opencode backend)

**Recommendation**: Option C (implicit routing) with Option B (explicit override) for flexibility.

### 2. Multi-Session Coordination

**Requirement**: Spawn multiple Opencode sessions for parallel work.

**Challenges**:
1. **Session isolation**: Each Opencode session needs separate workspace
2. **State synchronization**: Work state must be visible across sessions
3. **Resource limits**: How many concurrent sessions? (10? 20? 30?)

**Proposed Solution**:

```go
// SessionPool manages a pool of orchestrated sessions
type SessionPool struct {
    maxSessions int
    backend     Orchestrator
    registry    *SessionRegistry
}

func (p *SessionPool) SpawnWorkers(count int, role string) ([]SessionID, error)
func (p *SessionPool) AssignWork(workUnits []WorkUnit) error
func (p *SessionPool) Rebalance() error
```

**Open Questions**:
- [ ] Does Opencode limit concurrent sessions?
- [ ] What's the overhead per session?
- [ ] Can we use Opencode's native pooling?

### 3. Remote Session Support

**Use Case**: Run Opencode sessions on remote CI workers or distributed hosts.

**Opencode Capability**: Verify if Opencode supports remote sessions (need to check docs).

**Proposed Architecture**:

```
┌──────────────┐      SSH/RPC      ┌──────────────┐
│   Gastown    │◄─────────────────►│   Remote     │
│   Mayor      │    Commands        │   Host       │
│              │                    │              │
│              │      Status        │  Opencode    │
│              │◄─────────────────  │  Session     │
└──────────────┘                    └──────────────┘
```

**Implementation Options**:
1. **SSH-based**: Execute `opencode` via SSH, monitor via logs
2. **RPC/API**: If Opencode provides remote API
3. **Hybrid**: Local control plane, remote execution

**Areas for Experimentation**:
- [ ] Test Opencode remote session creation
- [ ] Verify state synchronization methods
- [ ] Measure latency and reliability

### 4. Cross-Session Communication

**Current Gastown Messaging** (via Beads):
- Mail system: durable messages in `.beads/`
- Nudges: tmux send-keys for real-time
- Wisps: ephemeral event records

**Opencode Considerations**:
- Does Opencode have native message passing?
- Can plugins send messages between sessions?
- Should we use Beads for all backends?

**Proposed Approach**:
Use Beads as universal message bus:
```
Session A (Claude)  ────┐
                        ├──► Beads Mail ◄──┬─── Session B (Opencode)
Session C (Opencode)────┘                  └─── Session D (Opencode)
```

### 5. Plugin vs Hook Parity

**Claude Hooks** provide:
- `SessionStart`: Inject context via `gt prime`
- `Compaction`: Run cleanup on session end
- Settings file: Configure hook behavior

**Opencode Plugins** provide:
- `session.created`: Similar to SessionStart
- Event-driven architecture
- JavaScript plugin API

**Gap Analysis**:

| Feature | Claude Hooks | Opencode Plugin | Status |
|---------|-------------|-----------------|--------|
| Session init | ✅ SessionStart | ✅ session.created | ✅ Equivalent |
| Cleanup | ✅ Compaction | ❓ Unknown | 🔍 Needs Research |
| Mail injection | ✅ Via hooks | ⚠️ Via plugin script | ⚠️ Less integrated |
| Context priming | ✅ Native | ✅ Via `gt prime` | ✅ Working |
| Session resume | ✅ Native | ❓ Unknown | 🔍 Needs Research |

**Action Items**:
- [ ] Research Opencode event catalog
- [ ] Test session cleanup hooks
- [ ] Verify resume capabilities

## Workflow Patterns

### Pattern 1: Single Agent Work (Existing)

No changes needed - already works with Opencode via plugin.

### Pattern 2: Fan-Out (Parallel Work Distribution)

**Use Case**: Split a large task across multiple agents.

**Implementation**:
```bash
# Mayor creates convoy
gt convoy create "Parallel Work" issue-1 issue-2 issue-3

# Spawn pool of Opencode workers
gt sling issue-1 myproject --agent opencode
gt sling issue-2 myproject --agent opencode
gt sling issue-3 myproject --agent opencode

# Monitor progress
gt convoy show
```

**Backend Handling**:
- Each sling spawns independent Opencode session
- Work assigned via mailbox
- Status tracked via Beads

### Pattern 3: Collaborative (Agent Handoff)

**Use Case**: Pass work from one agent to another mid-task.

**Challenge**: Requires shared context and state.

**Proposed Solution**:
```bash
# Agent A starts work
gt sling issue-123 myproject --agent opencode --name agent-a

# Agent A hands off to Agent B
gt handoff agent-a agent-b --context "completed research, needs implementation"

# Agent B resumes
# (Plugin loads context from hook/mailbox)
```

**Open Questions**:
- [ ] How to serialize work context?
- [ ] Can Opencode sessions share state files?
- [ ] Best handoff mechanism?

### Pattern 4: Remote Distributed (CI/Workers)

**Use Case**: Run agents in CI pipeline or distributed workers.

**Architecture**:
```
CI Job 1                    CI Job 2                    CI Job 3
┌─────────────┐            ┌─────────────┐            ┌─────────────┐
│ Opencode    │            │ Opencode    │            │ Opencode    │
│ Worker      │            │ Worker      │            │ Worker      │
│             │            │             │            │             │
│ Work: Tests │            │ Work: Build │            │ Work: Lint  │
└─────────────┘            └─────────────┘            └─────────────┘
       │                          │                          │
       └──────────────────────────┴──────────────────────────┘
                                  │
                          Central Gastown
                          State (Beads)
```

**Requirements**:
- Opencode must run headless
- State must be centralized (git-backed Beads)
- Results must be aggregated

**Implementation Path**:
1. Test Opencode in CI environment
2. Build remote session wrapper
3. Integrate with convoy tracking

## Migration & Compatibility

### Configuration Migration

**Existing Config**:
```json
{
  "runtime": {
    "provider": "claude",
    "command": "claude",
    "args": ["--dangerously-skip-permissions"]
  }
}
```

**Opencode Config**:
```json
{
  "runtime": {
    "provider": "opencode",
    "command": "opencode",
    "args": []
  }
}
```

**No breaking changes** - existing configs work as-is.

### Instruction Files

**Current**:
- Claude: `CLAUDE.md`
- Codex: `AGENTS.md`
- Opencode: `AGENTS.md`

**Unified Approach**:
Create symlinks or dual files:
```bash
CLAUDE.md → AGENTS.md  # Symlink for compatibility
```

### Hook/Plugin Compatibility

**Strategy**: Maintain parallel implementations
- Claude: Continue using hooks
- Opencode: Continue using plugins
- Shared: Common state layer (Beads)

## Open Questions & Experimentation Areas

### Critical Questions

1. **Opencode Remote Sessions**:
   - [ ] Does Opencode support remote session creation?
   - [ ] What's the API surface for remote control?
   - [ ] Latency characteristics?

2. **Opencode Session Limits**:
   - [ ] Max concurrent sessions per host?
   - [ ] Resource consumption per session?
   - [ ] Best practices for pooling?

3. **Plugin Lifecycle**:
   - [ ] Full event catalog available?
   - [ ] Can plugins persist state?
   - [ ] Error handling in plugins?

4. **State Synchronization**:
   - [ ] How to share work state across sessions?
   - [ ] Conflict resolution strategies?
   - [ ] Performance with 20-30 sessions?

### Experimentation Plan

**Phase 1: Basic Multi-Session** (2-3 days)
- [ ] Spawn 3 Opencode sessions simultaneously
- [ ] Assign independent work to each
- [ ] Monitor via `gt agents`
- [ ] Document resource usage

**Phase 2: Cross-Session Communication** (3-4 days)
- [ ] Implement Beads mail for Opencode agents
- [ ] Test message passing between sessions
- [ ] Verify state consistency
- [ ] Measure message latency

**Phase 3: Remote Sessions** (4-5 days)
- [ ] Set up remote test environment
- [ ] Spawn Opencode session remotely
- [ ] Test work assignment and completion
- [ ] Document remote setup process

**Phase 4: Integration Testing** (3-4 days)
- [ ] Mixed Claude + Opencode workflows
- [ ] Convoy with both backend types
- [ ] Stress test with 20+ sessions
- [ ] Performance profiling

## Implementation Phases

### Phase 1: Orchestrator Abstraction (Week 1-2)

**Goals**:
- Define `Orchestrator` interface
- Implement `ClaudeOrchestrator` (wrapper around existing)
- Implement basic `OpencodeOrchestrator`
- Session registry

**Deliverables**:
- `internal/orchestration/` package
- Tests for both orchestrators
- Documentation

### Phase 2: Multi-Session Support (Week 3-4)

**Goals**:
- Session pooling
- Work distribution across pools
- Monitoring and status

**Deliverables**:
- `SessionPool` implementation
- `gt agents` shows all backends
- Multi-session spawn working

### Phase 3: Remote Support (Week 5-6)

**Goals**:
- Remote session creation
- State synchronization
- Remote monitoring

**Deliverables**:
- SSH-based remote executor
- Remote session examples
- CI integration guide

### Phase 4: Production Hardening (Week 7-8)

**Goals**:
- Error handling
- Resource limits
- Performance optimization
- Documentation

**Deliverables**:
- Complete user guide
- Migration documentation
- Example workflows
- Performance benchmarks

## Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Opencode lacks remote API | Medium | High | Fall back to SSH; document limitation |
| Session pooling unstable | Low | Medium | Implement conservative limits initially |
| State sync conflicts | Medium | Medium | Use git-backed Beads with conflict detection |
| Performance degradation | Low | High | Profile early, set hard session limits |
| Plugin limitations | Medium | Medium | Document gaps, use fallback mechanisms |

### Operational Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| User confusion (2 backends) | High | Low | Clear documentation, sensible defaults |
| Config complexity | Medium | Low | Provide migration guide and examples |
| Breaking changes | Low | High | Maintain backward compatibility |
| Support burden | Medium | Medium | Document clearly which features work where |

## Success Criteria

### Must Have (P0)

- [ ] Spawn multiple Opencode sessions
- [ ] Assign work via `gt sling` with Opencode backend
- [ ] Track Opencode sessions in `gt agents`
- [ ] Basic cross-session messaging via Beads
- [ ] Documentation on choosing backend

### Should Have (P1)

- [ ] Remote Opencode session support
- [ ] Mixed Claude + Opencode workflows
- [ ] Session pooling with auto-balancing
- [ ] Performance parity with Claude

### Nice to Have (P2)

- [ ] Auto-migration of Claude configs to Opencode
- [ ] Visual dashboard for multi-backend view
- [ ] Advanced remote features (distributed locking, etc.)
- [ ] Opencode-specific optimizations

## References

### Internal Documentation

- [architecture.md](architecture.md) - Core Gastown architecture
- [plugin-system.md](plugin-system.md) - Plugin design principles
- [federation.md](federation.md) - Multi-workspace coordination
- [operational-state.md](operational-state.md) - State management

### External Resources

- [Opencode Repository](https://github.com/anomalyco/opencode)
- [oh-my-opencode](https://github.com/code-yeongyu/oh-my-opencode) - Opencode extensions
- Opencode API documentation (TODO: add link when found)
- Opencode plugin system docs (TODO: add link when found)

### Code Locations

- `internal/runtime/` - Runtime abstraction layer
- `internal/config/` - Configuration types
- `internal/opencode/` - Opencode integration
- `internal/agent/` - Agent state management
- `internal/session/` - Session lifecycle
- `internal/tmux/` - Session spawning (Claude)

## Next Steps

1. **Immediate** (This week):
   - [ ] Review this plan with stakeholders
   - [ ] Prioritize open questions
   - [ ] Set up Opencode test environment

2. **Short-term** (Next 2 weeks):
   - [ ] Begin Phase 1 implementation
   - [ ] Run Phase 1 experiments
   - [ ] Document findings

3. **Medium-term** (Next month):
   - [ ] Complete Phase 1-2
   - [ ] Begin remote testing
   - [ ] User feedback on multi-session

4. **Long-term** (Next quarter):
   - [ ] Production-ready multi-backend support
   - [ ] Advanced features based on feedback
   - [ ] Performance optimization

---

**Last Updated**: 2026-01-15
**Owner**: Gastown Team
**Status**: Planning - Awaiting Approval
