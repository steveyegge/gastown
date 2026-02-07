# CRD Schema Design: GasTown and Rig Resources

> **Phase 7 — Kubernetes Controller and CRD**
> Task: gt-naa65p.1 — Design CRD schema for GasTown and Rig resources

## Architecture: Beads Drives, Controller Reacts

The Gas Town K8s controller is a **reactive bridge** between beads and
Kubernetes. It does NOT orchestrate agent lifecycle — beads does that.
The controller's only job is: **watch beads for agent lifecycle events →
translate to K8s pod operations**.

```
┌─────────────────────────────────────────────────────────┐
│                    Beads (Dolt)                          │
│  Single source of truth for ALL state:                  │
│  - Agent lifecycle (spawn, hook, complete, kill)        │
│  - Work routing (sling, hook, molecules)                │
│  - Configuration (town, rig, roles, agents)             │
│  - Merge queue, mail, escalation                        │
│                                                         │
│  Events flow OUT:                                       │
│    witness decides "spawn polecat furiosa"               │
│    → beads records event                                │
│    → controller observes                                │
│    → controller creates Pod                             │
│                                                         │
│    polecat runs "gt done"                               │
│    → beads records completion                           │
│    → controller observes                                │
│    → controller deletes Pod                             │
└───────────────┬─────────────────────────────────────────┘
                │ watches (bd daemon RPC / polling)
                ▼
┌─────────────────────────────────────────────────────────┐
│           Gas Town Controller (thin bridge)              │
│                                                         │
│  ONLY does:                                             │
│  1. Watch beads for agent lifecycle events               │
│  2. Create/delete K8s Pods based on those events        │
│  3. Project beads state → CRD status subresources       │
│                                                         │
│  Does NOT:                                              │
│  - Make lifecycle decisions (beads/agents do this)      │
│  - Manage infrastructure (Helm does this)               │
│  - Write beads config (beads owns its own state)        │
│  - Run reconciliation loops against desired state       │
└───────────────┬─────────────────────────────────────────┘
                │ creates/deletes
                ▼
┌─────────────────────────────────────────────────────────┐
│              Kubernetes Resources                        │
│                                                         │
│  Managed by Helm (NOT the controller):                  │
│  - Dolt StatefulSet + Service                           │
│  - BD Daemon Deployment + Service                       │
│  - Controller Deployment itself                         │
│  - Redis (optional)                                     │
│  - ExternalSecrets, PDBs, ServiceAccounts               │
│                                                         │
│  Managed by the controller (reactive):                  │
│  - Agent Pods (polecats, crew, witness, refinery)       │
│    Created when beads says "spawn"                      │
│    Deleted when beads says "done/killed"                │
└─────────────────────────────────────────────────────────┘
```

### What Lives Where

| Concern | Owner | How |
|---------|-------|-----|
| Dolt, BD Daemon, Redis, Controller | **Helm** | Standard Helm charts (existing) |
| Agent lifecycle decisions | **Beads** | Witness spawns, gt sling, gt done |
| Work routing, hooks, molecules | **Beads** | bd commands, daemon RPC |
| Merge queue processing | **Beads** | Refinery agent via beads state |
| Config (town, rig, roles, agents) | **Beads** | JSON config files on disk |
| Agent Pod creation/deletion | **Controller** | Reacts to beads events |
| CRD status projection | **Controller** | Reads beads state, writes K8s status |
| Pod template (resources, image, scheduling) | **CRD spec** | Controller reads when creating pods |

---

## CRD Purpose

CRDs serve two purposes in this architecture:

1. **Pod Templates**: Define HOW agent pods are created (image, resources,
   node selectors, storage) — the K8s-specific concerns that beads doesn't
   know about.

2. **Status Projection**: Give `kubectl` users a K8s-native view of the
   Gas Town state that actually lives in beads.

CRDs do NOT store beads configuration (merge strategy, naming pools,
workflow formulas, etc.) — that config lives in beads JSON files and is
managed by `gt config` commands.

### CRD Resources

| CRD | Scope | Purpose |
|-----|-------|---------|
| **GasTown** | Namespace | Town-level pod defaults + projected town status |
| **Rig** | Namespace | Per-rig pod template overrides + projected rig status |
| **AgentPool** | Namespace | Role-specific pod templates + projected pool status |

All CRDs use the `gastown.io/v1alpha1` API group.

