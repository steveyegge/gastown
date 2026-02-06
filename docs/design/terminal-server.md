# Terminal Server Architecture Design

Design document for the Gas Town terminal server component (Phase C of the K8s epic).

## Problem

Gas Town agents on K8s run inside pods, each with a screen session hosting
Claude Code. The existing `gt nudge` and `gt peek` commands assume local tmux
sessions. A bridge is needed: something that connects local tmux windows to
remote pod screen sessions, so all existing gt commands work unchanged.

## Design Decisions

### D1: Deployment Model — gt subcommand (standalone binary)

**Decision**: Terminal server runs as `gt terminal-server` — a long-running Go
process that manages connections to all agent pods in a rig.

**Alternatives considered**:

| Option | Pros | Cons |
|--------|------|------|
| **gt subcommand** (chosen) | Single binary, shares gt codebase, no separate deploy | Must run somewhere with kubectl access |
| Sidecar of gastown chart | Co-located with beads infra | Wrong abstraction boundary, gastown chart is data plane |
| Standalone K8s Deployment | Self-contained, scales independently | Extra binary to build/deploy, duplicates gt code |

**Rationale**: The terminal server is fundamentally a gt operation — it bridges
gt CLI commands to K8s pods. It needs the Connection interface, tmux wrapper,
and beads client, all of which live in the gt codebase. Running as a gt
subcommand means zero additional binaries and the same deployment path as gt
itself.

**Deployment**: Runs on any machine with kubectl access to the cluster. For
production, deploy as a K8s Deployment in the `gastown-test` namespace with a
ServiceAccount that has pod exec permissions.

### D2: Connection Protocol — kubectl exec

**Decision**: Use `kubectl exec -it <pod> -- screen -x <session>` to attach to
pod screen sessions, piped through a local tmux pane.

**Alternatives considered**:

| Option | Pros | Cons |
|--------|------|------|
| **kubectl exec** (chosen) | Zero extra networking, RBAC built-in, works out of box | Depends on K8s API server, slightly higher latency |
| Direct TCP to pod | Lower latency, simpler pipe | Needs NetworkPolicy, custom auth, firewall holes |
| WebSocket proxy | Web UI possible | Massive complexity, custom server in pod |

