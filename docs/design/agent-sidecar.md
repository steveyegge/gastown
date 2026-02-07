# Agent Sidecar Architecture Design

Design document for the Gas Town agent sidecar — a lightweight gRPC daemon
running inside each K8s pod that manages terminal access for remote polecats.

**Status**: Proposed (supersedes terminal-server.md and Phase 5 SSH approach)
**Epic**: gt-3bwtas (Phase 5: SSH-Tmux Solution for Remote Terminal Viewing)

## Problem

Gas Town agents running on K8s need `gt peek` and `gt nudge` support. The
existing commands assume local tmux sessions. Two prior approaches were
explored and found wanting:

1. **Terminal server bridge** (terminal-server.md): A `gt terminal-server`
   process that proxies via `kubectl exec` to screen sessions inside pods.
   Problems: requires local tmux mirroring, screen + tmux dual multiplexer,
   polling-based state sync.

2. **SSH approach** (Phase 5, first implementation): Direct SSH into pods
   running sshd + tmux. Problems: dual transport (SSH + kubectl exec),
   SSH key management overhead, sshd attack surface in pods.

## Design: Agent Sidecar

Run a small Go binary (`gt-sidecar`) as PID 1 inside each agent pod. The
sidecar manages tmux internally and exposes gRPC RPCs for terminal access.

```
┌─────────────────────────────────────────┐
│  K8s Pod                                │
│                                         │
│  ┌─────────────────────────────────┐    │
│  │  gt-sidecar (PID 1)            │    │
│  │                                 │    │
│  │  gRPC :9090                     │    │
│  │  ├─ Peek() → capture-pane      │    │
│  │  ├─ Nudge(msg) → send-keys     │    │
│  │  ├─ Status() → health, phase   │    │
│  │  └─ WatchOutput() → stream     │    │
│  │                                 │    │
│  │  tmux session "claude"          │    │
│  │  └─ Claude Code (attached)     │    │
│  └─────────────────────────────────┘    │
│                                         │
└─────────────────────────────────────────┘
```

### Why Sidecar, Not Bridge

| Concern | Bridge (terminal-server.md) | Sidecar |
|---------|---------------------------|---------|
| Transport | kubectl exec (fragile) | gRPC (robust, typed) |
| Multiplexer | screen in pod + tmux local | tmux only, in pod |
| State | Polling + local mirror | Direct, no mirror |
| Auth | kubectl RBAC | K8s network policy |
| Scaling | One bridge per rig | One sidecar per pod |
| Failure domain | Bridge down = all pods dark | Sidecar down = one pod dark |

## Design Decisions

### D1: Transport — Connect-RPC (gRPC)

**Decision**: Sidecar exposes Connect-RPC on port 9090, same framework used by
the existing Gas Town RPC server (`internal/rpcserver/server.go`).

**Rationale**: Reuse existing patterns. Connect-RPC supports both gRPC and
gRPC-Web protocols, works with `buf` for code generation, and the team already
has working examples.

### D2: One Multiplexer — tmux

**Decision**: tmux inside pods. No screen. No local tmux mirror.

**Rationale**: Gas Town already has a mature tmux wrapper (`internal/tmux/tmux.go`).
Using screen added a second multiplexer with different semantics. The sidecar
wraps tmux directly, so all existing tmux code is reusable.

### D3: Fixed Session Name — "claude"

**Decision**: Each pod has exactly one tmux session named "claude".

**Rationale**: One agent per pod. No ambiguity about which session to target.
The sidecar manages the session lifecycle — creating it at startup, monitoring
health, restarting if it dies.

### D4: Pod Entrypoint — gt-sidecar as PID 1

**Decision**: `gt-sidecar` is PID 1 in the container, responsible for:
1. Creating the tmux session
2. Launching Claude Code (or other agent) inside the session
3. Serving gRPC
4. Forwarding signals for graceful shutdown

**Alternative**: Separate init container + sidecar container. Rejected because
it adds container coordination complexity and the sidecar is inherently tied
to the agent lifecycle.

### D5: Backend Resolution — Bead Metadata

**Decision**: `ResolveBackend(agentID)` checks agent bead metadata for
`backend=k8s` plus `sidecar_host` and `sidecar_port` fields. If present,
returns `GRPCSidecarBackend`. Otherwise returns local `TmuxBackend`.

**Rationale**: Same pattern as the SSH approach but with different metadata
fields. Phase 4 (remote polecats) sets these fields when spawning K8s pods.

### D6: Auth — K8s Network Policy (cluster-internal)

**Decision**: No authentication on sidecar gRPC. Access controlled by K8s
network policy — only pods in the Gas Town namespace can reach sidecar ports.

**Rationale**: SSH key management was unnecessary complexity. The sidecar is
cluster-internal infrastructure. Network policy is the standard K8s answer
for pod-to-pod access control.