---

## 1. GasTown CRD

The top-level resource. Defines town-wide defaults for agent pod creation
and projects overall town health from beads.

### 1.1 Example YAML

```yaml
apiVersion: gastown.io/v1alpha1
kind: GasTown
metadata:
  name: production
  namespace: gastown
spec:
  # Town identity (for labeling and status display)
  name: gt11

  # BD Daemon connection (controller uses this to watch beads events)
  daemon:
    host: gastown-daemon.gastown.svc
    port: 9876
    tokenSecretRef:
      name: daemon-token
      key: token

  # Default pod template for all agents in this town
  # Rigs and AgentPools can override these.
  defaults:
    image: 909418727440.dkr.ecr.us-east-1.amazonaws.com/gastown-agent:latest
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: "2"
        memory: 4Gi
    storage:
      size: 10Gi
      storageClassName: gp3
    screenScrollback: 10000
    env:
      - name: ANTHROPIC_API_KEY
        valueFrom:
          secretKeyRef:
            name: anthropic-api-key
            key: api-key
      - name: GIT_USERNAME
        valueFrom:
          secretKeyRef:
            name: git-credentials
            key: username
      - name: GIT_TOKEN
        valueFrom:
          secretKeyRef:
            name: git-credentials
            key: token

  # Rig selector — which Rig CRs belong to this town
  rigSelector:
    matchLabels:
      gastown.io/town: production

# Status is projected FROM beads, not computed by the controller.
status:
  phase: Running
  observedGeneration: 3
  conditions:
    - type: DaemonConnected
      status: "True"
      lastTransitionTime: "2026-02-06T00:00:00Z"
    - type: Healthy
      status: "True"
      lastTransitionTime: "2026-02-06T00:00:00Z"
  # Projected from beads: bd list --rigs | wc -l
  rigCount: 2
  # Projected from beads: count of active agents across all rigs
  agentCount: 12
  daemon:
    connected: true
    endpoint: gastown-daemon.gastown.svc:9876
```

### 1.2 Go Types

```go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Rigs",type=integer,JSONPath=`.status.rigCount`
// +kubebuilder:printcolumn:name="Agents",type=integer,JSONPath=`.status.agentCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GasTown is the top-level resource. It configures how the controller
// connects to beads and provides default pod templates for agent creation.
// Status is projected from beads state, not managed by the controller.
type GasTown struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   GasTownSpec   `json:"spec,omitempty"`
    Status GasTownStatus `json:"status,omitempty"`
}

// GasTownSpec defines the controller's configuration and pod defaults.
type GasTownSpec struct {
    // Name is the town identifier (e.g., "gt11"). Used for labeling.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    Name string `json:"name"`

    // Daemon configures the BD Daemon connection. The controller watches
    // beads lifecycle events through this daemon.
    Daemon DaemonConnectionSpec `json:"daemon"`

    // Defaults provides default pod template values for all agents.
    // Rigs and AgentPools can override individual fields.
    // +optional
    Defaults *PodDefaultsSpec `json:"defaults,omitempty"`

    // RigSelector selects which Rig resources belong to this town.
    // +optional
    RigSelector *metav1.LabelSelector `json:"rigSelector,omitempty"`
}

// DaemonConnectionSpec configures how the controller connects to BD Daemon.
type DaemonConnectionSpec struct {
    // Host is the BD Daemon service hostname.
    Host string `json:"host"`

    // Port is the BD Daemon RPC port.
    // +kubebuilder:default=9876
    Port int32 `json:"port"`

    // TokenSecretRef references an optional auth token for the daemon.
    // +optional
    TokenSecretRef *SecretKeyRef `json:"tokenSecretRef,omitempty"`
}