**Rationale**: kubectl exec uses the K8s API server as the transport, which
means:
- No additional networking (pods don't need to be exposed)
- Authentication via kubeconfig (already configured)
- Authorization via RBAC (already in place for the controller)
- Encryption via TLS (API server handles it)
- Works through NATs, firewalls, and VPNs transparently

The latency budget for nudge/peek is generous (seconds, not milliseconds).
kubectl exec adds ~50-100ms which is invisible in practice.

### D3: Tmux Mapping — One tmux window per pod, one session per rig

**Decision**: Terminal server creates a tmux session named `gt-ts-<rig>` with
one window per agent pod. Window names match the agent session naming convention.

```
tmux session: gt-ts-gastown
├── window 0: gt-gastown-witness     → kubectl exec witness-pod -- screen -x agent
├── window 1: gt-gastown-refinery    → kubectl exec refinery-pod -- screen -x agent
├── window 2: gt-gastown-crew-k8s    → kubectl exec crew-k8s-pod -- screen -x agent
├── window 3: gt-gastown-alpha       → kubectl exec alpha-pod -- screen -x agent
└── window 4: gt-gastown-bravo       → kubectl exec bravo-pod -- screen -x agent
```

**Why one session per rig**: Matches existing rig-scoped operations. `gt peek gastown/alpha`
resolves to tmux window `gt-gastown-alpha` inside session `gt-ts-gastown`.

**Why one window per pod**: Each kubectl exec subprocess gets its own tmux
window. Windows can be killed and recreated independently when connections drop.

**Backward compatibility**: Existing gt commands find sessions by name. The
terminal server creates windows with the same names as the local tmux sessions
would have. `gt nudge gastown/alpha` calls `tmux send-keys -t gt-gastown-alpha`
which resolves to the correct window.

**Key insight**: tmux windows within a session are addressable by name with the
`:<window>` suffix. But gt currently addresses sessions directly. The terminal
server exposes agent windows as **tmux sessions** (not windows inside a session)
to maintain backward compatibility:

```
# Each agent gets its own tmux session, piped to kubectl exec
tmux session: gt-gastown-alpha      → kubectl exec alpha-pod -- screen -x agent
tmux session: gt-gastown-bravo      → kubectl exec bravo-pod -- screen -x agent
```

This is the simplest approach — one tmux session per agent, exactly matching the
existing naming convention. The session's pane runs `kubectl exec`.

### D4: Discovery — Poll beads for pod list

**Decision**: Terminal server polls beads at a configurable interval (default 10s)
to discover agent pods.

**Alternatives considered**:

| Option | Pros | Cons |
|--------|------|------|
| **Poll beads** (chosen) | Simple, beads is already source of truth | Polling interval adds latency to discovery |
| Watch K8s API | Real-time events | Bypasses beads as source of truth, complex |
| Event-driven via bd bus | Instant notification | bd bus not yet mature enough (od-k3o epic) |
| NATS pub/sub | Real-time, scalable | Extra infrastructure dependency |

**Rationale**: Beads already stores agent pod information (pod_name, pod_ip,
pod_status fields added in Phase B). Polling beads maintains beads as the single
source of truth. The controller updates beads when pods change; the terminal
server reads beads to learn about pods. Clean separation of concerns.

**Future**: When bd bus matures (od-k3o epic), terminal server can subscribe to
agent lifecycle events for instant notification. The polling fallback remains for
reliability.

### D5: Reconnection Strategy — Kill and recreate tmux session

**Decision**: When a kubectl exec connection drops (network blip, pod restart,
API server timeout), the terminal server kills the local tmux session and
creates a new one with a fresh kubectl exec.

**Why not reconnect**: kubectl exec connections are not resumable. Once the
WebSocket closes, a new exec must be established. Screen inside the pod
preserves agent state — the agent process is unaffected by connection drops.

**Recovery flow**:
```
1. kubectl exec drops (connection closed)
2. Terminal server detects broken pipe / EOF on the tmux pane
3. Terminal server kills tmux session (gt-gastown-alpha)
4. Terminal server creates new tmux session with new kubectl exec
5. Screen reattach (-x) reconnects to the running agent session
6. Agent is unaware of the reconnection (screen buffered all I/O)
```

**Detection**: Terminal server monitors pane liveness using `tmux list-panes -F '#{pane_dead}'`.
Dead panes indicate a dropped kubectl exec. The `remain-on-exit` option keeps
the pane visible for debugging before recreation.

### D6: Backward Compatibility — Transparent routing via Connection interface

**Decision**: Implement `K8sConnection` that satisfies the existing `Connection`
interface. Terminal server is a singleton that gt commands can discover.

**How existing commands route to terminal server**:

```
gt nudge gastown/alpha "hello"
  ↓
1. Resolve target → session name "gt-gastown-alpha"
2. Check: Is session local? → tmux has-session gt-gastown-alpha
3. If yes (terminal server created it): tmux send-keys works as-is
4. If no: fall back to error (pod not connected)
```

**Key insight**: gt nudge and gt peek already use tmux by session name. The
terminal server creates tmux sessions that exactly match the expected names.
No changes to nudge/peek are required. The terminal server is invisible to
them — it just ensures the right tmux sessions exist.

The only change needed is in `gt sling` when `--target=k8s` is specified
(Phase E), which routes through the controller instead of local tmux.

### D7: Connection Interface — New K8sConnection type

**Decision**: Add `K8sConnection` to `internal/connection/` that implements
the `Connection` interface for K8s pods.

```go
type K8sConnection struct {
    podName   string
    namespace string
    container string
    tmux      *tmux.Tmux
}
```

**Method implementations**:

| Method | Implementation |
|--------|---------------|
| `Name()` | Returns pod name |
| `IsLocal()` | Returns false |
| `ReadFile(path)` | `kubectl exec <pod> -- cat <path>` |
| `WriteFile(path, data)` | `kubectl exec <pod> -i -- tee <path>` |
| `MkdirAll(path)` | `kubectl exec <pod> -- mkdir -p <path>` |
| `Remove(path)` | `kubectl exec <pod> -- rm <path>` |
| `Exec(cmd, args)` | `kubectl exec <pod> -- <cmd> <args>` |
| `TmuxSendKeys(session, keys)` | Via local tmux session (terminal server) |
| `TmuxCapturePane(session, lines)` | Via local tmux session (terminal server) |
| `TmuxHasSession(name)` | Check local tmux (terminal server manages) |

**Tmux operations route locally**: The terminal server's tmux sessions are
local. `TmuxSendKeys("gt-gastown-alpha", "hello")` sends to the local tmux
session that pipes to the pod via kubectl exec. No special K8s tmux handling
needed.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Terminal Server Process (gt terminal-server)                 │
│                                                              │
│  ┌─────────────┐    ┌──────────────────────────────────┐    │
│  │ Discovery    │    │ Connection Manager                │    │
│  │ Loop         │    │                                    │    │
│  │              │    │  ┌────────────────────────────┐   │    │
│  │ Poll beads   │───→│  │ For each discovered agent: │   │    │
│  │ every 10s    │    │  │                            │   │    │
│  │              │    │  │ 1. Create tmux session     │   │    │
│  │ bd agent     │    │  │    (gt-<rig>-<name>)       │   │    │
│  │ list --pod   │    │  │                            │   │    │
│  │              │    │  │ 2. Run kubectl exec in pane│   │    │
│  └─────────────┘    │  │    → screen -x agent       │   │    │
│                      │  │                            │   │    │
│  ┌─────────────┐    │  │ 3. Monitor pane liveness   │   │    │
│  │ Health       │    │  │    (pane_dead check)       │   │    │
│  │ Monitor      │    │  │                            │   │    │
│  │              │    │  │ 4. Reconnect on failure    │   │    │
│  │ Check pane   │    │  └────────────────────────────┘   │    │
│  │ dead status  │    │                                    │    │
│  │ every 5s     │    └──────────────────────────────────┘    │
│  └─────────────┘                                             │
└──────────┬───────────────────────────────────────────────────┘
           │
           │  kubectl exec -it <pod> -n gastown-test -- screen -x agent
           │
┌──────────▼───────────────────────────────────────────────────┐
│  K8s Cluster (gastown-test namespace)                         │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐            │
│  │ witness-pod│  │ alpha-pod  │  │ bravo-pod  │   ...       │
│  │ ┌────────┐ │  │ ┌────────┐ │  │ ┌────────┐ │            │
│  │ │ screen │ │  │ │ screen │ │  │ │ screen │ │            │
│  │ │└claude │ │  │ │└claude │ │  │ │└claude │ │            │
│  │ └────────┘ │  │ └────────┘ │  │ └────────┘ │            │
│  └────────────┘  └────────────┘  └────────────┘            │
└──────────────────────────────────────────────────────────────┘
```

## Component Interactions

```
gt nudge gastown/alpha "hello"
  │
  ├─ tmux send-keys -t gt-gastown-alpha -l "hello"
  │    │
  │    └─ tmux session gt-gastown-alpha (managed by terminal server)
  │         │
  │         └─ pane running: kubectl exec alpha-pod -n gastown-test -it -- screen -x agent
  │              │
  │              └─ screen session inside pod receives keystroke
  │                   │
  │                   └─ Claude Code process receives input
  │
  └─ (no change to gt nudge code)


gt peek gastown/alpha
  │
  ├─ tmux capture-pane -t gt-gastown-alpha -p -S -100
  │    │
  │    └─ Returns content from the local tmux pane
  │         (which mirrors the kubectl exec / screen output)
  │
  └─ (no change to gt peek code)


gt sling <bead> gastown  (Phase E, K8s mode)
  │
  ├─ Controller creates pod (via bead state change)
  │    │
  │    └─ Controller registers pod in beads (pod_name, pod_ip, etc.)
  │
  ├─ Terminal server discovers new pod (next poll cycle, ≤10s)
  │    │
  │    └─ Creates tmux session gt-gastown-<polecat>
  │         │
  │         └─ Runs kubectl exec <pod> -n gastown-test -it -- screen -x agent
  │
  └─ gt nudge gastown/<polecat> works immediately after session exists
```

## API Surface

### CLI: `gt terminal-server`

```bash
# Start terminal server for a rig
gt terminal-server --rig gastown --namespace gastown-test

# With custom poll interval
gt terminal-server --rig gastown --namespace gastown-test --poll-interval 5s

# With kubeconfig
gt terminal-server --rig gastown --namespace gastown-test --kubeconfig ~/.kube/config

# Dry run (show what would be connected, don't create sessions)
gt terminal-server --rig gastown --namespace gastown-test --dry-run
```

### Configuration

Terminal server settings in rig config (`gt config`):

```yaml
k8s:
  enabled: true
  namespace: gastown-test
  terminal_server:
    poll_interval: 10s      # Beads discovery interval
    health_interval: 5s     # Pane liveness check interval
    reconnect_delay: 2s     # Delay before reconnecting dropped session
    max_reconnect: 5        # Max consecutive reconnect attempts
    screen_session: "agent" # Screen session name inside pods
```

### Internal API

```go
// internal/terminal/server.go

type Server struct {
    rig           string
    namespace     string
    kubeconfig    string
    pollInterval  time.Duration
    healthInterval time.Duration
    tmux          *tmux.Tmux
    connections   map[string]*PodConnection  // agentID → connection
    mu            sync.RWMutex
}

type PodConnection struct {
    agentID       string
    podName       string
    sessionName   string     // gt-<rig>-<name>
    connected     bool
    lastConnected time.Time
    reconnectCount int
}

func NewServer(cfg ServerConfig) *Server
func (s *Server) Run(ctx context.Context) error     // Main loop
func (s *Server) discover() ([]AgentPod, error)     // Poll beads
func (s *Server) reconcile(pods []AgentPod)          // Create/remove sessions
func (s *Server) connect(pod AgentPod) error         // Create tmux + kubectl exec
func (s *Server) disconnect(agentID string) error    // Kill tmux session
func (s *Server) healthCheck()                        // Check pane liveness
func (s *Server) reconnect(pc *PodConnection) error  // Reconnect dropped session
```

## Failure Modes and Recovery

| Failure | Detection | Recovery | Impact |
|---------|-----------|----------|--------|
| kubectl exec drops | `pane_dead=1` in tmux | Kill session, recreate with new exec | Agent unaffected (screen preserves state) |
| Pod restarted | Beads poll shows new pod_name | Old session killed, new session for new pod | Agent process restarts (pod restart killed it) |
| Pod deleted | Beads poll shows agent gone | Kill orphaned tmux session | Session cleaned up |
| API server unreachable | kubectl exec hangs/fails | Exponential backoff on reconnect | Existing sessions continue (already connected) |
| Terminal server crash | No tmux sessions for K8s agents | Restart terminal server; all sessions recreated | Agents unaffected (screen preserves state) |
| Beads daemon unreachable | Discovery poll fails | Retry with backoff; maintain existing sessions | No new pods discovered, existing connections fine |
| Screen session gone in pod | kubectl exec connects but screen -x fails | Log error, retry on next health cycle | Indicates pod problem, not terminal server issue |
| tmux server crash | All sessions lost | Terminal server detects on next health check, recreates all | Brief disruption, full auto-recovery |

### Reconnection Backoff

```
Attempt 1: immediate
Attempt 2: 2s delay
Attempt 3: 4s delay
Attempt 4: 8s delay
Attempt 5: 16s delay
After 5 failures: log error, skip until next discovery cycle
```

## Performance Characteristics

### Latency Budget for gt nudge

```
gt nudge gastown/alpha "hello"
  │
  ├─ tmux send-keys (local)          ~1ms
  ├─ kubectl exec pipe (established) ~0ms  (already connected)
  ├─ screen relay inside pod         ~1ms
  └─ Total: ~2ms (same as local tmux)

Note: The kubectl exec connection is already established.
The tmux pane is already piped to the pod's screen session.
Sending keys is just writing to the tmux pane — no new network calls.
```

### Latency Budget for gt peek

```
gt peek gastown/alpha
  │
  ├─ tmux capture-pane (local)       ~5ms
  └─ Total: ~5ms (same as local tmux)

Note: Capture reads from the local tmux pane buffer.
The buffer is continuously updated by the kubectl exec pipe.
No network call needed for capture.
```

### Discovery Latency

```
New pod → visible to terminal server: ≤ poll_interval (default 10s)
New pod → tmux session created: ≤ poll_interval + connect time (~1s)
New pod → nudge-able: ≤ 11s
```

### Resource Usage

- One tmux session per K8s agent (~1MB per session)
- One kubectl exec process per agent (~10MB RSS each)
- Beads poll: one bd query per poll cycle
- Health check: one `tmux list-panes` per cycle

For 50 agents: ~550MB RSS, negligible CPU.

## Integration with Existing gt Commands

### Commands That Work Unchanged

| Command | Why it works |
|---------|-------------|
| `gt nudge <agent>` | Finds tmux session by name (terminal server created it) |
| `gt peek <agent>` | Captures from tmux pane (terminal server keeps it updated) |
| `gt polecat list` | Reads from beads (controller populates pod info) |
| `gt status` | Reads agent beads (unchanged) |
| `gt mail` | Routes through beads (unchanged) |

### Commands That Need Changes (Phase E)

| Command | Change needed |
|---------|--------------|
| `gt sling` | Add `--target=k8s` to route through controller instead of local tmux |
| `gt polecat nuke` | Delete K8s pod instead of killing local tmux session |
| `gt session` | Show K8s pod info alongside tmux session info |
| `gt done` | Signal controller for pod cleanup (in addition to MQ submit) |

### Commands That Are Obsoleted for K8s Agents

| Command | Replaced by |
|---------|-------------|
| Local worktree creation | Pod init container handles workspace |
| Local screen/tmux setup | Pod entrypoint handles screen session |

## File Structure

```
internal/
├── terminal/
│   ├── server.go          # Main terminal server loop
│   ├── server_test.go     # Unit tests
│   ├── discovery.go       # Beads polling for pod inventory
│   ├── discovery_test.go
│   ├── connection.go      # PodConnection management
│   └── connection_test.go
├── connection/
│   ├── k8s.go            # K8sConnection implementing Connection interface
│   └── k8s_test.go
└── cmd/
    └── terminal_server.go # gt terminal-server subcommand
```

## Configuration Defaults

```go
const (
    DefaultPollInterval    = 10 * time.Second
    DefaultHealthInterval  = 5 * time.Second
    DefaultReconnectDelay  = 2 * time.Second
    DefaultMaxReconnect    = 5
    DefaultScreenSession   = "agent"
    DefaultNamespace       = "gastown-test"
)
```

## Security Considerations

- Terminal server needs a kubeconfig with pod exec permissions in `gastown-test`
- RBAC: ServiceAccount with `pods/exec` verb on pods in the target namespace
- No additional network exposure — all traffic goes through K8s API server
- Agent credentials (API keys) are in pod secrets, not on the terminal server
- Terminal server sees agent I/O (unavoidable for tmux bridging) — run it in
  a trusted context

## Open Questions for Phase E

1. **Multi-rig**: Should one terminal server handle multiple rigs, or one per rig?
   Current design: one per rig, for simplicity and isolation.

2. **Terminal server HA**: If the terminal server crashes, all K8s agent tmux
   sessions are lost (though agents themselves are fine). Should we run two
   replicas? Leader election adds complexity.

3. **Local + K8s hybrid**: When a rig has both local and K8s agents, gt commands
   need to transparently handle both. The naming convention makes this work
   (same tmux session names), but operations like `gt polecat nuke` need to
   know which path to take.
