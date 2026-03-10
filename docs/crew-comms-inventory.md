# Crew Mesh Communications Capability Inventory

> **Bead**: st-zmn.1 | **Date**: 2026-03-10 | **Author**: sfgastown/crew/dunks

## Overview

Structured inventory of what each crew member owns in the comms stack,
compiled from live broadcast testing and direct crew responses.

---

## Dunks (sfgastown/crew/dunks)

**Domain**: gt CLI, nudge system, observer hooks

| Capability | Description | Status |
|------------|-------------|--------|
| `gt nudge` CLI | Send messages to agents via tmux send-keys | Production |
| Delivery modes | `wait-idle` (default), `queue`, `immediate` | Production |
| Observer hooks | Fire-and-forget HTTP POST on nudge events | Production |
| `observer.Flush()` | WaitGroup-based drain before process exit | Production |
| Priority field | Observer payloads include priority metadata | Production |

**Known limitations**:

- Nudge ✓ means "tmux accepted keystrokes", NOT "agent received message"
- No broadcast primitive — fan-out requires N individual sends
- No response tracking — can't detect who hasn't replied
- `wait-idle` and `queue` modes are human-gated in nvim-claude harnesses
- Only `--mode=immediate` auto-wakes crew sessions

---

## Timmy (tmux_adapter/crew/timmy)

**Domain**: Adapter internals, WebSocket protocol, agent registry, REST API

| Capability | Description | Status |
|------------|-------------|--------|
| WebSocket server | `ws://127.0.0.1:8080/ws` with agent identity | Production |
| Agent registry | Track connected agents by name | Production |
| `identify` message | Register agent identity on WS connect | Production |
| `send-message` type | Direct crew-to-crew messaging via WS | Production |
| `list-agents` type | Query who's currently connected | Production |
| REST API | `POST /api/agents/{name}/prompt` | Production |
| tmux socket | Connects to gt tmux for session management | Production |

**Known limitations**:

- WS identity mismatch: claudecode.nvim identifies as generic 'nvim-claude' not crew name
- Direct WS messages fall back to nudge queue when identity doesn't match
- REST API requires double-quote JSON (single quotes fail silently)
- No built-in broadcast — adapter handles point-to-point only

---

## Neil (nvimconfig/crew/neil)

**Domain**: Neovim integration, harness WebSocket client

| Capability | Description | Status |
|------------|-------------|--------|
| Adapter WS connection | `require('claudecode.adapter').connect()` | Production |
| `:GasMessages` | View mesh messages in Neovim | Production |
| `:GasSend` | Send mesh messages from Neovim | Production |
| Telescope agents picker | Browse connected agents via Telescope | Production |
| Lifecycle subscription | Subscribe to agent connect/disconnect events | Production |

**Known limitations**:

- No adapter broadcast primitive yet (tracked as ta-a33.2)
- Fan-out = N individual sends for now
- Most crew haven't called `adapter.connect()` — WS is underutilized

---

## CC (claudecode/crew/cc)

**Domain**: claudecode.nvim session wrapper, MCP tooling

| Capability | Description | Status |
|------------|-------------|--------|
| claudecode.nvim plugin | Session wrapper for Claude Code in Neovim | Production |
| `crew_messages` MCP tool | Pull mesh messages via MCP tool call | Deployed (needs restart) |
| Auto-identify fix | Plugin identifies with tmux session name automatically | Production |
| WS Quick Start Guide | Documentation for crew mesh setup | Delivered via mail |

**Known limitations**:

- MCP tool discovery is static — new tools need session restart
- `crew_messages` is pull-based (polling), not push-based (event-driven)

---

## Charlie (context_mode/crew/charlie)

**Domain**: context-mode plugin, persistent DBs, cross-agent knowledge sharing

| Capability | Description | Status |
|------------|-------------|--------|
| `ctx_list_peers` | Discover running context-mode agents | Production |
| `ctx_search_peers` | Search other agents' indexed knowledge (read-only) | Production |
| Persistent databases | Named DBs that survive sessions (`~/.claude/context-mode/`) | Production |
| `ctx_index` / `ctx_search` | BM25 full-text search over indexed content | Production |

**Known limitations**:

- Did not respond to broadcast test (silent throughout st-zmn testing)
- Peer discovery requires context-mode MCP to be running
- Knowledge sharing is read-only — no write-back or update notification

---

## Mayor (global coordinator)

**Domain**: Coordination layer, topology, dispatch

| Capability | Description | Status |
|------------|-------------|--------|
| `gt mail` system | Durable mail with Dolt persistence | Production |
| Escalation routing | Receives all `gt escalate` calls | Production |
| Nudge loop protocol | Initiate coordinated multi-crew nudge rounds | Protocol defined |
| Topology awareness | Knows rig/crew/polecat structure | Production |

**Known limitations**:

- Coordination is manual — no automated orchestration
- No built-in consensus or quorum mechanism
- Mail creates permanent Dolt commits (heavyweight for routine comms)

---

## Cross-Cutting Findings

### Channel Matrix

| Channel | Mechanism | Durable? | Delivery guarantee | Latency |
|---------|-----------|----------|--------------------|---------|
| `gt nudge --mode=immediate` | tmux send-keys | No | None (fire-and-forget) | ~instant |
| `gt nudge --mode=wait-idle` | tmux send-keys (waits) | No | None (human-gated in harness) | Variable |
| `gt nudge --mode=queue` | File queue + hook | No | Hook-dependent | Next turn |
| `gt mail send` | Dolt commit | Yes | Persistent | Next inbox check |
| WebSocket `send-message` | WS via adapter | No | Connected agents only | ~instant |
| REST `/api/agents/{name}/prompt` | HTTP POST | No | Returns `{"ok":true}` | ~instant |
| `ctx_search_peers` | context-mode FTS5 | Read-only | Requires running peer | ~instant |

### Problems Identified (8 total)

### Broadcast Test #1 Results (wait-idle mode, st-zmn.1 era)

- **Sent**: 4 nudges (wait-idle mode)
- **Responded**: 5/6 crew (Neil, Timmy, CC, Tooly, Charlie silent)
- **Via mail** (requested channel): 0/5
- **Via nudge-back**: 5/5
- **Confirmed non-receipt**: 1 (Timmy — nudge returned ✓ but never arrived)

### Broadcast Test #2 Results (immediate mode, st-zmn.2)

- **Sent**: 5 nudges (immediate mode) at 23:14 AEDT
- **Fan-out time**: ~10s sequential (1.2-3.0s per target)
- **Responded**: 4/5 via mail (the requested channel!)
  - CC: Option D (Hybrid) — biggest problem: silent failure modes
  - Charlie: Option D (Hybrid) — biggest problem: silent failure modes
  - Timmy: Option D (Hybrid) — biggest problem: adapter.connect() reliability
  - Tooly: Option D (Hybrid) — biggest problem: no broadcast primitive
- **Missing**: Neil (no response yet)
- **Channel compliance**: 4/4 replied via mail as requested (vs 0/5 in test #1!)
- **Consensus**: Unanimous Option D (Hybrid: WS online + mail fallback)

**Key improvement over test #1**: `--mode=immediate` + explicit mail instructions
yielded 100% channel compliance from responders (vs 0% with wait-idle).

- **Via mail** (requested channel): 0/5
- **Via nudge-back**: 5/5
- **Confirmed non-receipt**: 1 (Timmy — nudge returned ✓ but never arrived)

### Broadcast Test #2 Results (immediate mode, st-zmn.2)

- **Sent**: 5 nudges (immediate mode) at 23:14 AEDT
- **Fan-out time**: ~10s sequential (1.2-3.0s per target)
- **Responded**: 4/5 via mail (the requested channel!)
  - CC: Option D (Hybrid) — biggest problem: silent failure modes
  - Charlie: Option D (Hybrid) — biggest problem: silent failure modes
  - Timmy: Option D (Hybrid) — biggest problem: adapter.connect() reliability
  - Tooly: Option D (Hybrid) — biggest problem: no broadcast primitive
- **Missing**: Neil (no response yet)
- **Channel compliance**: 4/4 replied via mail as requested (vs 0/5 in test #1!)
- **Consensus**: Unanimous Option D (Hybrid: WS online + mail fallback)

**Key improvement over test #1**: `--mode=immediate` + explicit mail instructions
yielded 100% channel compliance from responders (vs 0% with wait-idle).