// PodDefaultsSpec defines default values for agent pod creation.
// These are K8s-specific concerns that beads doesn't manage.
type PodDefaultsSpec struct {
    // Image is the default agent container image.
    // +optional
    Image string `json:"image,omitempty"`

    // Resources defines default compute resources.
    // +optional
    Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

    // Storage configures default workspace PVC.
    // +optional
    Storage *StorageSpec `json:"storage,omitempty"`

    // ScreenScrollback sets the default screen session scrollback.
    // +kubebuilder:default=10000
    // +optional
    ScreenScrollback int32 `json:"screenScrollback,omitempty"`

    // Env provides default environment variables for all agent pods.
    // Typically includes secret references for API keys and git credentials.
    // +optional
    Env []corev1.EnvVar `json:"env,omitempty"`

    // NodeSelector constrains default scheduling.
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // Tolerations for agent pods.
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

    // ServiceAccountName for agent pods.
    // +optional
    ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// StorageSpec configures a PVC.
type StorageSpec struct {
    // Size is the requested storage capacity.
    // +kubebuilder:default="10Gi"
    Size resource.Quantity `json:"size"`

    // StorageClassName is the storage class.
    // +kubebuilder:default="gp3"
    // +optional
    StorageClassName string `json:"storageClassName,omitempty"`
}

// SecretKeyRef references a key within a Secret.
type SecretKeyRef struct {
    // Name of the Secret.
    Name string `json:"name"`

    // Key within the Secret data.
    // +optional
    Key string `json:"key,omitempty"`
}

// GasTownStatus is projected from beads state. The controller reads beads
// via the daemon and writes this status — it never computes state itself.
type GasTownStatus struct {
    // Phase is the high-level town state, projected from beads.
    // +kubebuilder:validation:Enum=Pending;Running;Degraded;Failed
    Phase string `json:"phase,omitempty"`

    // ObservedGeneration is the last spec generation the controller acted on.
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Conditions represent controller-level observations.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // RigCount is projected from beads (number of configured rigs).
    RigCount int32 `json:"rigCount,omitempty"`

    // AgentCount is projected from beads (total running agents).
    AgentCount int32 `json:"agentCount,omitempty"`

    // Daemon reflects the controller's connection to BD Daemon.
    // +optional
    Daemon *DaemonConnectionStatus `json:"daemon,omitempty"`
}

// DaemonConnectionStatus reports the controller's daemon connection.
type DaemonConnectionStatus struct {
    // Connected indicates the controller can reach the daemon.
    Connected bool `json:"connected"`

    // Endpoint is the daemon address.
    // +optional
    Endpoint string `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
type GasTownList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []GasTown `json:"items"`
}
```

---

## 2. Rig CRD

Each Rig provides per-rig pod template overrides and projects rig status
from beads. The controller uses Rig spec to know HOW to create pods when
beads says "spawn agent in this rig."

### 2.1 Example YAML

```yaml
apiVersion: gastown.io/v1alpha1
kind: Rig
metadata:
  name: gastown
  namespace: gastown
  labels:
    gastown.io/town: production
spec:
  # Rig identity (must match beads rig name)
  name: gastown

  # Pod template overrides for agents in this rig.
  # Merges with GasTown.spec.defaults — rig values take precedence.
  podOverrides:
    # Override resources for this rig's agents
    resources:
      requests:
        cpu: "1"
        memory: 2Gi
      limits:
        cpu: "4"
        memory: 8Gi
    # Override storage
    storage:
      size: 20Gi
    # Rig-specific env vars (merged with town defaults)
    env:
      - name: GT_RIG
        value: gastown

  # Role-specific overrides within this rig
  roleOverrides:
    polecat:
      nodeSelector:
        node-type: agent-burst
      resources:
        requests:
          cpu: 500m
          memory: 1Gi
        limits:
          cpu: "2"
          memory: 4Gi
    crew:
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "4"
          memory: 8Gi
      storage:
        size: 50Gi
    witness:
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
        limits:
          cpu: "1"
          memory: 2Gi
    refinery:
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
        limits:
          cpu: "1"
          memory: 2Gi

# Status projected from beads
status:
  phase: Running
  observedGeneration: 5
  conditions:
    - type: Reconciled
      status: "True"
      lastTransitionTime: "2026-02-06T00:00:00Z"
  # All counts projected from beads
  polecatCount: 3
  crewCount: 3
  witness:
    ready: true
    podName: witness-gastown-abc123
  refinery:
    ready: true
    podName: refinery-gastown-def456
  mergeQueue:
    depth: 2
    processing: 1
```

### 2.2 Go Types

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Polecats",type=integer,JSONPath=`.status.polecatCount`
// +kubebuilder:printcolumn:name="Crew",type=integer,JSONPath=`.status.crewCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Rig provides per-rig pod template overrides and projects rig status
// from beads. When beads emits a lifecycle event for an agent in this rig,
// the controller uses these overrides to build the pod spec.
type Rig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   RigSpec   `json:"spec,omitempty"`
    Status RigStatus `json:"status,omitempty"`
}

// RigSpec defines pod template overrides for a rig.
type RigSpec struct {
    // Name must match the beads rig name exactly.
    // The controller uses this to correlate beads events to this Rig CR.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_-]*$`
    Name string `json:"name"`

    // PodOverrides provides rig-level pod template overrides.
    // Merged with GasTown.spec.defaults — rig values take precedence.
    // +optional
    PodOverrides *PodDefaultsSpec `json:"podOverrides,omitempty"`

    // RoleOverrides provides role-specific pod template overrides.
    // Merged: GasTown defaults < Rig overrides < Role overrides.
    // +optional
    RoleOverrides map[string]PodDefaultsSpec `json:"roleOverrides,omitempty"`
}

// RigStatus is projected from beads state for this rig.
type RigStatus struct {
    // Phase is the high-level rig state, projected from beads.
    // +kubebuilder:validation:Enum=Pending;Running;Degraded;Failed
    Phase string `json:"phase,omitempty"`

    // ObservedGeneration is the last spec generation the controller acted on.
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Conditions represent controller-level observations.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // PolecatCount is the number of active polecats (from beads).
    PolecatCount int32 `json:"polecatCount,omitempty"`

    // CrewCount is the number of active crew members (from beads).
    CrewCount int32 `json:"crewCount,omitempty"`

    // Witness reflects witness agent status (from beads).
    // +optional
    Witness *SystemAgentStatus `json:"witness,omitempty"`

    // Refinery reflects refinery agent status (from beads).
    // +optional
    Refinery *SystemAgentStatus `json:"refinery,omitempty"`

    // MergeQueue reflects merge queue metrics (from beads).
    // +optional
    MergeQueue *MergeQueueStatus `json:"mergeQueue,omitempty"`
}

// SystemAgentStatus reports a system agent's state.
type SystemAgentStatus struct {
    // Ready indicates the agent is healthy (from beads health checks).
    Ready bool `json:"ready"`

    // PodName is the K8s pod running this agent.
    // +optional
    PodName string `json:"podName,omitempty"`
}

// MergeQueueStatus reports merge queue metrics from beads.
type MergeQueueStatus struct {
    // Depth is the number of items in the queue.
    Depth int32 `json:"depth,omitempty"`

    // Processing is the number of items currently being merged.
    Processing int32 `json:"processing,omitempty"`
}

// +kubebuilder:object:root=true
type RigList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Rig `json:"items"`
}
```

---

## 3. AgentPool CRD

AgentPool provides fine-grained pod templates for specific agent roles in
a rig. The controller creates one AgentPool per (rig, role) pair. When
beads emits "spawn polecat furiosa in gastown," the controller looks up
the AgentPool for (gastown, polecat) to build the pod spec.

AgentPools are typically auto-created by the controller from Rig
roleOverrides, but can be manually created for advanced cases.

### 3.1 Example YAML

```yaml
apiVersion: gastown.io/v1alpha1
kind: AgentPool
metadata:
  name: gastown-polecats
  namespace: gastown
  labels:
    gastown.io/town: production
    gastown.io/rig: gastown
    gastown.io/role: polecat