## Proto Definition

```protobuf
// proto/gastown/v1/sidecar.proto
syntax = "proto3";
package gastown.v1;

service SidecarService {
  // Peek captures terminal output from the agent's tmux session
  rpc Peek(SidecarPeekRequest) returns (SidecarPeekResponse);

  // Nudge sends a message to the agent's terminal
  rpc Nudge(SidecarNudgeRequest) returns (SidecarNudgeResponse);

  // Status returns agent health and phase information
  rpc Status(SidecarStatusRequest) returns (SidecarStatusResponse);

  // WatchOutput streams terminal output updates (future)
  rpc WatchOutput(SidecarWatchRequest) returns (stream SidecarOutputUpdate);
}

message SidecarPeekRequest {
  int32 lines = 1;  // Number of lines (default 50)
  bool all = 2;     // Capture all scrollback
}

message SidecarPeekResponse {
  string output = 1;
  bool session_alive = 2;
}

message SidecarNudgeRequest {
  string message = 1;
}

message SidecarNudgeResponse {
  bool delivered = 1;
  string error = 2;
}

message SidecarStatusRequest {}

message SidecarStatusResponse {
  bool session_alive = 1;
  string agent_phase = 2;   // "starting", "running", "idle", "stuck"
  int64 uptime_seconds = 3;
  string last_output_at = 4; // RFC3339 timestamp
}

message SidecarWatchRequest {
  int32 lines = 1;
  int32 interval_ms = 2;
}

message SidecarOutputUpdate {
  string output = 1;
  bool session_alive = 2;
  string timestamp = 3;
}
```

## Component Layout

```
cmd/gt-sidecar/
  main.go              # Entrypoint: start gRPC, create tmux, launch agent

internal/sidecar/
  server.go            # SidecarService implementation (wraps tmux)
  monitor.go           # Background tmux session health monitor

internal/terminal/
  backend.go           # Backend interface (unchanged)
  local.go             # TmuxBackend (unchanged)
  grpc_sidecar.go      # GRPCSidecarBackend (replaces ssh.go)
  resolve.go           # ResolveBackend (updated metadata fields)

proto/gastown/v1/
  sidecar.proto        # SidecarService definition
  terminal.proto       # Existing (SendInput RPC survives)

deploy/sidecar/
  Dockerfile           # gt-sidecar + tmux + agent tools (no sshd)

deploy/k8s/
  sidecar-pod.yaml     # Pod spec with gRPC port, health probes
```

## Implementation Sequence

### Phase A: Proto + Sidecar Binary (no external dependencies)

1. Define `proto/gastown/v1/sidecar.proto`
2. Generate Go code with `buf generate`
3. Implement `internal/sidecar/server.go` (wraps tmux operations)
4. Implement `internal/sidecar/monitor.go` (session health)
5. Create `cmd/gt-sidecar/main.go` (entrypoint)

### Phase B: Client-Side Integration

6. Create `internal/terminal/grpc_sidecar.go` (GRPCSidecarBackend)
7. Update `internal/terminal/resolve.go` (sidecar_host/sidecar_port)
8. Update `internal/cmd/peek.go` (type assertion → GRPCSidecarBackend)
9. Update `internal/cmd/nudge.go` (type assertion → GRPCSidecarBackend)

### Phase C: Container + K8s

10. Create `deploy/sidecar/Dockerfile`
11. Create `deploy/k8s/sidecar-pod.yaml`
12. Remove SSH artifacts (ssh.go, polecat-ssh-keys.yaml)

### Phase D: End-to-End Testing (requires Phase 4)

13. Deploy sidecar pod to K8s
14. Verify `gt peek` and `gt nudge` work against remote pod
15. Load test concurrent peek/nudge operations

## Relationship to Other Phases

- **Phase 4** (remote polecats): Must set bead metadata `backend=k8s`,
  `sidecar_host`, `sidecar_port` when spawning K8s pods. Sidecar depends
  on Phase 4 for end-to-end testing but can be developed in parallel.

- **Future: K8s informers**: Replace polling-based agent discovery with
  K8s watch/informer pattern. The sidecar's Status RPC provides the health
  data; informers would handle discovery and lifecycle events.

- **Future: gRPC to BD daemon**: Replace `bd` CLI shelling (197+ call sites)
  with proper gRPC client to BD daemon. Orthogonal to sidecar but part of
  the same "gRPC everywhere" direction.

## Deprecated by This Design

- `docs/design/terminal-server.md` — bridge architecture (kubectl exec + screen + local tmux)
- `internal/terminal/ssh.go` — SSH backend
- `deploy/k8s/polecat-ssh-keys.yaml` — SSH key Secret
- `deploy/polecat/entrypoint.sh` — SSH-based entrypoint
