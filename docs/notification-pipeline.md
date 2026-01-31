# Real-Time Notification Pipeline Design

**Task:** gt-t2nz
**Status:** In Progress
**Date:** 2026-01-27

## Executive Summary

This document designs the real-time notification pipeline for surfacing Gas Town decisions to Slack. The core challenge: how do decisions flow from agent creation to human attention with minimal latency?

## Current Architecture

```
┌─────────────┐     bd decision request     ┌──────────────┐
│   Agent     │ ─────────────────────────── │  Beads DB    │
│  (Claude)   │                             │  (Decisions) │
└─────────────┘                             └──────────────┘
                                                   │
                                                   │ File I/O
                                                   ▼
                                            ┌──────────────┐
┌─────────────┐     Poll every 5s           │  gtmobile    │
│  RPC Client │ ◄────────────────────────── │  RPC Server  │
└─────────────┘                             └──────────────┘
```

**Current latency:** 5-10 seconds (polling interval + processing)

### Existing Components

| Component | Location | Purpose |
|-----------|----------|---------|
| WatchDecisions RPC | `mobile/cmd/gtmobile/server.go:320` | Polls beads every 5s, streams new decisions |
| Events system | `internal/events/events.go` | Writes to `.events.jsonl`, has decision types |
| Daemon | `internal/daemon/daemon.go` | Heartbeat every 3 min, manages feed curator |
| Feed curator | `internal/feed/curator.go` | Processes `.events.jsonl` into `.feed.jsonl` |

### Key Observations

1. **No file watching** - System uses polling, not inotify/fsnotify
2. **Events exist but aren't streamed** - `TypeDecisionRequested` events are logged but not pushed
3. **Daemon is recovery-focused** - 3-minute heartbeat, not designed for real-time
4. **RPC streaming is fake** - Uses `time.Ticker` + poll, not true event-driven push

## Latency Requirements

| Use Case | Target Latency | Rationale |
|----------|---------------|-----------|
| Critical decisions (blocking work) | <5 seconds | Agent waiting for human input |
| Normal decisions | <30 seconds | Reasonable human response expectation |
| Low urgency decisions | <5 minutes | Non-blocking, can batch |

## Architecture Options

### Option A: Enhanced Polling (Minimal Change)

```
┌─────────────┐     Poll every 2s           ┌──────────────┐
│  Slack Bot  │ ◄────────────────────────── │  gtmobile    │
└─────────────┘                             │  (poll 2s)   │
                                            └──────────────┘
```

**Implementation:**
- Reduce WatchDecisions poll interval from 5s to 2s
- Add urgency-based poll frequency (high urgency = 1s, low = 10s)

**Pros:**
- Minimal code changes
- Works with existing infrastructure
- Simple to reason about

**Cons:**
- Still polling (resource overhead)
- Latency floor = poll interval
- Doesn't scale well with many clients

**Estimated work:** 1-2 hours

### Option B: Event-Driven with File Watcher

```
┌─────────────┐                             ┌──────────────┐
│   Agent     │ ────── bd decision ───────▶ │  Beads DB    │
└─────────────┘                             └──────┬───────┘
                                                   │
                                            ┌──────▼───────┐
                                            │ Event Logger │
                                            │ (.events.jsonl)
                                            └──────┬───────┘
                                                   │ fsnotify
                                            ┌──────▼───────┐
                                            │  gtmobile    │
                                            │ (event-driven)
                                            └──────┬───────┘
                                                   │ push
                                            ┌──────▼───────┐
                                            │  Slack Bot   │
                                            └──────────────┘
```

**Implementation:**
1. Add fsnotify watcher on `.events.jsonl`
2. Parse new lines, filter for decision events
3. Push to subscribed WatchDecisions streams immediately

**Pros:**
- Near real-time (<1s latency)
- Efficient (no polling, only reacts to changes)
- Leverages existing events system

**Cons:**
- Adds fsnotify dependency
- File watching has platform quirks (Linux/macOS differences)
- Events file rotation needs handling