spec:
  # Must match beads rig and role names
  rig: gastown
  role: polecat

  # Pod template for agents in this pool.
  # Final merge order: GasTown defaults < Rig overrides < AgentPool template.
  template:
    image: 909418727440.dkr.ecr.us-east-1.amazonaws.com/gastown-agent:latest
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: "2"
        memory: 4Gi
    storage:
      size: 10Gi
      storageClassName: gp3
    nodeSelector:
      node-type: agent-burst
    tolerations:
      - key: agent-workload
        operator: Exists
        effect: NoSchedule
    screenScrollback: 10000

# Status projected from beads
status:
  ready: 3
  active: 3
  pending: 0
  agents:
    - name: furiosa
      phase: Running
      hookBead: "gt-naa65p.1"
      podName: polecat-gastown-furiosa-x7k2m
      startedAt: "2026-02-06T20:00:00Z"
    - name: nux
      phase: Running
      hookBead: "gt-abc123"
      podName: polecat-gastown-nux-p3n9q
      startedAt: "2026-02-06T20:05:00Z"
    - name: slit
      phase: Running
      hookBead: "gt-def456"
      podName: polecat-gastown-slit-m1v4w
      startedAt: "2026-02-06T20:10:00Z"
```

### 3.2 Go Types

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.ready
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Rig",type=string,JSONPath=`.spec.rig`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentPool defines the pod template for a specific (rig, role) pair.
// The controller watches beads for lifecycle events and uses this template
// to create agent pods. Beads decides WHEN to spawn — AgentPool defines HOW.
type AgentPool struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   AgentPoolSpec   `json:"spec,omitempty"`
    Status AgentPoolStatus `json:"status,omitempty"`
}

// AgentPoolSpec defines the pod template for a role in a rig.
type AgentPoolSpec struct {
    // Rig must match the beads rig name.
    Rig string `json:"rig"`

    // Role must match the beads role name.
    // +kubebuilder:validation:Enum=polecat;crew;witness;refinery;mayor;deacon
    Role string `json:"role"`

    // Template defines the pod specification for agents in this pool.
    // Overrides GasTown defaults and Rig overrides.
    Template PodDefaultsSpec `json:"template"`
}

// AgentPoolStatus is projected from beads state for this (rig, role) pair.
type AgentPoolStatus struct {
    // Ready is the number of running agents (from beads).
    Ready int32 `json:"ready,omitempty"`

    // Active is the number of agents with hooked work (from beads).
    Active int32 `json:"active,omitempty"`

    // Pending is the number of agents starting up.
    Pending int32 `json:"pending,omitempty"`

    // Agents lists individual agent statuses (from beads).
    // +optional
    Agents []AgentInstanceStatus `json:"agents,omitempty"`
}

// AgentInstanceStatus reports an individual agent's state.
// All fields except PodName are projected from beads.
type AgentInstanceStatus struct {
    // Name is the agent name from beads (e.g., "furiosa").
    Name string `json:"name"`

    // Phase is the agent lifecycle phase from beads.
    // +kubebuilder:validation:Enum=Pending;Starting;Running;Completing;Terminated
    Phase string `json:"phase"`

    // HookBead is the bead ID the agent is working on (from beads).
    // +optional
    HookBead string `json:"hookBead,omitempty"`

    // PodName is the K8s pod running this agent (set by controller).
    // +optional
    PodName string `json:"podName,omitempty"`

    // StartedAt is when the agent started (from beads).
    // +optional
    StartedAt *metav1.Time `json:"startedAt,omitempty"`
}

// +kubebuilder:object:root=true
type AgentPoolList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []AgentPool `json:"items"`
}
```

