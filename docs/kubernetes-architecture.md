# Kubernetes Architecture: Beads as the Control Plane

> **Date**: 2026-02-12
> **Scope**: Cross-repo architecture (gastown + beads + coop)
> **Audience**: Contributors, operators, and future maintainers

## Executive Summary

Gas Town runs AI agents as Kubernetes pods, using **beads** (the git-backed issue tracker)
as the control plane. There are no CRDs. No custom operators watching K8s resources.
Instead, the beads daemon is the single source of truth for which agents should exist,
and a thin controller translates bead state into pod operations.

The architecture follows a simple principle: **a bead IS an agent**. Creating an agent
bead with the right labels causes a pod to appear. Closing the bead causes the pod to
disappear. Everything in between -- health checks, status sync, event distribution,
session management -- flows through the daemon's RPC API and NATS event bus.

```
Human creates bead ──→ Daemon stores in Dolt ──→ Controller creates pod
         ↑                                              │
         │              Status sync                     │
         └──────────── Daemon updates bead ←────────────┘
```

---

## Table of Contents

1. [System Components](#1-system-components)
2. [The Bead-First Model](#2-the-bead-first-model)
3. [Daemon: The Control Plane](#3-daemon-the-control-plane)
4. [Agent Controller: The Reactive Bridge](#4-agent-controller-the-reactive-bridge)
5. [Agent Pods: Anatomy of a Worker](#5-agent-pods-anatomy-of-a-worker)
6. [Coop: Terminal Session Management](#6-coop-terminal-session-management)
7. [Event Distribution: NATS JetStream](#7-event-distribution-nats-jetstream)
8. [Session Registry: Cross-Backend Discovery](#8-session-registry-cross-backend-discovery)
9. [Helm Deployment Topology](#9-helm-deployment-topology)
10. [Data Flow: End-to-End Lifecycle](#10-data-flow-end-to-end-lifecycle)
11. [Security Model](#11-security-model)
12. [Operational Reference](#12-operational-reference)

---

## 1. System Components

The system spans three repositories with distinct responsibilities:

| Repository | Language | Role |
|------------|----------|------|
| **beads** | Go | Control plane: daemon, RPC server, storage (Dolt), event bus, CLI |
| **gastown** | Go | Orchestration: controller, agent image, entrypoint, Helm charts, session registry, backend interface |
| **coop** | Rust | Agent sidecar: PTY management, HTTP API, NATS integration, health endpoints |

### Deployed Components (in-cluster)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        KUBERNETES CLUSTER                               │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  BD Daemon Pod                                                    │  │
│  │  ├─ bd-daemon container (beads binary)                            │  │
│  │  │   ├─ RPC server (TCP :9876, HTTP :9080)                        │  │
│  │  │   ├─ Embedded NATS server (:4222, JetStream)                   │  │
│  │  │   └─ Event bus (local dispatch + JetStream publish)            │  │
│  │  └─ slack-bot sidecar (optional)                                  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                              │                                          │
│                    Dolt SQL (:3306)                                      │
│                              │                                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  Dolt StatefulSet (dolthub/dolt-sql-server)                       │  │
│  │  └─ Beads database: issues, labels, dependencies, config          │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  Agent Controller Deployment                                      │  │
│  │  ├─ Watches daemon events (SSE or NATS)                           │  │
│  │  ├─ Periodic reconciliation (60s)                                 │  │
│  │  └─ Creates/deletes agent pods via K8s API                        │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                              │                                          │
│               Creates/deletes pods                                      │
│                              │                                          │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐              │
│  │  Agent Pod 1  │  │  Agent Pod 2  │  │  Agent Pod N  │   ...        │
│  │  (crew-k8s)   │  │  (polecat-x)  │  │  (witness)    │              │
│  │  ┌──────────┐ │  │  ┌──────────┐ │  │  ┌──────────┐ │              │
│  │  │  Agent   │ │  │  │  Agent   │ │  │  │  Agent   │ │              │
│  │  │Container │ │  │  │Container │ │  │  │Container │ │              │
│  │  └──────────┘ │  │  └──────────┘ │  │  └──────────┘ │              │
│  │  ┌──────────┐ │  │  ┌──────────┐ │  │  ┌──────────┐ │              │
│  │  │   Coop   │ │  │  │   Coop   │ │  │  │   Coop   │ │              │
│  │  │ Sidecar  │ │  │  │ Sidecar  │ │  │  │ Sidecar  │ │              │
│  │  └──────────┘ │  │  └──────────┘ │  │  └──────────┘ │              │
│  └───────────────┘  └───────────────┘  └───────────────┘              │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  NATS Server (embedded in daemon or standalone)                    │  │
│  │  └─ JetStream streams: HOOK_EVENTS, MUTATION_EVENTS               │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌──────────────────┐  ┌──────────────────┐                            │
│  │  Git Mirror(s)   │  │  Coop Broker     │  (optional)                │
│  │  (bare repos)    │  │  (OAuth + mux)   │                            │
│  └──────────────────┘  └──────────────────┘                            │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. The Bead-First Model

The central design insight: **agent beads ARE the desired state**. There is no separate
CRD, no ConfigMap, no annotation-based scheme. The beads issue database is the
authoritative record of which agents should exist.

### Agent Bead Anatomy

An agent bead is a regular beads issue with specific labels and metadata:

```
ID:          gt-gastown-crew-k8s
Title:       crew-k8s
Type:        task (or agent)
Status:      in_progress
Labels:      gt:agent, execution_target:k8s, rig:gastown, role:crew, agent:k8s
Agent State: working
```

**Required labels for K8s execution:**
- `gt:agent` -- marks this issue as an agent (not work)
- `execution_target:k8s` -- signals the controller to create a pod

**Identity labels** (extracted by controller for pod naming and routing):
- `rig:<name>` -- which rig (repository) this agent works on
- `role:<type>` -- mayor, deacon, crew, polecat, witness, refinery
- `agent:<name>` -- unique name within the role

**Pod metadata fields** (written by controller's status reporter):
```go
PodName       string  // K8s pod name (e.g., "gt-gastown-crew-k8s")
PodIP         string  // Pod IP for Coop endpoint resolution
PodNode       string  // K8s node name
PodStatus     string  // pending | running | terminating | terminated
ScreenSession string  // Coop session name inside the pod
```

**Agent lifecycle fields:**
```go
AgentState    string     // spawning | working | done | stuck | dead
LastActivity  *time.Time // Heartbeat for timeout detection
HookBead      string     // Currently assigned work item
RoleType      string     // polecat | crew | witness | refinery | mayor | deacon
```

### State Machine

```
             ┌────────────────────────────────────────┐
             │                                        │
  bd create  │   Controller         Agent running     │   bd close
  ──────────→ spawning ──────────→ working ──────────→ done ──────→ (pod deleted)
             │         pod created       │            │
             │                           │            │
             │                    timeout│            │
             │                    (15m)  │            │
             │                           ↓            │
             │                        stuck ──────────┘
             │                           │   witness escalation
             │                           ↓
             │                         dead
             └────────────────────────────────────────┘
```

---

## 3. Daemon: The Control Plane

The beads daemon (`bd daemon start`) is the heart of the control plane. It runs as a
single-replica Deployment in the cluster and provides:

### RPC Server

**Location**: `beads/internal/rpc/`

The daemon exposes 100+ RPC operations over two transports:

| Transport | Port | Protocol | Used By |
|-----------|------|----------|---------|
| TCP (Unix/TCP socket) | 9876 | Custom binary RPC | `bd` CLI in agent pods |
| HTTP | 9080 | JSON-over-HTTP (Connect-RPC style) | Controller, external tools |

**Agent-specific operations:**

```go
OpAgentPodRegister   = "agent_pod_register"    // Pod registers itself on startup
OpAgentPodDeregister = "agent_pod_deregister"  // Pod clears metadata on shutdown
OpAgentPodStatus     = "agent_pod_status"      // Periodic status update
OpAgentPodList       = "agent_pod_list"        // Query all active agent pods
```

**Core bead operations that trigger K8s actions:**

```go
OpCreate  // Creating an agent bead → controller spawns pod
OpUpdate  // Status change → controller may act
OpClose   // Closing agent bead → controller deletes pod
OpDelete  // Deleting agent bead → controller deletes pod
```

**Key files:**
- `beads/internal/rpc/protocol.go` -- Operation constants and request/response types
- `beads/internal/rpc/server_agent_pod.go` -- Agent pod RPC handlers
- `beads/internal/rpc/server_issues_epics.go` -- Core CRUD handlers (~2,200 lines)
- `beads/internal/rpc/http_server.go` -- HTTP transport (used by controller)
- `beads/internal/rpc/client.go` -- Client used by `bd` CLI

### Storage: Dolt

The daemon connects to a Dolt SQL server for persistent storage:

```
Daemon → Dolt SQL (:3306) → issues table (beads)
                           → config table (deploy.* keys)
                           → labels, dependencies, comments
```

Dolt provides git-like versioning of the database, with optional S3 remote backup.
The daemon handles auto-commit, sync, and garbage collection.

### Embedded NATS Server

**Location**: `beads/internal/daemon/nats.go`

The daemon starts an embedded NATS server with JetStream for event distribution:

```go
type NATSServer struct {
    server   *server.Server  // Embedded NATS
    conn     *nats.Conn      // In-process connection
    storeDir string          // JetStream persistence
    port     int             // TCP :4222
}
```

On startup, the daemon writes `nats-info.json` so sidecars can discover the NATS endpoint:

```json
{
    "url": "nats://127.0.0.1:4222",
    "port": 4222,
    "token": "<BD_DAEMON_TOKEN>",
    "jetstream": true,
    "stream": "HOOK_EVENTS",
    "subjects": "hooks.>"
}
```

### Deploy Configuration

**Location**: `beads/internal/config/deploy.go`

K8s-specific configuration lives in the Dolt config table under `deploy.*` keys:

| Key | Env Var | Purpose |
|-----|---------|---------|
| `deploy.dolt_host` | `BEADS_DOLT_SERVER_HOST` | Dolt service hostname |
| `deploy.dolt_port` | `BEADS_DOLT_SERVER_PORT` | Dolt port (3306) |
| `deploy.daemon_tcp_addr` | `BD_DAEMON_TCP_ADDR` | RPC listener (:9876) |
| `deploy.daemon_http_addr` | `BD_DAEMON_HTTP_ADDR` | HTTP listener (:9080) |
| `deploy.nats_url` | `BD_NATS_URL` | NATS endpoint |
| `deploy.redis_url` | `BD_REDIS_URL` | Redis for ephemeral state |

---

## 4. Agent Controller: The Reactive Bridge

**Location**: `gastown/controller/`

The controller is a standalone Go binary that bridges beads state to K8s pod operations.
It is intentionally thin -- no CRDs, no controller-runtime, no informers. All intelligence
lives in beads.

### Architecture

```go
// controller/cmd/controller/main.go
func main() {
    // 1. Connect to daemon (HTTP client)
    daemon := daemonclient.New(cfg)

    // 2. Connect to K8s (pod CRUD)
    pods := podmanager.New(k8sClient, namespace)

    // 3. Start beads watcher (SSE or NATS)
    watcher := beadswatcher.New(daemon, eventHandler)

    // 4. Start reconciler (periodic sync)
    reconciler := reconciler.New(daemon, pods, interval)

    // 5. Start status reporter (pod → bead sync)
    reporter := statusreporter.New(daemon, pods)
}
```

### Two Paths to Pod Creation

**Path A: Event-driven (real-time)**

The `beadswatcher` subscribes to daemon mutation events and reacts immediately:

```
Daemon mutation event (SSE or NATS)
    ↓
beadswatcher.handleEvent()
    ↓
Map to lifecycle event:
  - Mutation "create" on gt:agent bead     → AgentSpawn
  - Status change to "in_progress"         → AgentSpawn (reactivation)
  - Status change to "closed"              → AgentDone
  - Agent state "stuck"                    → AgentStuck
    ↓
podmanager.CreateAgentPod() or DeleteAgentPod()
```

**Path B: Reconciler (periodic, every 60s)**

Convergence loop that catches anything the event path missed:

```
reconciler.Reconcile()
    ↓
Desired state: daemon.ListAgentBeads()
  → POST /bd.v1.BeadsService/List
  → {exclude_status: ["closed"], labels: ["gt:agent", "execution_target:k8s"]}
    ↓
Actual state: pods.ListAgentPods()
  → K8s API: list pods with label app=gastown-agent
    ↓
Diff and converge:
  - Desired but no pod → Create pod
  - Pod but not desired → Delete pod (orphan)
  - Pod failed → Recreate
```

### Daemon Client

**Location**: `gastown/controller/internal/daemonclient/client.go`

The controller talks to the daemon via HTTP, using the same Connect-RPC style API:

```go
type DaemonClient struct {
    baseURL    string        // http://bd-daemon:9080
    token      string        // BD_DAEMON_TOKEN
    httpClient *http.Client
}

// Core queries
func (c *DaemonClient) ListAgentBeads(ctx) ([]AgentBead, error)
func (c *DaemonClient) ListRigBeads(ctx) (map[string]RigInfo, error)
func (c *DaemonClient) UpdateBeadNotes(ctx, beadID, notes string) error
```

`ListAgentBeads` queries the daemon for all non-closed agent beads with `execution_target:k8s`,
extracts identity from labels (`rig:X`, `role:Y`, `agent:Z`), and returns structured
`AgentBead` objects the pod manager uses for pod naming and configuration.

### Status Reporter

The status reporter provides the reverse sync: K8s pod status back to beads.

```
K8s Pod Phase        →   Bead Agent State
─────────────────────────────────────────
Pending              →   spawning
Running              →   working
Succeeded            →   done
Failed               →   stuck (triggers witness escalation)
```

It also writes backend metadata to the bead's notes field:
```
backend: coop
coop_url: http://10.0.1.5:8080
pod_name: gt-gastown-crew-k8s
pod_namespace: gastown
```

This metadata is how other components (session registry, `gt peek`, `gt nudge`) discover
the Coop endpoint for a given agent.

### Controller Configuration

```
--namespace              K8s namespace (refuses "gastown" as safety)
--interval              Reconcile interval (default 60s)
--stale-timeout         Agent stale threshold (default 15m)
--daemon-addr           BD_DAEMON_HOST
--daemon-token          BD_DAEMON_TOKEN
--agent-image           Default agent container image
--coop-image            Coop sidecar image (optional)
--coop-builtin          Use coop compiled into agent image
```

---

## 5. Agent Pods: Anatomy of a Worker

**Location**: `gastown/deploy/agent/`

### Image Build

The agent image (`ghcr.io/groblegark/gastown/gastown-agent`) is a multi-stage Docker build:

1. **Build stage**: Compiles `gt` CLI from gastown source
2. **Binary stage**: Downloads `coop` and `bd` binaries from GitHub releases
3. **Final stage**: Ubuntu 24.04 with `gt`, `coop`, `bd`, Claude CLI, and `entrypoint.sh`

### Pod Spec

The pod manager (`gastown/controller/internal/podmanager/`) generates pod specs:

**Main container** (agent):
- Image: `ghcr.io/groblegark/gastown/gastown-agent:latest`
- User: agent (UID 1000)
- Working directory: `/home/agent/gt`
- Entry: `/entrypoint.sh`
- Environment:
  ```
  GT_ROLE=crew          GT_AGENT=k8s         GT_RIG=gastown
  BD_DAEMON_HOST=bd-daemon-0.ns.svc.cluster.local
  BD_DAEMON_PORT=9876   BD_DAEMON_HTTP_PORT=9080
  BD_DAEMON_TOKEN=<from secret>
  ANTHROPIC_API_KEY=<from secret>
  ```

**Coop sidecar** (optional, or builtin):
- Ports: 8080 (API), 9090 (health)
- `shareProcessNamespace: true` (to observe agent process)
- Health probes target coop endpoints

**Init containers** (optional):
- `init-clone`: Clones from in-cluster git mirror
- Toolchain sidecar: `restartPolicy: Always` for persistent tooling

**Volumes:**
- Crew roles: PVC (persistent workspace across restarts)
- Polecat roles: EmptyDir (ephemeral, destroyed with pod)
- Home directory: Symlinked state (`~/.claude` → PVC `.state/claude/`)

**Security context:**
```yaml
runAsUser: 1000
runAsGroup: 1000
runAsNonRoot: true
fsGroup: 1000
shareProcessNamespace: true  # if coop sidecar
```

### Health Probes

All probes target the Coop HTTP API:

| Probe | Endpoint | Period | Timeout | Failures |
|-------|----------|--------|---------|----------|
| **Startup** | `GET /api/v1/health` (port 9090) | 5s | 5s | 30 (150s total) |
| **Liveness** | `GET /api/v1/health` (port 9090) | 30s | 5s | 5 |
| **Readiness** | `GET /api/v1/health` (port 9090) | 10s | 5s | 5 |

The generous startup probe (150s) accommodates Claude CLI boot time.

### Entrypoint Sequence

`deploy/agent/entrypoint.sh` runs the following on pod start:

```
1. Clean FIFO artifacts      find sessions -name 'hook.pipe' -delete
2. Init git repo             git init (if fresh PVC)
3. Create workspace          mayor/town.json, role dirs
4. Connect to daemon         gt connect --url http://daemon:9080
5. Persist Claude state      symlink ~/.claude → PVC .state/claude/
6. Register pod              bd agent pod-register <id> --pod-name=... --pod-ip=...
7. Launch agent              claude --dangerously-skip-permissions (with --resume if session exists)
```

Role-specific behavior:
- **Mayor/Deacon**: Town-level singletons at `${WORKSPACE}/${ROLE}/`
- **Crew**: Persistent workers at `${WORKSPACE}/crew/${AGENT}/`
- **Polecat**: Ephemeral workers (no persistent directory)

---

## 6. Coop: Terminal Session Management

**Repository**: coop (Rust, v0.7.x)

Coop is a PTY management sidecar that runs alongside (or inside) each agent container.
It provides an HTTP API for observing and controlling the agent's terminal session.

### Coop API

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/api/v1/health` | Health check (port 9090) |
| GET | `/api/v1/status` | Process status |
| GET | `/api/v1/agent/state` | Agent state (idle, busy) |
| GET | `/api/v1/screen` | Terminal text + cursor |
| GET | `/api/v1/screen/text` | Plain text capture |
| POST | `/api/v1/input` | Send text input |
| POST | `/api/v1/input/keys` | Send keystrokes |
| POST | `/api/v1/input/raw` | Send base64-encoded input |
| POST | `/api/v1/agent/nudge` | Message an idle agent |
| POST | `/api/v1/agent/respond` | Answer permission/plan prompts |
| POST | `/api/v1/signal` | Send Unix signal |
| PUT | `/api/v1/env/:key` | Set environment variable |
| GET | `/api/v1/env/:key` | Get environment variable |
| PUT | `/api/v1/session/switch` | Switch/respawn session |

### Two Operating Modes

**Builtin mode** (`COOP_BUILTIN=true`):
- Coop binary compiled into agent image
- Entrypoint starts coop which spawns/attaches to agent process
- Agent container exposes ports 8080 + 9090 directly
- Health probes use HTTP

**Sidecar mode** (`COOP_IMAGE=...`):
- Separate container in the pod
- `shareProcessNamespace: true` for PTY access
- Better isolation, independent updates
- Agent container uses exec probes

### Backend Interface

**Location**: `gastown/internal/terminal/backend.go`

The `Backend` interface abstracts terminal operations across transport mechanisms:

```go
type Backend interface {
    HasSession(session string) (bool, error)
    CapturePane(session string, lines int) (string, error)
    NudgeSession(session string, message string) error
    SendKeys(session string, keys string) error
    IsAgentRunning(session string) (bool, error)
    GetAgentState(session string) (string, error)
    SetEnvironment(session, key, value string) error
    SendInput(session string, text string, enter bool) error
    KillSession(session string) error
    AttachSession(session string) error
    // ... 17 methods total
}
```

**Implementations:**

| Backend | Transport | Status |
|---------|-----------|--------|
| **CoopBackend** | HTTP to Coop API | Active (K8s) |
| **TmuxBackend** | `tmux` CLI | Legacy (being removed) |
| **SSHBackend** | SSH tunnel | Legacy (being removed) |

The `CoopBackend` maps each session name to a Coop base URL (e.g., `http://10.0.1.5:8080`)
and translates Backend method calls to HTTP requests:

```go
// gastown/internal/terminal/coop.go
func (b *CoopBackend) NudgeSession(session, message string) error {
    url := b.sessions[session]  // "http://10.0.1.5:8080"
    return httpPost(url + "/api/v1/nudge", map[string]string{"message": message})
}
```

### Coop NATS Integration

Coop connects to the daemon's NATS server for bidirectional event flow:

```
Agent Coop ←──── NATS ────→ Daemon
  │                              │
  ├─ Subscribes to:              ├─ Publishes:
  │   hooks.>                    │   hooks.agent.*
  │   decisions.<agent>.*        │   mutations.*
  │                              │
  └─ Publishes:                  └─ Subscribes to:
      hooks.agent.<events>           (local handlers)
```

Configuration via environment:
```
COOP_NATS_URL=nats://bd-daemon-nats:4222
COOP_NATS_TOKEN=<daemon token>
COOP_NATS_PREFIX=hooks
```

---

## 7. Event Distribution: NATS JetStream

### Event Architecture

The event bus serves two functions: local handler dispatch and cross-component distribution.

```
RPC Handler (e.g., OpCreate)
    │
    ├─→ EventBus.Dispatch()
    │       │
    │       ├─→ Local handlers (Slack notifier, stop-loop detector, etc.)
    │       │
    │       └─→ JetStream publish
    │               │
    │               ├─→ Controller (beadswatcher)
    │               ├─→ Agent Coop instances
    │               └─→ Slack bot
    │
    └─→ MutationEvent (SSE stream for controller)
```

### Event Types

**Bead mutation events** (trigger controller actions):
```go
EventMutationCreate  // New bead → possible pod creation
EventMutationUpdate  // Status change → possible pod action
EventMutationDelete  // Bead deleted → pod deletion
EventMutationStatus  // Status transition
```

**Agent lifecycle events** (operational telemetry):
```go
EventOjAgentSpawned       // Pod registered with daemon
EventOjAgentIdle          // Agent waiting for work
EventOjAgentEscalated     // Agent stuck, needs help
EventOjJobCompleted       // Work finished
EventOjJobFailed          // Work failed
EventAgentStarted         // Agent process started
EventAgentStopped         // Agent process stopped
EventAgentCrashed         // Agent process crashed
EventAgentHeartbeat       // Periodic heartbeat
EventStopLoopDetected     // Agent in stop-restart loop
```

**Decision events** (human-in-the-loop):
```go
EventDecisionCreated      // Agent needs human input
EventDecisionResponded    // Human responded
EventDecisionEscalated    // Decision timed out
EventDecisionExpired      // Decision auto-expired
```

### JetStream Streams

| Stream | Subjects | Retention | Purpose |
|--------|----------|-----------|---------|
| `HOOK_EVENTS` | `hooks.>` | 256 MiB mem, 1 GiB disk | All lifecycle events |
| `MUTATION_EVENTS` | `mutations.>` | Limits-based | Bead CRUD for controller |
| (scoped) | `decisions.<agent>.*` | Per-agent | Decision events (prevents cross-agent pollution) |

---

## 8. Session Registry: Cross-Backend Discovery

**Location**: `gastown/internal/registry/registry.go`

The session registry provides unified agent discovery regardless of backend:

```go
type Session struct {
    ID           string  // Bead ID (e.g., "gt-gastown-crew-k8s")
    Rig          string  // Rig name
    Role         string  // mayor, deacon, crew, polecat, witness, refinery
    Name         string  // Agent name within role
    BackendType  string  // "coop", "ssh", or "tmux"
    CoopURL      string  // Coop HTTP endpoint
    Alive        bool    // Health check result
    AgentState   string  // spawning, working, done, stuck
    HookBead     string  // Currently assigned work
    Target       string  // "local" or "k8s"
}
```

**Discovery flow:**

```
DiscoverAll(ctx, opts)
    │
    ├─→ Query daemon for agent beads (gt:agent label)
    │
    ├─→ For each bead:
    │       ├─ Read labels: execution_target, rig, role, agent
    │       ├─ Read notes: backend, coop_url, pod_name
    │       └─ Build Session struct
    │
    ├─→ (Optional) Health check each session
    │       └─ HTTP GET coop_url/api/v1/health (parallel, 10 concurrent)
    │
    └─→ Return []Session
```

This powers commands like `gt status`, `gt peek <agent>`, `gt nudge <agent>`.

---

## 9. Helm Deployment Topology

**Location**: `gastown/helm/gastown/`

### Chart Structure

The gastown Helm chart (v0.5.3) is a unified chart with subcharts:

```
gastown/
├── Chart.yaml          (main chart)
├── values.yaml         (defaults)
├── charts/
│   └── bd-daemon/      (subchart: Dolt + daemon + NATS + Redis)
└── templates/
    ├── agent-controller/
    │   ├── deployment.yaml
    │   ├── rbac.yaml
    │   └── serviceaccount.yaml
    ├── coop-broker/
    │   ├── deployment.yaml
    │   └── service.yaml
    └── git-mirror/
        └── (per-rig Deployment + Service + PVC)
```

### Key Values

**BD Daemon** (`bd-daemon.*`):
```yaml
bd-daemon:
  image:
    repository: ghcr.io/groblegark/beads
    tag: "0.57.17"
  daemon:
    tcp:
      port: 9876
    http:
      port: 9080
    resources:
      requests: { cpu: 250m, memory: 512Mi }
      limits:   { cpu: "1", memory: 2Gi }
  dolt:
    image:
      repository: dolthub/dolt-sql-server
      tag: "1.80.2"
    persistence:
      size: 10Gi
      storageClass: gp3
  nats:
    enabled: true
    jetstream:
      maxMemory: 256M
    persistence:
      size: 2Gi
```

**Agent Controller** (`agentController.*`):
```yaml
agentController:
  enabled: false  # opt-in
  image:
    repository: ghcr.io/groblegark/gastown/agent-controller
    tag: latest
  agentImage:
    repository: ghcr.io/groblegark/gastown/gastown-agent
    tag: latest
  apiKeySecret: ""           # K8s secret with ANTHROPIC_API_KEY
  coopImage: ""              # Coop sidecar image
  coopBuiltin: false         # Or use builtin coop
  sidecarProfiles:
    toolchain-full:
      image: "ghcr.io/groblegark/gastown-toolchain:latest"
      cpuLimit: "2"
      memoryLimit: "4Gi"
```

**Coop Broker** (`coopBroker.*`, optional):
```yaml
coopBroker:
  enabled: false
  credentials:
    accountName: "claude-max"
    provider: "claude"
    refreshMarginSecs: 300
  mux:
    enabled: false  # terminal multiplexer
```

### RBAC

The agent controller's ServiceAccount needs:
```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["create", "delete", "get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["watch"]
```

### ExternalSecrets

Secrets are pulled from AWS Secrets Manager (or similar) via ExternalSecrets:

| Secret | Purpose |
|--------|---------|
| Dolt root password | Database auth |
| BD daemon token | RPC + NATS auth (shared by all components) |
| Git credentials | Agent repo access (username + token) |
| Slack credentials | Bot + app tokens |
| Claude credentials | OAuth refresh tokens (via coop broker) |
| NATS token | Optional separate NATS auth |

---

## 10. Data Flow: End-to-End Lifecycle

### Creating an Agent

```
1. Human/Mayor runs:
   bd create --title="crew-k8s" --type=task \
     --labels="gt:agent,execution_target:k8s,rig:gastown,role:crew,agent:k8s"

2. bd CLI → RPC OpCreate → Daemon:
   - Stores issue in Dolt (status: open, labels: [gt:agent, execution_target:k8s, ...])
   - Emits MutationCreate event to EventBus
   - EventBus publishes to JetStream: mutations.create

3. Controller receives event (beadswatcher):
   - Parses mutation: type=create, labels contain gt:agent + execution_target:k8s
   - Maps to AgentSpawn lifecycle event
   - Calls ListRigBeads() for rig config (git mirror, image override, etc.)
   - Calls podmanager.CreateAgentPod(spec)

4. K8s creates pod:
   - Scheduler places pod on node
   - Init containers run (git clone from mirror if configured)
   - Agent container starts: entrypoint.sh runs
   - Coop sidecar starts (if configured)

5. Entrypoint registers with daemon:
   - bd agent pod-register <id> --pod-name=gt-gastown-crew-k8s --pod-ip=10.0.1.5
   - → RPC OpAgentPodRegister → Daemon updates issue fields
   - Daemon emits OjAgentSpawned event

6. Controller status reporter syncs back:
   - Reads pod status from K8s API (Running)
   - Updates bead: agent_state=working
   - Writes notes: coop_url=http://10.0.1.5:8080, pod_name=gt-gastown-crew-k8s

7. Agent is now working:
   - Claude CLI running inside pod
   - Coop health probes keep pod alive
   - Agent uses bd commands (all routed to daemon via RPC)
```

### Monitoring an Agent

```
Human runs: gt peek crew-k8s

1. SessionRegistry.Lookup("gt-gastown-crew-k8s")
   - Queries daemon for bead
   - Reads notes: coop_url=http://10.0.1.5:8080

2. CoopBackend.CapturePane("gt-gastown-crew-k8s", 50)
   - GET http://10.0.1.5:8080/api/v1/screen/text
   - Returns terminal contents

3. Display to human
```

### Nudging an Agent

```
Human runs: gt nudge crew-k8s "Check the failing test"

1. Resolve backend (same as peek)

2. CoopBackend.NudgeSession("gt-gastown-crew-k8s", "Check the failing test")
   - POST http://10.0.1.5:8080/api/v1/agent/nudge
   - Coop injects message into Claude session
```

### Closing an Agent

```
1. Human/Mayor runs: bd close gt-gastown-crew-k8s

2. bd CLI → RPC OpClose → Daemon:
   - Updates status to "closed" in Dolt
   - Emits MutationStatus event (status: in_progress → closed)

3. Controller receives event:
   - Maps to AgentDone lifecycle event
   - Calls podmanager.DeleteAgentPod("gt-gastown-crew-k8s")

4. K8s deletes pod:
   - Pod terminates (graceful shutdown)
   - PVC retained (for crew roles)
```

---

## 11. Security Model

### Authentication

All inter-component auth uses a single shared token (`BD_DAEMON_TOKEN`):

```
Agent Pod ──[Bearer token]──→ Daemon RPC (TCP :9876)
Agent Pod ──[Bearer token]──→ Daemon HTTP (:9080)
Controller ──[Bearer token]──→ Daemon HTTP (:9080)
Coop ──[NATS token]──→ NATS Server (:4222)
```

The token is stored as a K8s Secret (sourced from ExternalSecrets) and injected into
pod environments.

### Pod Security

- All containers run as non-root (UID 1000 for agents, UID 65534 for controller)
- `runAsNonRoot: true` enforced
- `fsGroup: 1000` for shared volume access
- Controller has minimal RBAC (pods + PVCs only)
- Agent pods have no K8s API access

### Network Boundaries

- Daemon TCP/HTTP: ClusterIP only (no external access by default)
- Coop ports: Pod-to-pod only (no Service, accessed via pod IP)
- NATS: ClusterIP only
- Optional: Traefik IngressRoute for daemon HTTP (with IP whitelist + rate limiting)

---

## 12. Operational Reference

### Service DNS

| Component | DNS Name | Port |
|-----------|----------|------|
| Daemon (RPC) | `bd-daemon.{ns}.svc.cluster.local` | 9876 |
| Daemon (HTTP) | `bd-daemon.{ns}.svc.cluster.local` | 9080 |
| Dolt | `dolt.{ns}.svc.cluster.local` | 3306 |
| NATS | `{release}-bd-daemon-nats.{ns}.svc.cluster.local` | 4222 |
| Git Mirror | `git-mirror-{rig}.{ns}.svc.cluster.local` | 9418 |
| Coop Broker | `coop-broker.{ns}.svc.cluster.local` | 8080 |

### Agent Roles and Their Lifecycle

| Role | Persistence | Managed By | PVC | Restart Behavior |
|------|-------------|------------|-----|------------------|
| **Mayor** | Singleton, persistent | Human | Yes | Resume session |
| **Deacon** | Singleton, persistent | Human | Yes | Resume session |
| **Witness** | One per rig, persistent | Human | Yes | Resume session |
| **Refinery** | One per rig, persistent | Human | Yes | Resume session |
| **Crew** | Long-lived | Human | Yes | Resume session |
| **Polecat** | Ephemeral | Witness | No (EmptyDir) | Fresh start |
| **Dog** | Ephemeral | Deacon | No (EmptyDir) | Fresh start |

### Key Environment Variables

| Variable | Set By | Used By | Purpose |
|----------|--------|---------|---------|
| `BD_DAEMON_HOST` | Controller/Helm | Agent pods | Daemon TCP address |
| `BD_DAEMON_PORT` | Controller/Helm | Agent pods | Daemon TCP port |
| `BD_DAEMON_HTTP_URL` | Entrypoint | bd CLI | Daemon HTTP URL |
| `BD_DAEMON_TOKEN` | K8s Secret | All | Auth token |
| `GT_ROLE` | Controller | Entrypoint | Agent role type |
| `GT_AGENT` | Controller | Entrypoint | Agent name |
| `GT_RIG` | Controller | Entrypoint | Rig name |
| `GT_TOWN_NAME` | Helm | Entrypoint | Town identifier |
| `ANTHROPIC_API_KEY` | K8s Secret | Claude CLI | API access |
| `COOP_NATS_URL` | Controller | Coop | NATS endpoint |
| `COOP_NATS_TOKEN` | K8s Secret | Coop | NATS auth |

### Troubleshooting

**Pod not creating:**
1. Check bead has both labels: `gt:agent` AND `execution_target:k8s`
2. Check controller logs: `kubectl logs deploy/agent-controller`
3. Check reconciler: `kubectl logs deploy/agent-controller | grep reconcile`
4. Verify daemon reachable: `kubectl exec deploy/agent-controller -- wget -qO- http://bd-daemon:9080/health`

**Pod stuck in startup:**
1. Startup probe allows 150s -- wait for Claude CLI boot
2. Check coop health: `kubectl exec <pod> -c coop -- wget -qO- http://localhost:9090/api/v1/health`
3. Check entrypoint: `kubectl logs <pod> -c agent`

**Agent not responding:**
1. Check agent state: `bd show <id>` (look at agent_state, last_activity)
2. Check coop: `curl http://<pod-ip>:8080/api/v1/agent/state`
3. Check pod status: `kubectl describe pod <pod-name>`
4. Check NATS connectivity: `kubectl exec <pod> -- env | grep NATS`

**Orphan pods:**
- Reconciler catches these every 60s
- Manual: `kubectl delete pod <name>` (controller will not recreate if bead is closed)
