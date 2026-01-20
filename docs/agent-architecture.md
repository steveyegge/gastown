# Agent Architecture

This document describes the unified factory architecture for agent management in Gas Town.

## Overview

The refactored architecture consolidates agent lifecycle management into a layered, interface-driven design. Previously, each role (deacon, mayor, witness, refinery) had its own manager with duplicated startup, shutdown, and communication logic. The new design provides:

- **Single source of truth** for agent lifecycle operations
- **Interface segregation** allowing consumers to depend only on needed capabilities
- **Testable design** with conformance-tested doubles
- **Future extensibility** for horizontal scaling and remote execution

---

## High-Level Object Design

### Package Hierarchy

```
internal/ids/          → Agent identity and addressing (zero dependencies)
    ↓
internal/session/      → Terminal session abstraction (tmux-agnostic)
    ↓
internal/agent/        → Process lifecycle management
    ↓
internal/factory/      → Composition and startup orchestration
```

### Core Interfaces

The design follows the **Interface Segregation Principle** with five focused interfaces:

```go
// Read-only observation of agents
type AgentObserver interface {
    Exists(id AgentID) bool
    GetInfo(id AgentID) (*session.Info, error)
    List() ([]AgentID, error)
}

// Lifecycle cleanup
type AgentStopper interface {
    Stop(id AgentID, graceful bool) error
}

// Startup and readiness
type AgentStarter interface {
    StartWithConfig(id AgentID, cfg StartConfig) error
    WaitReady(id AgentID) error
}

// Interactive communication
type AgentCommunicator interface {
    Nudge(id AgentID, message string) error
    Capture(id AgentID, lines int) (string, error)
    CaptureAll(id AgentID) (string, error)
    Attach(id AgentID) error
}

// Atomic restart for handoff
type AgentRespawner interface {
    Respawn(id AgentID) error
}

// Full interface composes all capabilities
type Agents interface {
    AgentObserver
    AgentStopper
    AgentStarter
    AgentCommunicator
    AgentRespawner
}
```

### Implementation Structure

```
┌─────────────────────────────────────────────────────────┐
│                    factory.Start()                       │
│  - Resolves agent preset from config                     │
│  - Builds environment variables                          │
│  - Computes working directory                            │
│  - Orchestrates full startup with theming                │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│              agent.Implementation                        │
│  - Wraps session.Sessions                                │
│  - Handles zombie detection                              │
│  - Manages readiness checking                            │
│  - Delegates to session layer                            │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│              session.Sessions (interface)                │
│  - Start/Stop/Exists/Respawn                             │
│  - Send/SendControl/Nudge                                │
│  - Capture/CaptureAll/IsRunning/WaitFor                  │
│  - List/GetInfo/Attach/SwitchTo                          │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│                   tmux.Tmux                              │
│  - Concrete implementation                               │
│  - Subprocess execution                                  │
│  - Theming and configuration                             │
└─────────────────────────────────────────────────────────┘
```

### Data Flow

**Startup Flow:**
```
factory.Start(townRoot, agentID, opts...)
    → agents.StartWithConfig(id, cfg)
        → sess.Start(sessionID, workDir, command)
            → tmux new-session -d -s name command
        → cfg.OnCreated(sessionID)  // theming callback
    → agents.WaitReady(id)  // polls until prompt visible
```

**Communication Flow:**
```
agents.Nudge(id, message)
    → sess.Nudge(sessionID, message)
        → per-session mutex lock
        → tmux send-keys (with vim mode handling)
```

**Shutdown Flow:**
```
agents.Stop(id, graceful=true)
    → sess.SendControl(sessionID, "C-c")  // SIGINT
    → time.Sleep(100ms)
    → kill descendants deepest-first
    → sess.Stop(sessionID)
```

### Identity Model

The `ids.AgentID` struct provides hierarchical addressing:

```go
type AgentID struct {
    Role   string  // mayor, deacon, witness, refinery, crew, polecat
    Rig    string  // rig name (empty for town-level singletons)
    Worker string  // worker name (empty for rig-level singletons)
}
```

