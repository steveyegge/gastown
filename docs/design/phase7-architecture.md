# Phase 7: Beads-Driven Kubernetes Controller — Architecture Design

> **Phase 7 — Kubernetes Controller and CRD**
> Task: gt-naa65p.12 — Architecture design document
> Date: 2026-02-07

---

## 1. Executive Summary

### What problem does the controller solve?

Gas Town is a multi-agent system where AI agents (polecats, crew, witnesses,
refineries) perform software engineering work. Currently, these agents run as
local tmux sessions on a single machine. Phase 7 moves agent execution into
Kubernetes, enabling elastic scaling, fault isolation, and cloud-native
operations — without replacing the existing control plane.

### Why beads-as-control-plane instead of CRD-driven?

Beads (backed by Dolt) already implements the full agent lifecycle:

- **Identity**: Agent beads track spawning, running, and completion states
- **Work routing**: Hooks, molecules, and the sling system assign work
- **Health monitoring**: Witnesses patrol polecats, Deacons check system health
- **Communication**: Mail protocol enables async coordination
- **Merge processing**: Refineries manage merge queues via beads state
- **Dependencies**: `bd dep` and `bd blocked` track work ordering

Reimplementing any of this in a Kubernetes operator would create a second
source of truth and duplicate ~10,000 lines of existing logic. Instead, the
controller is a **thin reactive bridge**: it watches beads events and translates
them into pod create/delete operations. The brain stays in beads; the
controller is just the hands.

### Controller is reactive: observes beads, creates/deletes pods

```
  Agent calls gt sling         Polecat runs gt done
       │                              │
       ▼                              ▼
  Beads records                 Beads records
  "hook" event                  "done" event
       │                              │
       ▼                              ▼
  Controller observes           Controller observes
       │                              │
       ▼                              ▼
  Creates K8s Pod               Deletes K8s Pod
```

The controller makes **zero** lifecycle decisions. It does not decide when to
spawn polecats, how many to run, or when to kill them. Those decisions are
made by existing agents (witnesses, the sling system, `gt done`) operating
through beads.

---

## 2. Architecture Overview

### 2.1 System Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Beads (Dolt)                                   │
│   Single source of truth for ALL state:                             │
│   - Agent lifecycle (spawn, hook, done, kill, stuck)                │
│   - Work routing (sling, hook, molecules, convoys)                  │
│   - Configuration (town, rig, roles, agents)                        │
│   - Merge queue, mail, escalation, dependencies                     │
│                                                                     │
│   Events flow OUT via bd-daemon activity stream:                    │
│     witness decides "spawn polecat furiosa"                         │
│       → beads records event → controller observes → creates Pod     │
│     polecat runs "gt done"                                          │
│       → beads records completion → controller observes → deletes Pod│
└───────────────────────────────┬─────────────────────────────────────┘
                                │ bd activity --follow --json (NDJSON)
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                Gas Town Controller (thin reactive bridge)             │
│                                                                     │
│   ONLY does:                                                        │
│   1. Watch beads activity stream for lifecycle events                │
│   2. Create/delete K8s Pods based on those events                   │
│   3. Report pod status back to beads (future: gt-naa65p.7)          │
│                                                                     │
│   Does NOT:                                                         │
│   - Make lifecycle decisions (beads agents do this)                 │
│   - Manage infrastructure (Helm does this)                          │
│   - Store config (beads owns its own state)                         │
│   - Run controller-runtime reconciliation loops                     │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ client-go: Pod create/delete
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Kubernetes Cluster                              │
│                                                                     │
│   Managed by Helm (infrastructure):          Managed by Controller: │
│   ┌──────────────┐ ┌──────────────┐          ┌──────────────┐      │
│   │ Dolt         │ │ BD Daemon    │          │ Agent Pods   │      │
│   │ StatefulSet  │ │ Deployment   │          │ (bare pods)  │      │
│   └──────────────┘ └──────────────┘          │              │      │
│   ┌──────────────┐ ┌──────────────┐          │ ● polecats   │      │
│   │ RPC Server   │ │ Controller   │          │ ● crew       │      │
│   │ (gtmobile)   │ │ Deployment   │          │ ● witness    │      │
│   └──────────────┘ └──────────────┘          │ ● refinery   │      │
│                                              └──────────────┘      │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 What Lives Where