---

## 4. Controller Event Loop

The controller is a single loop that watches beads events via the BD Daemon
RPC connection and reacts:

### 4.1 Beads Event → Controller Action

| Beads Event | Source | Controller Action |
|-------------|--------|-------------------|
| Agent spawned | `gt sling`, witness | Look up AgentPool for (rig, role) → create Pod |
| Agent completed | `gt done` | Find Pod by agent name → delete Pod |
| Agent killed | Witness kill | Find Pod by agent name → delete Pod |
| Crew started | `gt crew start` | Look up AgentPool for (rig, crew) → create Pod |
| Crew stopped | `gt crew stop` | Find Pod by agent name → delete Pod |
| Rig added | `gt rig add` | Create Rig CR + AgentPool CRs (if not exist) |
| Rig removed | `gt rig remove` | Delete Rig CR + AgentPool CRs |

### 4.2 Pod Creation Flow

When the controller observes a "spawn" event from beads:

```
1. Beads event: {rig: "gastown", role: "polecat", name: "furiosa", bead: "gt-naa65p.1"}

2. Controller resolves pod template (merge order):
   a. GasTown.spec.defaults         (base)
   b. Rig(gastown).spec.podOverrides  (rig override)
   c. Rig(gastown).spec.roleOverrides["polecat"]  (role override)
   d. AgentPool(gastown-polecats).spec.template   (pool override, if exists)

3. Controller builds Pod:
   metadata:
     name: polecat-gastown-furiosa-<hash>
     labels:
       gastown.io/town: production
       gastown.io/rig: gastown
       gastown.io/role: polecat
       gastown.io/agent: furiosa
   spec:
     containers:
       - name: agent
         image: <resolved from template merge>
         env:
           - GT_ROLE=gastown/polecats/furiosa  (from beads event)
           - GT_RIG=gastown                     (from beads event)
           - BD_DAEMON_HOST=...                 (from GasTown.spec.daemon)
           - BD_DAEMON_PORT=...                 (from GasTown.spec.daemon)
           - <secrets from template>
         resources: <resolved from template merge>
     nodeSelector: <resolved from template merge>
     tolerations: <resolved from template merge>

4. Controller creates Pod via K8s API
5. Controller updates AgentPool status (adds agent entry)
```