**Address formats:**
- Town singletons: `mayor`, `deacon`
- Rig singletons: `myrig/witness`, `myrig/refinery`
- Named workers: `myrig/polecat/Toast`, `myrig/crew/alice`

**Session name mapping:**
- `gt-mayor` → `{Role: "mayor"}`
- `gt-myrig-witness` → `{Role: "witness", Rig: "myrig"}`
- `gt-myrig-polecat-Toast` → `{Role: "polecat", Rig: "myrig", Worker: "Toast"}`

---

## Testing Benefits

### Test Double Patterns

The architecture provides multiple test double implementations following the Meszaros/Fowler taxonomy:

| Type | Class | Purpose |
|------|-------|---------|
| **Fake** | `agent.Double` | Full in-memory implementation with spy capabilities |
| **Fake** | `agent.ObserverDouble` | Minimal fake for read-only testing |
| **Fake** | `session.Double` | Session-layer fake for agent tests |
| **Stub** | `agent.AgentsStub` | Wrapper for error injection |

**agent.Double capabilities:**
```go
// Setup
fake := agent.NewDouble()
fake.CreateAgent(id, "/work/dir", "command")

// Spy accessors
fake.StopCalls()           // []StopCall with ID and Graceful flag
fake.GetNudgeLog(id)       // []string of nudged messages
fake.GetStartConfig(id)    // StartConfig used at creation
fake.GetWorkDir(id)        // Working directory
fake.GetCommand(id)        // Start command
```

**Error injection with AgentsStub:**
```go
fake := agent.NewDouble()
stub := agent.NewAgentsStub(fake)
stub.StartErr = errors.New("simulated failure")
// Now StartWithConfig returns the injected error
```

### Conformance Testing

The `agent/conformance_test.go` runs identical test cases against both the real implementation and the Double:

```go
func TestConformance(t *testing.T) {
    testCases := []struct {
        name string
        test func(t *testing.T, agents agent.Agents)
    }{
        {"StartAlreadyRunning", testStartAlreadyRunning},
        {"StopRunning", testStopRunning},
        {"StopNotRunning", testStopNotRunning},
        // ...
    }

    for _, tc := range testCases {
        t.Run("Real/"+tc.name, func(t *testing.T) {
            tc.test(t, realAgents)
        })
        t.Run("Double/"+tc.name, func(t *testing.T) {
            tc.test(t, agent.NewDouble())
        })
    }
}
```

This ensures the Double behaves identically to the real implementation for:
- `ErrAlreadyRunning` on duplicate start
- `ErrNotRunning` on operations against non-existent agents
- Graceful vs force shutdown behavior
- Nudge and capture operations

### Mocking Boundaries

**Session-level mocking:**
- Test agent logic without tmux
- Verify correct session method calls
- Inject session failures

**Agent-level mocking:**
- Test managers without agent complexity
- Verify manager business logic
- Focus on domain behavior

```go
// Manager tests use agent.Double (not session.Double)
func TestWitnessManager(t *testing.T) {
    agents := agent.NewDouble()
    manager := witness.NewManager(agents)
    // Test manager logic with predictable agent behavior
}
```

---

## Maintenance Benefits

### Single Point of Change

Previously, startup logic was duplicated across:
- `internal/deacon/manager.go`
- `internal/mayor/manager.go`
- `internal/witness/manager.go`
- `internal/refinery/manager.go`
- `internal/boot/` startup orchestration

Now, all startup logic lives in:
- `internal/agent/start_config.go` - StartWithConfig implementation
- `internal/factory/factory.go` - Orchestration and configuration

**Impact:** Changes to startup behavior (zombie handling, readiness checking, callback ordering) now require editing one location.

### Clear Ownership Boundaries

| Package | Owns | Does Not Own |
|---------|------|--------------|
| `ids` | Identity parsing, address formats | Lifecycle, communication |
| `session` | Terminal abstraction, tmux operations | Agent-specific behavior |
| `agent` | Process lifecycle, zombie detection | Working directories, env vars |
| `factory` | Orchestration, role-specific config | Low-level session ops |

### Bug Fix Propagation

With unified implementation, fixes apply to all roles:

| Bug Class | Fix Location | Affects |
|-----------|--------------|---------|
| Race condition in startup | `start_config.go` | All agent types |
| Orphan process cleanup | `agent.go:Stop()` | All agent types |
| Nudge interleaving | `tmux.go:Nudge()` | All sessions |
| Zombie detection | `agent.go:Exists()` | All agent types |

### Reduced Learning Curve

**Before:** Developers needed to understand:
- Deacon manager pattern
- Mayor manager pattern
- Witness manager pattern
- Connection package patterns
- Each with subtle differences

**After:** One pattern to learn:
1. `factory.Start()` for startup
2. `agent.Agents` interface for operations
3. `agent.Double` for testing

---

## Future Extensibility

### Polecat Execution Abstraction

The session interface abstracts the execution environment:

```go
type Sessions interface {
    Start(name, workDir, command string) (SessionID, error)
    Stop(id SessionID) error
    // ... other operations
}
```

Current implementation: `tmux.Tmux` (local subprocess)

Future implementations could include:
- **Container execution:** Run polecats in Docker/Podman
- **Kubernetes pods:** Each polecat as a K8s pod
- **Remote SSH:** Execute on remote machines

### Remote Session Backends

The `Sessions` interface is network-transparent:

```go
// Current: local tmux
sessions := tmux.NewTmux()

// Future: remote tmux over SSH
sessions := remote.NewSSHSessions("worker-1.example.com")

// Future: Kubernetes backend
sessions := k8s.NewPodSessions(clientset, "polecats-namespace")
```

Agents layer remains unchanged - it only depends on the `Sessions` interface.

### Multi-Machine Deployment

The architecture enables horizontal scaling:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Control Plane │     │    Worker 1     │     │    Worker 2     │
│                 │     │                 │     │                 │
│  ┌───────────┐  │     │  ┌───────────┐  │     │  ┌───────────┐  │
│  │  Factory  │──┼────►│  │  Polecats │  │     │  │  Polecats │  │
│  └───────────┘  │     │  └───────────┘  │     │  └───────────┘  │
│                 │     │                 │     │                 │
│  ┌───────────┐  │     └─────────────────┘     └─────────────────┘
│  │  Mayor    │  │              ▲                      ▲
│  │  Deacon   │  │              │                      │
│  └───────────┘  │              └──────────────────────┘
└─────────────────┘                Remote Sessions
```

**Required changes for multi-machine:**
1. Implement `RemoteSessions` backend
2. Add worker registration/discovery
3. Route `Start()` calls to appropriate worker
4. Proxy `Capture()`/`Nudge()` to correct worker

The agent and factory layers require no changes.

### Pluggable Session Implementations

The current codebase already demonstrates pluggability:

| Implementation | Use Case |
|---------------|----------|
| `tmux.Tmux` | Production execution |
| `session.Double` | Agent-layer tests |
| `agent.Double` | Manager-layer tests |

Adding new session backends:

```go
// Implement session.Sessions interface
type ZellijSessions struct { /* ... */ }

func (z *ZellijSessions) Start(name, workDir, command string) (SessionID, error) {
    // zellij-specific implementation
}

// Use with existing agent layer
agents := agent.New(zellijSessions, config)
```

---

## Package Reference

### internal/ids

Zero-dependency identity layer:
- `AgentID` struct with Role/Rig/Worker
- Address constructors (`MayorAddress`, `WitnessAddress`, etc.)
- Parsing functions (`ParseAddress`, `ParseSessionName`)

### internal/session

Portable terminal session abstraction:
- `Sessions` interface (Start, Stop, Nudge, Capture, etc.)
- `SessionID` type
- `Info` struct for session metadata

### internal/agent

Process lifecycle management:
- Segregated interfaces (`AgentObserver`, `AgentStopper`, etc.)
- `Implementation` struct (production)
- `Double` and `ObserverDouble` (testing)
- `AgentsStub` (error injection)
- `Config` and `StartConfig` types

### internal/factory

Orchestration and startup:
- `Agents()` - singleton accessor
- `Start()` - full production startup
- `StartWithAgents()` - testable startup
- `StartOption` functional options

### internal/tmux

Concrete tmux implementation:
- `Tmux` struct implementing `Sessions`
- Theming support
- Session configuration