| Concern | Owner | How |
|---------|-------|-----|
| Dolt, BD Daemon, RPC, Controller | **Helm** | Standard charts |
| Agent lifecycle decisions | **Beads** | Witness spawns, gt sling, gt done |
| Work routing, hooks, molecules | **Beads** | bd commands, daemon RPC |
| Merge queue processing | **Beads** | Refinery agent via beads state |
| Config (town, rig, roles, agents) | **Beads** | JSON config files in Dolt |
| Agent Pod creation/deletion | **Controller** | Reacts to beads activity events |
| Pod templates (image, resources) | **CRD spec** | Controller reads when creating pods |
| CRD status projection | **Controller** | Reads beads state, writes K8s status |

### 2.3 Helm Manages Infrastructure, Controller Manages Agent Pods

This separation is critical. Infrastructure components (Dolt, BD Daemon, RPC
server, the controller itself) are standard stateless/stateful services with
well-understood Helm patterns. The controller only manages the **dynamic**
part: agent pods that come and go based on beads events.

---

## 3. Event Flow

### 3.1 Beads Event Types

The controller watches the beads activity stream via `bd activity --follow --json`,
which emits NDJSON events. The beads watcher maps these to four lifecycle event types:

| Controller Event | Beads Activity Types | Meaning |
|-----------------|---------------------|---------|
| `AgentSpawn` | `sling`, `hook`, `spawn`, status→`in_progress` | New agent needs a pod |
| `AgentDone` | `done`, `unhook`, status→`closed` | Agent completed, delete pod |
| `AgentKill` | `kill`, `session_death` | Agent should be terminated |
| `AgentStuck` | `escalation_sent`, `polecat_nudged` | Agent unresponsive, restart pod |

### 3.2 Event → Pod Operation Mapping

```
Beads Event Stream (NDJSON via bd activity --follow --json)
        │
        ▼
    ActivityWatcher parses NDJSON
        │
        ├─ Extract: rig, role, agent_name, bead_id, metadata
        │  Sources (priority order):
        │    1. Actor field: "gastown/polecats/furiosa"
        │    2. Payload fields: rig, role, agent/agent_name
        │    3. Target field: fallback parsing
        │
        ├─ Map to event type:
        │    sling/hook/spawn    → AgentSpawn
        │    done/unhook         → AgentDone
        │    kill/session_death  → AgentKill
        │    escalation/nudged   → AgentStuck
        │    status (closed)     → AgentDone
        │    status (in_progress)→ AgentSpawn
        │
        ▼
    Controller Main Loop (select on event channel)
        │
        ├─ AgentSpawn:
        │    1. Resolve pod template (merge defaults hierarchy)
        │    2. Build AgentPodSpec with env vars, secrets, volumes
        │    3. Call pods.CreateAgentPod()
        │
        ├─ AgentDone / AgentKill:
        │    1. Derive pod name: gt-{rig}-{role}-{agent}
        │    2. Call pods.DeleteAgentPod()
        │
        └─ AgentStuck:
             1. Delete existing pod (may not exist)
             2. Rebuild pod spec from event
             3. Call pods.CreateAgentPod() (restart)
```

### 3.3 Polecat Lifecycle: End-to-End

```
Witness/Crew calls gt sling <bead> <rig>
    │
    ├── gt CLI validates bead, allocates polecat name from NamePool
    ├── gt CLI sets hook_bead in beads (assigns work)
    ├── Beads records "sling" activity event
    │
    ▼
Controller observes "sling" event
    │
    ├── Extracts: rig=gastown, role=polecat, name=furiosa, bead=gt-xyz
    ├── Merges pod template hierarchy
    ├── Creates K8s Pod: gt-gastown-polecat-furiosa
    │
    ▼
Pod starts
    │
    ├── Init: git clone/worktree setup
    ├── Main: claude-code → gt prime → reads hook from beads
    ├── Works on assigned bead
    ├── Runs gt done → beads records "done" event
    ├── Container exits (RestartPolicy: Never)
    │
    ▼
Controller observes "done" event
    │
    └── Deletes Pod: gt-gastown-polecat-furiosa
```

### 3.4 Status Reporting: Pod → Beads (Future: gt-naa65p.7)

Pod status flows **back** to beads so that existing monitoring agents
(Witness, Deacon) can incorporate K8s state:

| K8s Pod Phase | Beads agent_state | Action |
|---------------|-------------------|--------|
| Pending | `starting` | Pod is scheduling |
| Running | `working` | Agent is active |
| Succeeded | `done` | Normal completion |
| Failed | `failed` | Create escalation bead |
| Unknown | `unknown` | Investigate |

The status reporter is currently a stub (`StubReporter`). The real
implementation (gt-naa65p.7) will use BD Daemon RPC to update beads.

---

## 4. Component Design

### 4.1 Project Structure

```
controller/
├── cmd/controller/
│   └── main.go                    Entry point + event loop (215 lines)
├── internal/
│   ├── beadswatcher/
│   │   ├── watcher.go             Activity stream consumer (387 lines)
│   │   └── watcher_test.go        Tests (438 lines)
│   ├── podmanager/
│   │   ├── manager.go             K8s pod CRUD (373 lines)
│   │   ├── manager_test.go        Tests (641 lines)
│   │   ├── defaults.go            Template merge hierarchy (199 lines)
│   │   └── defaults_test.go       Tests
│   ├── statusreporter/
│   │   ├── reporter.go            Status sync stub (53 lines)
│   │   └── reporter_test.go       Tests
│   └── config/
│       ├── config.go              Env/flag config (82 lines)
│       └── config_test.go         Tests
├── Dockerfile                     Multi-stage: golang:1.24 → distroless
├── Makefile                       Build, test, lint, docker targets
├── go.mod                         Module: github.com/steveyegge/gastown/controller
└── go.sum
```

**Total**: ~1,026 lines of core code, ~1,128 lines of tests.

### 4.2 Beads Activity Watcher

**Package**: `controller/internal/beadswatcher`

The watcher consumes the beads activity stream and emits typed lifecycle events.

**Architecture**:
- Runs `bd activity --follow --town --json` as a subprocess
- Reads NDJSON events line by line via `bufio.Scanner`
- Reconnects with exponential backoff (1s → 30s max) on stream errors
- Emits events on a buffered channel (capacity 64)

**Interfaces**:
```go
type Watcher interface {
    Start(ctx context.Context) error    // Blocks until ctx canceled
    Events() <-chan Event               // Read-only event channel
}

type Event struct {
    Type      EventType                 // AgentSpawn/Done/Kill/Stuck
    Rig       string                    // e.g., "gastown"
    Role      string                    // polecat, crew, witness, refinery
    AgentName string                    // e.g., "furiosa"
    BeadID    string                    // Triggering bead ID
    Metadata  map[string]string         // namespace, image, daemon_host, etc.
}
```

**Agent Info Extraction** (priority order):
1. **Actor** field: `"gastown/polecats/rictus"` → split on `/`
2. **Payload** fields: `rig`, `role`, `agent`/`agent_name`
3. **Target** field: fallback parsing in `"rig/role/name"` format

**Role normalization**: Plural forms (`polecats` → `polecat`, `crews` → `crew`)
are normalized to singular for consistency with pod manager.

**Testing**: `StubWatcher` provides a no-op implementation for tests.

### 4.3 Pod Manager

**Package**: `controller/internal/podmanager`

Pure K8s pod CRUD. Executes lifecycle decisions made by beads — never makes
decisions itself.

**Interface**:
```go
type Manager interface {
    CreateAgentPod(ctx context.Context, spec AgentPodSpec) error
    DeleteAgentPod(ctx context.Context, name, namespace string) error
    ListAgentPods(ctx context.Context, namespace string, labels map[string]string) ([]corev1.Pod, error)
    GetAgentPod(ctx context.Context, name, namespace string) (*corev1.Pod, error)
}
```

**Pod Naming Convention**: `gt-{rig}-{role}-{agentName}`
- Example: `gt-gastown-polecat-furiosa`, `gt-gastown-crew-colonization`

**Pod Labels** (for discovery and filtering):
```yaml
app.kubernetes.io/name: gastown
gastown.io/rig: <rig>
gastown.io/role: <role>
gastown.io/agent: <agentName>
```

**Pod Construction**:

| Aspect | Configuration |
|--------|---------------|
| Security | RunAsUser: 1000, RunAsGroup: 1000, RunAsNonRoot: true, Drop ALL capabilities |
| Image pull | Always (ensures latest agent image) |
| Termination grace | 30 seconds |
| Restart policy | Never (polecats), Always (all others) |