**Estimated work:** 4-6 hours

### Option C: In-Process Event Bus

```
┌─────────────┐    bd decision     ┌──────────────┐
│   Agent     │ ─────────────────▶ │  Beads DB    │
└─────────────┘                    └──────┬───────┘
                                          │
                                   ┌──────▼───────┐
                                   │  Event Bus   │◄────────┐
                                   │ (in gtmobile)│         │
                                   └──────┬───────┘         │
                                          │                 │
                    ┌─────────────────────┼─────────────────┤
                    │                     │                 │
             ┌──────▼───────┐      ┌──────▼───────┐  ┌──────▼───────┐
             │  Slack Bot   │      │  TUI Client  │  │  Future App  │
             └──────────────┘      └──────────────┘  └──────────────┘
```

**Implementation:**
1. Create event bus within gtmobile process
2. Modify beads client to publish decision events
3. WatchDecisions subscribes to bus, pushes immediately

**Pros:**
- Lowest latency (<100ms)
- Clean pub/sub pattern
- Scales to multiple subscribers

**Cons:**
- Requires gtmobile to be the decision writer (or IPC with writers)
- In-process only (doesn't work for CLI-created decisions)
- More complex architecture

**Estimated work:** 8-12 hours

### Option D: Hybrid (Recommended)

```
┌─────────────┐                             ┌──────────────┐
│   Agent     │ ────── bd decision ───────▶ │  Beads DB    │
└─────────────┘           │                 └──────────────┘
                          │
                          ▼ (emit event)
                   ┌──────────────┐
                   │ Event Logger │
                   └──────┬───────┘
                          │ tail -f style
                   ┌──────▼───────┐
                   │  gtmobile    │
                   │ ┌──────────┐ │
                   │ │File Tail │ │◄─── fsnotify (Linux)
                   │ │+ Polling │ │◄─── poll fallback (all platforms)
                   │ └────┬─────┘ │
                   │      │       │
                   │ ┌────▼────┐  │
                   │ │Broadcast│  │
                   │ └────┬────┘  │
                   └──────┼───────┘
                          │
         ┌────────────────┼────────────────┐
         │                │                │
  ┌──────▼───────┐ ┌──────▼───────┐ ┌──────▼───────┐
  │  Slack Bot   │ │  TUI Client  │ │  CLI Watch   │
  └──────────────┘ └──────────────┘ └──────────────┘
```

**Implementation:**
1. Add event file tailer to gtmobile (fsnotify + poll fallback)
2. Broadcast decision events to all WatchDecisions streams
3. Keep poll-based refresh as safety net (30s interval)
4. Add heartbeat/keepalive for connection health

**Key features:**
- **Graceful degradation:** If file watching fails, falls back to polling
- **Cross-platform:** Works on Linux (inotify) and macOS (kqueue)
- **Multiple clients:** Event bus pattern supports many subscribers
- **Reconnection:** Polling ensures clients eventually sync even after disconnect

**Estimated work:** 6-8 hours

## Notification Deduplication

Regardless of architecture, deduplication is critical:

```go
type DecisionTracker struct {
    seen      map[string]time.Time  // decision ID → first seen
    notified  map[string]bool       // decision ID → notification sent
    mu        sync.RWMutex
}

// ShouldNotify returns true if this decision hasn't been notified recently
func (t *DecisionTracker) ShouldNotify(id string, urgency Urgency) bool {
    t.mu.RLock()
    defer t.mu.RUnlock()

    if t.notified[id] {
        return false  // Already notified
    }

    // For high urgency, always notify immediately
    if urgency == UrgencyHigh {
        return true
    }

    // For normal/low, throttle to avoid spam
    if firstSeen, ok := t.seen[id]; ok {
        if time.Since(firstSeen) < 10*time.Second {
            return false  // Too soon, batch with others
        }
    }

    return true
}
```

## Throttling Strategy

| Urgency | Initial Delay | Repeat Interval | Max Repeats |
|---------|---------------|-----------------|-------------|
| High | 0 (immediate) | 5 minutes | 3 |
| Medium | 5 seconds | 15 minutes | 2 |
| Low | 30 seconds | 1 hour | 1 |

## Offline/Reconnection Handling

### Client Disconnection

```
1. Client connects, receives stream
2. Connection drops (network issue, server restart)
3. Client reconnects
4. Server sends all pending decisions since last seen
5. Client deduplicates and processes
```

**Implementation:**
- Client sends `last_seen_id` on reconnect
- Server replays decisions after that ID
- Client maintains local seen set for dedup

### Server Restart

```
1. Server restarts (loses in-memory state)
2. Clients reconnect
3. Server reads current pending decisions from beads
4. Server sends full list to each client
5. Client deduplicates
```

**Key insight:** Beads is the source of truth. Clients are eventually consistent.

## Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        DECISION LIFECYCLE                         │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  1. CREATION                                                      │
│     Agent runs: gt decision request --prompt "..." --option "..." │
│          │                                                        │
│          ▼                                                        │
│     ┌─────────────┐                                              │
│     │   bd CLI    │ ──▶ Creates decision bead                    │
│     └──────┬──────┘     Logs TypeDecisionRequested event         │
│            │                                                      │
│            ▼                                                      │
│  2. NOTIFICATION                                                  │
│     ┌─────────────┐     ┌───────────────┐                        │
│     │  gtmobile   │ ◀── │ .events.jsonl │ (tail/watch)           │
│     └──────┬──────┘     └───────────────┘                        │
│            │                                                      │
│            ▼ (WatchDecisions stream)                              │
│     ┌─────────────┐                                              │
│     │  Slack Bot  │ ──▶ Posts to Slack channel                   │
│     └─────────────┘     with interactive buttons                  │
│                                                                   │
│  3. RESOLUTION                                                    │
│     Human clicks button in Slack                                  │
│          │                                                        │
│          ▼                                                        │
│     ┌─────────────┐                                              │
│     │  Slack Bot  │ ──▶ Calls Resolve RPC                        │
│     └──────┬──────┘                                              │
│            │                                                      │
│            ▼                                                      │
│     ┌─────────────┐                                              │
│     │  gtmobile   │ ──▶ Updates decision bead                    │
│     └──────┬──────┘     Logs TypeDecisionResolved event          │
│            │                                                      │
│            ▼                                                      │
│  4. AGENT WAKES                                                   │
│     Agent's gt decision await unblocks                            │
│     Agent continues with chosen option                            │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Event File Tailer (Foundation)

1. Add `internal/eventstream/tailer.go`:
   - Watch `.events.jsonl` with fsnotify
   - Parse JSON lines as events arrive
   - Broadcast to subscribers via channels

2. Integration test with manual event injection

### Phase 2: Enhanced WatchDecisions

1. Modify `DecisionServer.WatchDecisions()`:
   - Subscribe to event tailer
   - Push immediately on decision events
   - Keep 30s poll as backup

2. Add last_seen tracking for reconnection

### Phase 3: Slack Bot Integration

1. Slack bot connects to WatchDecisions stream
2. Formats decisions as Slack Block Kit messages
3. Handles button interactions
4. Calls Resolve RPC on button click

### Phase 4: Monitoring & Observability

1. Add metrics: notification latency, dedup hits, reconnection count
2. Add health endpoint for notification pipeline status
3. Alerting on high latency or failed notifications

## Open Questions

1. **Should notifications batch?** Group multiple low-urgency decisions into one Slack message?
2. **Channel routing:** Post to different channels based on urgency or requester?
3. **Thread vs top-level:** Use Slack threads for decision conversations?
4. **Mention routing:** @mention specific users based on decision metadata?

## References

- `mobile/cmd/gtmobile/server.go` - Current RPC server
- `internal/events/events.go` - Events system
- `internal/daemon/notification.go` - Notification deduplication pattern
- `docs/connect-rpc-integration.md` - Slack bot architecture
