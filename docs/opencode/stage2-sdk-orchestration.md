# Stage 2: SDK-Based Orchestration Layer

> **Status**: Planning  
> **Priority**: Future Enhancement  
> **Prerequisites**: Stage 1 (current: CLI + Plugin) validated in production  
> **Estimated Effort**: 2-3 weeks

## Executive Summary

This document outlines the architectural plan for migrating from CLI-based OpenCode integration to SDK-based orchestration using the [opencode-sdk-go](https://github.com/anomalyco/opencode-sdk-go) library.

**Key Insight**: While the current CLI + Plugin approach works for basic integration, an SDK-based approach provides superior programmatic control, error handling, and resource efficiency.

---

## Current Architecture (Stage 1)

```
┌─────────────────┐     spawn      ┌─────────────────────────────────┐
│   Gastown (gt)  │──────────────▶│      opencode CLI Process       │
│                 │                │                                 │
│  • exec.Command │ no feedback    │  ┌───────────────────────────┐ │
│  • PATH-based   │◀ ─ ─ ─ ─ ─ ─ ─│  │   gastown.js Plugin       │ │
└─────────────────┘  (fire-forget) │  │   • session.created       │ │
                                   │  │   • Calls: gt prime       │ │
                                   │  └───────────────────────────┘ │
                                   │  ┌───────────────────────────┐ │
                                   │  │   MCP Server #1           │ │
                                   │  │   (per-session!)          │ │
                                   │  └───────────────────────────┘ │
                                   └─────────────────────────────────┘
```

### Stage 1 Limitations

| Issue | Impact | Severity |
|-------|--------|----------|
| No feedback loop (plugin → gt) | Invisible failures | 🔴 High |
| PATH dependency (bd version skew) | Breaks silently in new shells | 🔴 High |
| Each session spawns own MCP servers | Resource exhaustion at scale | 🔴 High |
| Plugin errors invisible | Silent failures | 🟡 Medium |
| No programmatic session control | Limited orchestration | 🟡 Medium |
| Race conditions on bd access | Potential data corruption | 🟡 Medium |

---

## Proposed Architecture (Stage 2)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Gastown (gt)                                      │
│  ┌────────────────┐    ┌────────────────┐    ┌─────────────────┐        │
│  │ Orchestrator   │◀──▶│ Session Pool   │◀──▶│ Beads Client    │        │
│  │ Interface      │    │ Manager        │    │                 │        │
│  │                │    │                │    │                 │        │
│  │ • SpawnSession │    │ • Spawn        │    │ • Work assign   │        │
│  │ • AssignWork   │    │ • Monitor      │    │ • Status track  │        │
│  │ • AbortSession │    │ • Health check │    │                 │        │
│  └────────────────┘    └────────────────┘    └─────────────────┘        │
│           │                    │                                         │
│           ▼                    ▼                                         │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                    Backend Implementations                          │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────────┐  │ │
│  │  │ TmuxOrchestrator │  │OpenCodeOrchestrator│ │HybridOrchestrator│ │ │
│  │  │ (CLI, current)   │  │ (SDK, new)        │  │ (auto-select)   │  │ │
│  │  └──────────────────┘  └──────────────────┘  └─────────────────┘  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                                   │
                                   │ HTTP API (SDK)
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   OpenCode Server (Single Instance)                      │
│                            `opencode serve`                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────────────┐   │
│  │Session 1 │  │Session 2 │  │Session 3 │  │ Shared MCP Pool       │   │
│  │(polecat) │  │(polecat) │  │(witness) │  │ • searxng (1x)        │   │
│  └──────────┘  └──────────┘  └──────────┘  │ • github (1x)         │   │
│                                            └───────────────────────────│ │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## OpenCode Go SDK Overview

**Repository**: https://github.com/anomalyco/opencode-sdk-go  
**Version**: v0.19.2 (as of 2026-01)  
**Go Version**: Requires Go 1.22+

### Key API Endpoints

```go
import "github.com/sst/opencode-sdk-go"

client := opencode.NewClient()

// Session Lifecycle
client.Session.New(ctx, params)        // Create session
client.Session.List(ctx, query)        // List sessions  
client.Session.Get(ctx, id, query)     // Get session
client.Session.Delete(ctx, id, params) // Delete session
client.Session.Abort(ctx, id, params)  // Abort running session

// Session Operations
client.Session.Prompt(ctx, id, params) // Send prompt
client.Session.Command(ctx, id, params)// Execute command
client.Session.Shell(ctx, id, params)  // Run shell
client.Session.Messages(ctx, id, query)// Get history
client.Session.Children(ctx, id, query)// Get child sessions
client.Session.Revert(ctx, id, params) // Revert state
client.Session.Fork(ctx, id, params)   // Fork session

// Supporting Services
client.Agent.List(ctx, query)          // List agents
client.File.Read(ctx, query)           // Read files
client.Event.List(ctx, query)          // Get events
client.Config.Get(ctx, query)          // Get config
```

---

## Orchestrator Interface Design

```go
// internal/orchestration/orchestrator.go

package orchestration

import (
    "context"
    "time"
)

// Session represents an active agent session.
type Session struct {
    ID        string
    Agent     string            // "claude", "opencode", "gemini"
    WorkDir   string
    Role      string            // "polecat", "mayor", etc.
    Status    SessionStatus
    CreatedAt time.Time
}

type SessionStatus string

const (
    SessionCreating  SessionStatus = "creating"
    SessionReady     SessionStatus = "ready"
    SessionBusy      SessionStatus = "busy"
    SessionIdle      SessionStatus = "idle"
    SessionStopped   SessionStatus = "stopped"
    SessionError     SessionStatus = "error"
)

// Orchestrator manages agent sessions.
type Orchestrator interface {
    // Session lifecycle
    SpawnSession(ctx context.Context, cfg SessionConfig) (*Session, error)
    GetSession(ctx context.Context, id string) (*Session, error)
    ListSessions(ctx context.Context) ([]*Session, error)
    StopSession(ctx context.Context, id string) error
    
    // Work assignment
    AssignWork(ctx context.Context, sessionID string, work Work) error
    GetSessionOutput(ctx context.Context, sessionID string) (string, error)
    
    // Session control
    AbortSession(ctx context.Context, id string) error
    ResumeSession(ctx context.Context, id string) error
    ForkSession(ctx context.Context, id string, opts ForkOptions) (*Session, error)
    
    // Lifecycle hooks
    OnSessionStart(ctx context.Context, id string) error
    OnSessionIdle(ctx context.Context, id string) error
    OnPreCompact(ctx context.Context, id string) error
}

// SessionConfig configures a new session.
type SessionConfig struct {
    Agent      string
    WorkDir    string
    Role       string
    Model      string   // Optional model override
    InitPrompt string   // Optional initial prompt
}

// Work represents work to assign to a session.
type Work struct {
    Type    string // "prompt", "mail", "bead"
    Content string
    BeadID  string // For bead-based work
}

// ForkOptions configures session forking.
type ForkOptions struct {
    AtMessageID string // Fork from specific message
    NewRole     string // Role for forked session
}
```

---

## Implementation Plan

### Phase 1: Interface Definition (2-3 days)
- [ ] Create `internal/orchestration/` package
- [ ] Define `Orchestrator` interface
- [ ] Define supporting types (`Session`, `Work`, etc.)
- [ ] Add comprehensive documentation

### Phase 2: Refactor Current (3-5 days)
- [ ] Extract tmux logic into `TmuxOrchestrator`
- [ ] Ensure current tests pass with new structure
- [ ] No behavioral changes, pure refactor

### Phase 3: OpenCode Backend (3-5 days)
- [ ] Add `opencode-sdk-go` to `go.mod`
- [ ] Implement `OpenCodeOrchestrator`
- [ ] Add server lifecycle management (`opencode serve`)
- [ ] Implement all interface methods

### Phase 4: Integration (2-3 days)
- [ ] Add config-driven backend selection
- [ ] Implement `HybridOrchestrator` (per-agent routing)
- [ ] Update CLI to pass orchestrator via DI
- [ ] E2E testing with both backends

---

## Configuration

```toml
# town.toml

# Default: Use tmux-based orchestration (works with all agents)
[orchestration]
backend = "tmux"

# SDK-based for OpenCode only, tmux for others
[orchestration]
backend = "hybrid"
opencode_server_url = "http://localhost:4096"

# All-in on OpenCode SDK
[orchestration]
backend = "opencode"
opencode_server_url = "http://localhost:4096"
```

---

## Benefits vs Risks

### Benefits

| Benefit | Description |
|---------|-------------|
| **Version Pinning** | SDK is Go dependency, no PATH issues |
| **Shared MCP Pool** | Single server, shared MCP connections |
| **Programmatic Control** | Full lifecycle management |
| **Error Handling** | Structured errors, retries |
| **Session Events** | Real-time monitoring via Event API |
| **No Vendor Lock-in** | Interface-based, swap backends anytime |

### Risks

| Risk | Mitigation |
|------|------------|
| SDK pre-1.0 stability | Pin version, monitor releases |
| OpenCode project health | Keep tmux fallback, gradual migration |
| Server management overhead | Use daemon mode, health checks |
| Testing complexity | Separate test suites per backend |

---

## Decision Points Before Implementation

1. **Server Lifecycle**: Who manages `opencode serve`?
   - Option A: Gastown starts/stops server
   - Option B: User manages independently
   - Option C: Systemd/launchd service

2. **MCP Server Sharing**: How to configure shared MCP pool?
   - Option A: Global config in `~/.config/opencode/`
   - Option B: Per-town config
   - Option C: Dynamic discovery

3. **Fallback Strategy**: When SDK fails, what happens?
   - Option A: Hard fail
   - Option B: Auto-fallback to tmux
   - Option C: Alert and await user decision

---

## Related Documentation

- [OpenCode Integration Summary](./OPENCODE_INTEGRATION_SUMMARY.md)
- [E2E Test Results](./E2E_TEST_RESULTS.md)
- [OpenCode Experiments](./opencode-experiments.md)
- [Technical Research](./technical-research.md)

---

**Created**: 2026-01-18  
**Status**: Planning  
**Next Action**: Complete Stage 1 validation before proceeding