**Environment Variables** (injected into every agent pod):

| Variable | Source | Purpose |
|----------|--------|---------|
| `GT_ROLE` | Event | Agent role (polecat, crew, etc.) |
| `GT_RIG` | Event | Rig name (gastown, beads, etc.) |
| `GT_AGENT` | Event | Agent name (furiosa, nux, etc.) |
| `GT_POLECAT` | Event (if polecat) | Polecat-specific identity |
| `GT_CREW` | Event (if crew) | Crew-specific identity |
| `HOME` | Static | `/home/agent` |
| `BD_DAEMON_HOST` | Config | BD Daemon hostname |
| `BD_DAEMON_PORT` | Config | BD Daemon port |
| `BEADS_AUTO_START_DAEMON` | Static | `false` (use existing daemon) |
| `ANTHROPIC_API_KEY` | K8s Secret | API key for Claude |

**Volume Strategy**:

| Role | Workspace Volume | Rationale |
|------|-----------------|-----------|
| Polecat | EmptyDir | Ephemeral, one-shot execution |
| Crew | PVC (10Gi, gp3) | Persistent workspace for long-lived workers |
| Witness | PVC (5Gi, gp3) | Persistent state for monitoring |
| Refinery | PVC (5Gi, gp3) | Persistent state for merge processing |

All pods mount:
- `/home/agent/gt` — workspace (EmptyDir or PVC)
- `/tmp` — EmptyDir
- `/etc/agent-pod` — ConfigMap (optional, for agent configuration)

### 4.4 Pod Template Merge Hierarchy

**Package**: `controller/internal/podmanager` (`defaults.go`)

Pod templates follow a layered cascade. More specific layers override less
specific ones. Only explicitly set fields in an override are merged.

```
Layer 1: GasTown.spec.defaults             (town-wide base)
       ↓ overridden by
Layer 2: Rig.spec.podOverrides              (rig-level)
       ↓ overridden by
Layer 3: Rig.spec.roleOverrides[role]       (role-level)
       ↓ overridden by
Layer 4: AgentPool.spec.template            (pool-level, optional)
       ↓ overridden by
Layer 5: Event metadata                     (per-event, from beads)
```

**Merge rules**:
- Scalar values: override replaces base if non-zero
- Maps (env, nodeSelector): merged, override keys take precedence
- Slices (tolerations, secretEnv): override replaces entirely
- Resources: merged at the request/limit level

**Example resolution for a polecat in gastown rig**:

| Field | GasTown | Rig | Role | Resolved |
|-------|---------|-----|------|----------|
| image | agent:latest | — | — | agent:latest |
| cpu request | 500m | 1 | 500m | 500m |
| memory limit | 4Gi | 8Gi | 4Gi | 4Gi |
| nodeSelector | — | — | {node-type: burst} | {node-type: burst} |
| storage | PVC 10Gi | PVC 20Gi | — (EmptyDir) | EmptyDir |

### 4.5 Status Reporter

**Package**: `controller/internal/statusreporter`

**Current state**: Stub implementation (gt-naa65p.7 will implement).

**Interface**:
```go
type Reporter interface {
    ReportPodStatus(ctx context.Context, agentName string, status PodStatus) error
    SyncAll(ctx context.Context) error
}
```

**Planned design** (gt-naa65p.7):
- Use BD Daemon RPC to update agent beads with pod status
- Wire into main loop: call `ReportPodStatus()` after each pod operation
- Add periodic `SyncAll()` goroutine for reconciliation (30s interval)
- Expose Prometheus metrics:
  - `gastown_agents_total{rig, role, state}`
  - `gastown_polecats_active{rig}`
  - `gastown_pod_restarts_total{rig, agent}`
  - `gastown_beads_events_processed_total`

### 4.6 Configuration

**Package**: `controller/internal/config`