### 4.3 Status Projection

The controller periodically queries beads state and projects it to CRD
status subresources:

```
Every 30s (configurable):
  1. Query BD Daemon for town status  → update GasTown.status
  2. Query BD Daemon for each rig     → update Rig.status
  3. Query BD Daemon for agent pools  → update AgentPool.status
  4. Cross-reference with actual K8s Pods for PodName fields
```

Status is **eventually consistent** — beads is the source of truth.
CRD status may lag beads by up to one polling interval. This is by
design: the controller does not need real-time status to function.

---

## 5. Template Merge Hierarchy

Pod templates merge in a layered cascade. More specific layers override
less specific ones. Only fields that are explicitly set in an override
are merged — unset fields inherit from the parent layer.

```
Layer 1: GasTown.spec.defaults           (town-wide base)
Layer 2: Rig.spec.podOverrides            (rig-level override)
Layer 3: Rig.spec.roleOverrides[role]     (role-level override)
Layer 4: AgentPool.spec.template          (pool-level override, optional)
```

Example resolution for a polecat in the gastown rig:

| Field | GasTown | Rig | Role | AgentPool | Resolved |
|-------|---------|-----|------|-----------|----------|
| image | agent:latest | — | — | — | agent:latest |
| cpu request | 500m | 1 | 500m | — | 500m |
| memory limit | 4Gi | 8Gi | 4Gi | — | 4Gi |
| nodeSelector | — | — | {node-type: burst} | — | {node-type: burst} |
| storage size | 10Gi | 20Gi | — | — | 20Gi |

---

## 6. Resource Ownership

```
Helm (manages infrastructure):
├── Dolt StatefulSet + Service
├── BD Daemon Deployment + Service
├── Controller Deployment
├── Redis (optional)
├── ExternalSecrets
└── ServiceAccounts, PDBs, ConfigMaps

Controller (reactive to beads events):
├── GasTown CR → owned by Helm (created during install)
├── Rig CR → owned by GasTown CR
├── AgentPool CR → owned by Rig CR
└── Agent Pods → owned by AgentPool CR
    ├── polecat-gastown-furiosa-x7k2m
    ├── polecat-gastown-nux-p3n9q
    ├── crew-gastown-colonization-a1b2c
    ├── witness-gastown-m4n5o
    └── refinery-gastown-p6q7r
```

ownerReferences ensure cascading deletion:
- Delete GasTown CR → Rig CRs deleted → AgentPool CRs deleted → Pods deleted
- But infrastructure (Dolt, Daemon) is NOT affected — Helm owns those

---

## 7. Design Decisions

### 7.1 Beads drives, controller reacts

The controller makes NO lifecycle decisions. It doesn't decide when to
spawn polecats, how many to run, or when to kill them. Beads (via
witnesses, the sling system, and gt done) makes all those decisions.
The controller just translates them into pod operations.

**Why?** Beads already has sophisticated lifecycle management — witnesses
monitor health, molecules track work steps, the merge queue manages
ordering. Reimplementing this in a K8s controller would be duplication
and divergence.

### 7.2 Infrastructure stays in Helm

Dolt, BD Daemon, Redis, and the controller itself are deployed by Helm.
The controller does NOT manage StatefulSets, Deployments, Services, etc.

**Why?** These are standard infrastructure components with well-understood
Helm patterns (existing charts work). Putting them in a CRD would just
add indirection with no benefit. The controller only manages the dynamic
part: agent pods that come and go based on beads events.

### 7.3 CRD spec is K8s-only concerns

CRD spec fields are limited to things K8s needs to know: container image,
resource limits, node selectors, storage, scheduling. Beads concerns
(merge strategy, naming pools, workflow formulas, role definitions) stay
in beads config files.

**Why?** Beads config is already managed by `gt config` commands and
stored in Dolt. Duplicating it in CRDs would create two sources of truth.
The CRD only contains what the controller needs to build pods.

### 7.4 Status is projection, not truth