| Config Key | Env Var | Flag | Default |
|-----------|---------|------|---------|
| DaemonHost | `BD_DAEMON_HOST` | `--daemon-host` | `localhost` |
| DaemonPort | `BD_DAEMON_PORT` | `--daemon-port` | `9876` |
| Namespace | `NAMESPACE` | `--namespace` | `gastown` |
| KubeConfig | `KUBECONFIG` | `--kubeconfig` | (in-cluster) |
| LogLevel | `LOG_LEVEL` | `--log-level` | `info` |
| TownRoot | `GT_TOWN_ROOT` | `--town-root` | (empty) |
| BdBinary | `BD_BINARY` | `--bd-binary` | `bd` |
| DefaultImage | `AGENT_IMAGE` | `--agent-image` | (empty) |

Priority: flags > env vars > defaults.

### 4.7 CRD Schema (API Group: `gastown.io/v1alpha1`)

**Package**: `api/v1alpha1`

Three CRD resources provide K8s-native configuration and status projection:

| CRD | Scope | Purpose |
|-----|-------|---------|
| **GasTown** | Namespace | Town-level pod defaults + daemon connection + projected status |
| **Rig** | Namespace | Per-rig pod template overrides + projected rig status |
| **AgentPool** | Namespace | Role-specific pod templates + projected agent status |

**Key design rule**: CRD spec contains ONLY K8s-specific concerns (image,
resources, scheduling, storage). All beads configuration (merge strategy,
naming pools, workflows, role definitions) stays in beads.

**CRD status is projection, not truth**: The controller reads beads state
via BD Daemon and writes it to CRD status. It never computes agent state.
Status may lag beads by up to one polling interval (30s default).

**Resource ownership** with cascading deletion:
```
Helm (creates):
└── GasTown CR
    └── Rig CRs (ownerRef → GasTown)
        └── AgentPool CRs (ownerRef → Rig)
            └── Agent Pods (ownerRef → AgentPool)
```
Deleting a GasTown CR cascades: Rig CRs → AgentPool CRs → Pods.
Infrastructure (Dolt, BD Daemon) is NOT affected — Helm owns those.

**GasTown CRD example**:
```yaml
apiVersion: gastown.io/v1alpha1
kind: GasTown
metadata:
  name: production
  namespace: gastown
spec:
  name: gt11
  daemon:
    host: gastown-daemon.gastown.svc
    port: 9876
    tokenSecretRef:
      name: daemon-token
      key: token
  defaults:
    image: 909418727440.dkr.ecr.us-east-1.amazonaws.com/gastown-agent:latest
    resources:
      requests: { cpu: 500m, memory: 1Gi }
      limits:   { cpu: "2", memory: 4Gi }
    storage: { size: 10Gi, storageClassName: gp3 }
    env:
      - name: ANTHROPIC_API_KEY
        valueFrom:
          secretKeyRef: { name: anthropic-api-key, key: api-key }
status:
  phase: Running
  rigCount: 2
  agentCount: 12
```

**Rig CRD example**:
```yaml
apiVersion: gastown.io/v1alpha1
kind: Rig
metadata:
  name: gastown
  namespace: gastown
  labels: { gastown.io/town: production }
spec:
  name: gastown
  roleOverrides:
    polecat:
      nodeSelector: { node-type: agent-burst }
      resources:
        requests: { cpu: 500m, memory: 1Gi }
        limits:   { cpu: "2", memory: 4Gi }
    crew:
      resources:
        requests: { cpu: "1", memory: 2Gi }
        limits:   { cpu: "4", memory: 8Gi }
      storage: { size: 50Gi }
status:
  phase: Running
  polecatCount: 3
  crewCount: 1
  witness: { ready: true, podName: gt-gastown-witness-patrol }
  mergeQueue: { depth: 2, processing: 1 }
```

---

## 5. Comparison with Traditional Kubernetes Operator

### 5.1 What We DON'T Do

| Traditional Operator | Gas Town Controller |
|---------------------|---------------------|
| CRD reconciliation loops (controller-runtime) | Event-driven reaction to beads activity |
| Desired state in etcd via CRDs | Desired state in beads (Dolt) |
| Finalizers for cleanup | Direct pod deletion on beads events |
| Informer watches on K8s resources | Watch on beads NDJSON activity stream |
| Status computed from K8s state | Status projected from beads state |
| Operator makes scaling decisions | Beads agents make lifecycle decisions |
| Controller owns the lifecycle | Controller is just the hands |

### 5.2 What We DO

| Capability | Implementation |
|-----------|---------------|
| Watch beads events | `bd activity --follow --json` via subprocess |
| Create agent pods | client-go `CoreV1().Pods().Create()` |
| Delete agent pods | client-go `CoreV1().Pods().Delete()` |
| Template hierarchy | 4-layer merge: GasTown → Rig → Role → AgentPool |
| Restart stuck agents | Delete + recreate on `AgentStuck` events |
| Reconnect on failure | Exponential backoff (1s → 30s) |
| Graceful shutdown | Signal handling (SIGTERM, SIGINT) |

### 5.3 Why This Is Better

**Single source of truth**: Beads is the only place where agent state lives.
There is no split-brain between CRD status and actual state. The controller
never disagrees with beads because it never computes state — it only projects
what beads tells it.

**Simpler code**: The controller is ~1,000 lines of Go. A full operator with
controller-runtime, informers, reconciliation loops, finalizers, and status
computation would be 5-10x larger. Less code means fewer bugs and easier
maintenance.

**Existing logic preserved**: All the sophisticated lifecycle management —
witness patrols, molecule workflows, merge queue processing, mail routing,
escalation chains — continues to work unchanged. The controller just provides
the K8s pod layer beneath it.

**No migration required**: The transition from tmux to K8s is transparent to
agents. They detect the runtime via `GT_RUNTIME` env var and use BD Daemon
RPC instead of local beads operations. Most `gt` commands need zero changes.

---

## 6. Agent Pod Types

| Agent | K8s Kind | Restart Policy | Storage | Replicas |
|-------|----------|---------------|---------|----------|
| Mayor | Pod (bare) | Always | EmptyDir | 1 per town |
| Deacon | Pod (bare) | Always | EmptyDir | 1 per town |
| Witness | Pod (bare) | Always | PVC (5Gi) | 1 per rig |
| Refinery | Pod (bare) | Always | PVC (5Gi) | 1 per rig |
| Crew | Pod (bare) | Always | PVC (10Gi) | N (declared) |
| Polecat | Pod (bare) | Never | EmptyDir | On-demand |

**Why bare Pods, not Deployments?** Agent pods are singletons (or demand-driven
for polecats) with precise lifecycle control from beads. Deployments add
rollout complexity that conflicts with beads-driven lifecycle. The controller
directly creates/deletes pods and handles restarts via beads events.

---

## 7. tmux → K8s Mapping

| tmux Concept | K8s Equivalent |
|-------------|----------------|
| tmux session | Pod |
| `remain-on-exit` | RestartPolicy: Never (polecats), Always (persistent) |
| `send-keys` / nudge | BD Daemon RPC: `NudgeAgent()` |
| `capture-pane` / peek | `kubectl logs` or BD Daemon RPC: `GetAgentLogs()` |
| `kill-session` | Delete Pod |
| Session name (`gt-gastown-nux`) | Pod name (`gt-gastown-polecat-nux`) |

**Detection**: `gt sling` checks `GT_RUNTIME=k8s` env var to choose between
tmux path and K8s dispatcher path. Beads operations are identical in both modes.

---

## 8. gt Command Behavior in K8s

| Command | Local (tmux) | K8s | Changes Needed |
|---------|-------------|-----|---------------|
| `gt sling` | Create worktree + tmux session | Create beads hook → controller creates Pod | Runtime detection |
| `gt done` | Close bead + tmux exit | Close bead + container exit(0) | None |
| `gt peek` | `tmux capture-pane` | `kubectl logs` or BD Daemon RPC | Phase 5 (SSH-tmux) |
| `gt nudge` | `tmux send-keys` | BD Daemon RPC: `NudgeAgent()` | RPC endpoint needed |
| `gt status` | Read beads + tmux state | Read beads + pod state | Enhancement only |
| `gt doctor` | Check local health | Check pod + beads health | Enhancement only |
| `bd *` | Local beads DB | BD Daemon RPC | Already works via daemon |

**Key insight**: Most commands need ZERO changes because beads mediates everything.
The controller is invisible to agents.

---

## 9. Deployment Architecture

### 9.1 Infrastructure (Helm-managed)

```yaml
# helm install gastown ./charts/gastown -f values.yaml
#
# Creates:
#   - Dolt StatefulSet + Service (persistent database)
#   - BD Daemon Deployment + Service (beads RPC)
#   - RPC Server Deployment + Service (gtmobile, optional)
#   - Controller Deployment (this controller)
#   - Secrets (API keys, git credentials, daemon tokens)
#   - ServiceAccounts, RBAC, PDBs
```