CRD status is populated by querying beads and projecting the results.
The controller never computes agent state — it reads it from beads.
Status may lag by one polling interval (30s default).

**Why?** Beads tracks hooks, molecules, agent health, merge queue depth,
etc. The controller would need to reimplement all of that to compute
status independently. Instead, it just asks beads and writes what it gets.

### 7.5 Three CRDs for template layering

GasTown, Rig, and AgentPool form a merge hierarchy for pod templates.
This matches the existing config hierarchy (town → rig → role) without
duplicating beads config.

### 7.6 Namespace-scoped CRDs

All CRDs are namespace-scoped for standard RBAC isolation and to support
multiple Gas Towns in one cluster.

---

## 8. Full Deployment Example

### What Helm installs (infrastructure + controller):

```yaml
# helm install gastown ./charts/gastown -f values.yaml
# Creates: Dolt StatefulSet, BD Daemon Deployment, Controller Deployment,
#          Services, Secrets, GasTown CR
```

### What the user applies (rig configuration):

```
k8s/
├── rig-gastown.yaml    # Rig CR for gastown repo
└── rig-beads.yaml      # Rig CR for beads repo
```

### rig-gastown.yaml
```yaml
apiVersion: gastown.io/v1alpha1
kind: Rig
metadata:
  name: gastown
  namespace: gastown
  labels:
    gastown.io/town: production
spec:
  name: gastown
  roleOverrides:
    polecat:
      nodeSelector:
        node-type: agent-burst
      resources:
        requests:
          cpu: 500m
          memory: 1Gi
        limits:
          cpu: "2"
          memory: 4Gi
    crew:
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "4"
          memory: 8Gi
      storage:
        size: 50Gi
```

### rig-beads.yaml
```yaml
apiVersion: gastown.io/v1alpha1
kind: Rig
metadata:
  name: beads
  namespace: gastown
  labels:
    gastown.io/town: production
spec:
  name: beads
  podOverrides:
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: "2"
        memory: 4Gi
```

### What happens at runtime:

```bash
# Beads decides to spawn a polecat (e.g., via gt sling)
# → BD Daemon emits lifecycle event
# → Controller observes event
# → Controller looks up Rig(gastown).roleOverrides.polecat
# → Controller creates Pod with merged template
# → Pod starts, runs claude, picks up hooked work from beads

# Polecat runs gt done
# → Beads records completion
# → BD Daemon emits lifecycle event
# → Controller observes event
# → Controller deletes Pod
```

---

## 9. Mapping: Existing Config vs CRD

| Concern | Where It Lives | CRD Involvement |
|---------|---------------|-----------------|
| Town identity (name, owner) | Beads (town.json) | GasTown.spec.name (label only) |
| Rig identity (name, repo, branch) | Beads (rig config.json) | Rig.spec.name (correlation key) |
| Merge queue config | Beads (rig settings.json) | None — beads owns this |
| Namepool config | Beads (rig settings.json) | None — beads owns this |
| Workflow/formula config | Beads (rig settings.json) | None — beads owns this |
| Agent presets (RuntimeConfig) | Beads (town/rig settings) | None — beads owns this |
| Role definitions | Beads (roles/*.toml) | None — beads owns this |
| Messaging, escalation, accounts | Beads (config files) | None — beads owns this |
| Pod image | CRD (PodDefaultsSpec) | GasTown/Rig/AgentPool spec |
| Pod resources (CPU, memory) | CRD (PodDefaultsSpec) | GasTown/Rig/AgentPool spec |
| Pod storage (PVC size, class) | CRD (PodDefaultsSpec) | GasTown/Rig/AgentPool spec |
| Pod scheduling (nodeSelector) | CRD (PodDefaultsSpec) | GasTown/Rig/AgentPool spec |
| Pod secrets (API keys, git creds) | CRD (env secretKeyRef) | GasTown/Rig/AgentPool spec |
| BD Daemon connection | CRD (DaemonConnectionSpec) | GasTown.spec.daemon |
| Agent lifecycle state | Beads (Dolt) | Projected to CRD status |
| Agent hook/bead assignment | Beads (Dolt) | Projected to CRD status |
| Merge queue depth | Beads (Dolt) | Projected to CRD status |

**Rule of thumb**: If beads already tracks it → don't put it in the CRD.
If it's a K8s pod concern → put it in the CRD.