### 9.2 Controller Deployment

```yaml
# Controller needs RBAC to manage pods:
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "create", "delete", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
```

Controller is lightweight: 100m CPU, 128Mi memory baseline.

### 9.3 Container Image

```dockerfile
FROM golang:1.24 AS builder
# ... build with ldflags for version/commit ...
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /controller /controller
USER nonroot:nonroot
ENTRYPOINT ["/controller"]
```

### 9.4 Rig Configuration (User applies)

```yaml
# Per-rig configuration applied after Helm install
apiVersion: gastown.io/v1alpha1
kind: Rig
metadata:
  name: gastown
  namespace: gastown
  labels: { gastown.io/town: production }
spec:
  name: gastown
  roleOverrides:
    polecat:
      nodeSelector: { node-type: agent-burst }
    crew:
      storage: { size: 50Gi }
```

---

## 10. Upgrade Strategy (Future: gt-naa65p.8)

The controller does NOT decide when to upgrade. Beads signals the intent,
controller executes.

**Agent pod upgrades (beads-driven)**:
1. Overseer creates upgrade bead (new image version, config change)
2. Controller observes upgrade bead
3. Controller signals agents to handoff via beads
4. Controller waits for in-progress polecats to complete
5. Controller recreates pods with new spec
6. Controller reports upgrade status back to beads

**Pod update strategy by role**:
- **Persistent agents**: Rolling update, one at a time
- **Polecats**: Let running finish, new polecats get new spec
- **Witness**: Updated last (it monitors others)

**Infrastructure upgrades**: Helm upgrade for Dolt, BD Daemon, RPC, controller.

---

## 11. Dependencies and Phase Relationships

```
Phase 1 (Dolt in K8s)          Phase 2 (Cloud-Native Init)     Phase 3 (GitLab CI/CD)
  fhc-410                        bd-cma9                         fhc-8vy
      │                              │                              │
      ▼                              ▼                              ▼
Phase 4 (Remote Polecats)    Phase 5 (SSH-Tmux)            Phase 6 (RPC)
  gt-nm2lkj                    gt-3bwtas                     gt-f6ya1g
      │                            │                              │
      └────────────────────────────┼──────────────────────────────┘
                                   │
                                   ▼
                          Phase 7 (K8s Controller)
                            gt-naa65p ← YOU ARE HERE
```

**Direct dependencies**:
- Phase 5 (gt-3bwtas): SSH-Tmux for `gt peek` / `gt attach` in K8s pods
- Phase 6 (gt-f6ya1g): RPC improvements for remote BD Daemon operations

**Phase 7 internal task status**:

| Task | Status | Description |
|------|--------|-------------|
| gt-naa65p.1 | CLOSED | CRD schema design |
| gt-naa65p.2 | CLOSED | Controller reconciliation design |
| gt-naa65p.3 | CLOSED | Scaffold controller project |
| gt-naa65p.4 | CLOSED | Beads event watcher |
| gt-naa65p.5 | CLOSED | Pod manager |
| gt-naa65p.6 | OPEN | Agent pod lifecycle and identity |
| gt-naa65p.7 | OPEN | Bidirectional beads-K8s status sync |
| gt-naa65p.8 | OPEN | Graceful upgrade and rollout strategy |
| gt-naa65p.9 | OPEN | Integration tests |
| gt-naa65p.10 | HOOKED | Helm charts for controller/infrastructure |
| gt-naa65p.11 | HOOKED | Map gt commands to K8s-native equivalents |
| gt-naa65p.12 | IN_PROGRESS | This architecture document |

---

## 12. Existing Config vs CRD Boundary

| Concern | Where It Lives | CRD Involvement |
|---------|---------------|-----------------|
| Town identity | Beads (town.json) | GasTown.spec.name (label only) |
| Rig identity | Beads (rig config.json) | Rig.spec.name (correlation key) |
| Merge queue config | Beads (rig settings.json) | None |
| Namepool config | Beads (rig settings.json) | None |
| Workflow/formula config | Beads | None |
| Agent presets | Beads (RuntimeConfig) | None |
| Role definitions | Beads (roles/*.toml) | None |
| Messaging, escalation | Beads (config files) | None |
| Pod image | CRD | GasTown/Rig/AgentPool spec |
| Pod resources (CPU, memory) | CRD | GasTown/Rig/AgentPool spec |
| Pod storage (PVC) | CRD | GasTown/Rig/AgentPool spec |
| Pod scheduling | CRD | GasTown/Rig/AgentPool spec |
| Pod secrets (API keys) | CRD | env secretKeyRef |
| BD Daemon connection | CRD | GasTown.spec.daemon |
| Agent lifecycle state | Beads (Dolt) | Projected to CRD status |
| Agent hook/bead assignment | Beads (Dolt) | Projected to CRD status |
| Merge queue depth | Beads (Dolt) | Projected to CRD status |

**Rule of thumb**: If beads already tracks it, don't put it in the CRD.
If it's a K8s pod concern, put it in the CRD.

---

## 13. Key Design Decisions

### D1: Beads drives, controller reacts
The controller makes zero lifecycle decisions. Beads agents (witnesses,
sling system, gt done) make all decisions. The controller translates them
into pod operations. **Rationale**: Avoids duplicating ~10K lines of existing
lifecycle logic.

### D2: Infrastructure stays in Helm
Dolt, BD Daemon, RPC server, and the controller itself are Helm-managed.
**Rationale**: Standard infrastructure components with well-understood Helm
patterns. No benefit to CRD indirection.

### D3: CRD spec is K8s-only concerns
CRD spec fields are limited to container image, resources, scheduling, and
storage. **Rationale**: Prevents two sources of truth. Beads config is
managed by gt/bd commands and stored in Dolt.

### D4: Status is projection, not truth
CRD status is populated by querying beads. The controller never computes
agent state. **Rationale**: Beads tracks hooks, molecules, health, merge
queues, etc. Projecting is simpler and always consistent.

### D5: Bare Pods, not Deployments
Agent pods are managed directly (no Deployments, StatefulSets, or Jobs).
**Rationale**: Beads controls lifecycle timing precisely. Deployment rollout
semantics conflict with beads-driven restart/handoff.

### D6: Namespace-scoped CRDs
All CRDs are namespace-scoped. **Rationale**: Standard RBAC isolation;
supports multiple Gas Towns in one cluster.

### D7: No polecat controller logic
Polecats are spawned on-demand by agents calling `gt sling`. The controller
only observes the resulting beads event and creates the pod. **Rationale**:
Agents (Witness, Mayor, Crew) have context for dispatch decisions; the
controller does not.

---

## 14. Glossary

| Term | Definition |
|------|-----------|
| **Beads** | The issue/work tracking system backed by Dolt (a Git-versioned database). Source of truth for all state. |
| **BD Daemon** | Service that provides RPC access to the beads database. Agents communicate through it. |
| **Controller** | The thin reactive bridge between beads events and K8s pod operations (this component). |
| **CRD** | Custom Resource Definition — extends K8s API with GasTown-specific types. |
| **Crew** | Persistent AI worker agents with long-lived context and workspace. |
| **Deacon** | Daemon beacon — monitors system health and runs plugins. |
| **Gas Town** | The multi-agent system architecture for autonomous software engineering. |
| **Hook** | Assigning a bead (work item) to an agent for execution. |
| **Mayor** | Global coordinator that handles cross-rig communication and escalations. |
| **Molecule** | A workflow template with ordered steps that agents follow. |
| **Polecat** | Ephemeral one-shot AI worker agent. Born with work, does one task, then dies. |
| **Refinery** | Per-rig agent that processes the merge queue. |
| **Rig** | A project workspace with its own git repository, agents, and beads database. |
| **Sling** | Dispatching work to a polecat: `gt sling <bead> <rig>`. |
| **Witness** | Per-rig agent that monitors polecat health, handles nudging and cleanup. |

---

## References

- [CRD Schema Design](k8s-crd-schema.md) — Full CRD type definitions and examples
- [Reconciliation Loops Design](k8s-reconciliation-loops.md) — Controller loop pseudo-code
- [Gas Town Architecture](architecture.md) — Two-level beads architecture
- [Go Types](../../api/v1alpha1/gastown_types.go) — CRD Go struct definitions
- [Controller Source](../../controller/) — Implementation code
