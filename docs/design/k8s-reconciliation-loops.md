# K8s Controller Reconciliation Loops — Beads-First Design

> **Phase 7 Deliverable**: gt-naa65p.2
> **Status**: DESIGN
> **Author**: gastown/polecats/nux
> **Date**: 2026-02-06

## Core Principle: Beads IS the Control Plane

The GasTown K8s controller is **not** a traditional Kubernetes operator. It does not
implement reconciliation logic in Go using controller-runtime watches and informers.

Instead:

```
CRD Spec (desired state)
    │
    ▼
┌─────────────────────────┐
│   Thin Bridge Controller │  ← Translates CRD fields to bd/gt commands
│   (K8s ↔ Beads)         │
└─────────────────────────┘
    │                ▲
    ▼                │
┌─────────────────────────┐
│   BEADS                  │  ← The actual control plane
│   Agent beads, hooks,    │
│   mail, molecules,       │
│   dependencies, state    │
└─────────────────────────┘
    │                ▲
    ▼                │
┌─────────────────────────┐
│   Agent Pods             │  ← Claude Code agents running gt/bd
│   (Mayor, Witness, etc.) │     They self-manage via beads
└─────────────────────────┘
```

**Why?** Beads already handles:
- Agent identity and lifecycle (agent beads with state=spawning/running/completed)
- Work assignment (hook_bead, molecules, convoys)
- Communication (mail protocol)
- Health monitoring (Witness patrols, Deacon heartbeats)
- Dependency tracking (bd dep, bd blocked)
- Merge queue (Refinery processes MQ beads)

Reimplementing this in controller-runtime Go code would duplicate the entire Gas Town
brain. The controller's job is to **bridge** between K8s declarative YAML and the beads
operations that already work.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                         │
│                                                              │
│  ┌─────────────┐    ┌──────────────────────────────────┐    │
│  │  CRD:       │    │  Controller Pod                   │    │
│  │  GasTown    │───▶│                                   │    │
│  │  Rig        │    │  1. Watch CRD changes             │    │
│  └─────────────┘    │  2. Translate to bd/gt operations  │    │
│                     │  3. Ensure pods exist for agents   │    │
│                     │  4. Read beads state → CRD status  │    │
│                     └───────────┬───────────────────────┘    │
│                                 │                            │
│                    ┌────────────┼────────────────┐           │
│                    ▼            ▼                ▼           │
│              ┌──────────┐ ┌──────────┐    ┌──────────┐      │
│              │ Dolt     │ │ bd-daemon│    │ Agent    │      │
│              │ Stateful │ │ Deploy   │    │ Pods     │      │
│              │ Set      │ │          │    │          │      │
│              └──────────┘ └──────────┘    └──────────┘      │
│                                                              │
│  Agent pods run Claude Code with gt/bd CLI.                  │
│  They self-organize via beads — the controller doesn't       │
│  micromanage their behavior.                                 │
└──────────────────────────────────────────────────────────────┘
```

## What the Controller Does vs. What Beads Does

| Concern | Controller (thin bridge) | Beads (control plane) |
|---------|------------------------|-----------------------|
| Desired state declaration | Reads CRD spec | N/A |
| Agent identity | Creates agent bead at pod spawn | Owns agent bead lifecycle |
| Work dispatch | N/A | gt sling, hook_bead, molecules |
| Health monitoring | Liveness/readiness probes | Witness patrols, Deacon heartbeats |
| Agent recovery | Restarts failed pods | Witness detects, nudges, escalates |
| Mail routing | N/A | bd-daemon RPC handles all mail |
| Merge queue | N/A | Refinery processes MQ beads |
| Dependency tracking | N/A | bd dep, bd blocked |
| Pod lifecycle | Create/delete K8s pods | Agent beads track state transitions |
| Status reporting | Reads beads → writes CRD status | Source of truth for all state |
| Scaling polecats | N/A (polecats are demand-driven) | gt sling spawns on demand |
| Configuration | Reads CRD spec fields | Agents read config via bd/gt |

## Controller Reconciliation Loops

### Loop 1: GasTown Reconciler

**Trigger**: GasTown CRD create/update/delete

**What it manages** (K8s resources only):
- Dolt StatefulSet
- bd-daemon Deployment
- RPC server Deployment (gtmobile)
- Secrets (Anthropic keys, Slack tokens)
- Rig sub-resources (creates/updates Rig CRs)
- Mayor agent pod
- Deacon agent pod

```
GasTown Reconciler Pseudo-code
══════════════════════════════

func Reconcile(ctx, req) Result:
    gastown := GET GasTown CR from req.NamespacedName
    if gastown.DeletionTimestamp != nil:
        return handleDeletion(gastown)

    // ── Phase 1: Infrastructure (pure K8s) ──────────────
    //
    // These are standard K8s resources. The controller owns them directly
    // because they're infrastructure, not agent logic.

    reconcileDolt(gastown)
    reconcileBdDaemon(gastown)
    reconcileRpcServer(gastown)
    reconcileSecrets(gastown)

    // ── Phase 2: Rig sub-resources ──────────────────────
    //
    // Create/update Rig CRs based on gastown.spec.rigs[].
    // Each Rig CR triggers the Rig Reconciler (Loop 2).

    for rig in gastown.spec.rigs:
        ensureRigCR(gastown, rig)
    deleteOrphanedRigCRs(gastown)

    // ── Phase 3: Town-level agents ──────────────────────
    //
    // Mayor and Deacon are town-scoped singletons.
    // The controller ensures their pods exist. The agents
    // self-manage via beads once running.

    ensureAgentPod("mayor", gastown.spec.mayor)
    ensureAgentPod("deacon", gastown.spec.deacon)

    // ── Phase 4: Status from beads ──────────────────────
    //
    // Read beads state via bd-daemon RPC to populate CRD status.

    status := queryBeadsStatus(gastown)
    gastown.status = status
    UPDATE gastown status

    return Result{RequeueAfter: 60s}


func reconcileDolt(gastown):
    // Translate CRD spec to Dolt StatefulSet
    desired := buildDoltStatefulSet(
        image:       gastown.spec.dolt.image,
        storage:     gastown.spec.dolt.persistence.size,
        s3:          gastown.spec.dolt.s3,
        resources:   gastown.spec.dolt.resources,
    )
    existing := GET StatefulSet "gastown-dolt"
    if existing == nil:
        CREATE desired (owner: gastown)
    else if specChanged(existing, desired):
        UPDATE existing with desired spec
    // ConfigMap for dolt config.yaml
    ensureConfigMap("gastown-dolt-config", doltConfigFrom(gastown.spec.dolt))


func reconcileBdDaemon(gastown):
    desired := buildDaemonDeployment(
        image:     gastown.spec.daemon.image,
        port:      gastown.spec.daemon.port,       // default 9876
        httpPort:  gastown.spec.daemon.httpPort,    // optional
        resources: gastown.spec.daemon.resources,
    )
    existing := GET Deployment "gastown-bd-daemon"
    if existing == nil:
        CREATE desired (owner: gastown)
    else if specChanged(existing, desired):
        UPDATE existing with desired spec


func reconcileRpcServer(gastown):
    if !gastown.spec.rpcServer.enabled:
        DELETE Deployment "gastown-rpc" if exists
        return
    desired := buildRpcDeployment(gastown.spec.rpcServer)
    ensureDeployment("gastown-rpc", desired, owner: gastown)


func reconcileSecrets(gastown):
    // Create ExternalSecret CRs that pull from AWS Secrets Manager
    // (or equivalent) based on gastown.spec.auth
    for secretRef in gastown.spec.auth.secrets:
        ensureExternalSecret(secretRef, owner: gastown)


func ensureRigCR(gastown, rigSpec):
    desired := Rig{
        metadata: { name: rigSpec.name, namespace: gastown.namespace,
                    ownerRef: gastown },
        spec: rigSpec,
    }
    existing := GET Rig rigSpec.name
    if existing == nil:
        CREATE desired
    else if specChanged(existing.spec, rigSpec):
        UPDATE existing.spec = rigSpec


func ensureAgentPod(role, spec):
    // Agent pods are thin: they run Claude Code which self-manages via beads.
    // The controller just ensures the pod exists with the right env vars.
    podName := "gastown-" + role
    existing := GET Pod podName
    if existing == nil || existing.phase == Failed:
        if existing != nil:
            DELETE existing  // clean up failed pod
        pod := buildAgentPod(
            name:      podName,
            role:      role,
            image:     spec.image,
            resources: spec.resources,
            env: {
                GT_ROLE:          role,
                BD_DAEMON_HOST:   "gastown-bd-daemon",
                BD_DAEMON_PORT:   "9876",
                ANTHROPIC_API_KEY: secretRef("anthropic-api-key"),
            },
        )
        CREATE pod (owner: gastown)
    // Do NOT manage what the agent does inside the pod.
    // Mayor/Deacon self-organize via beads, mail, and gt commands.


func queryBeadsStatus(gastown) GasTownStatus:
    // Query bd-daemon RPC for aggregate state
    rigs := RPC bd-daemon.ListRigs()
    return GasTownStatus{
        phase:          "Running" if all healthy else "Degraded",
        rigCount:       len(rigs),
        doltReady:      isDoltReady(),
        daemonReady:    isDaemonReady(),
        mayorHealthy:   queryAgentHealth("mayor"),
        deaconHealthy:  queryAgentHealth("deacon"),
        rigs: [for rig in rigs: {
            name:           rig.name,
            polecatCount:   rig.polecatCount,
            crewCount:      rig.crewCount,
            witnessHealthy: rig.hasWitness,
            refineryHealthy: rig.hasRefinery,
        }],
        lastReconciled: now(),
    }
```

### Loop 2: Rig Reconciler

**Trigger**: Rig CRD create/update/delete (owned by GasTown CR)

**What it manages** (K8s resources only):
- Witness agent pod
- Refinery agent pod
- Crew member pods (persistent)
- PVC for git workspace
- ServiceAccount and RBAC per rig
- NetworkPolicy per rig

**What it does NOT manage** (beads handles these):
- Polecat spawning (demand-driven via gt sling in beads)
- Work assignment
- Mail routing
- Health monitoring logic

```
Rig Reconciler Pseudo-code
═══════════════════════════

func Reconcile(ctx, req) Result:
    rig := GET Rig CR from req.NamespacedName
    if rig.DeletionTimestamp != nil:
        return handleRigDeletion(rig)

    gastown := GET owner GasTown CR

    // ── Phase 1: Rig workspace ──────────────────────────
    //
    // Each rig needs a PVC with the git repo cloned.
    // Init containers handle git clone/fetch.

    ensureRigPVC(rig)
    ensureRigServiceAccount(rig)

    // ── Phase 2: Infrastructure agents ──────────────────
    //
    // Witness and Refinery are per-rig singletons.
    // They self-manage once running.

    ensureWitnessPod(rig, gastown)
    ensureRefineryPod(rig, gastown)

    // ── Phase 3: Crew pods ──────────────────────────────
    //
    // Crew members are persistent workers declared in the CRD.
    // Each gets its own pod with a PVC.

    reconcileCrewPods(rig, gastown)

    // ── Phase 4: Polecat ceiling ────────────────────────
    //
    // The controller does NOT spawn polecats. Polecats are spawned
    // on-demand when gt sling is called (by Witness, Mayor, or crew).
    //
    // The controller only enforces resource limits:
    // - ResourceQuota caps total polecat pod resources
    // - The maxPolecats field is read by gt sling inside agents

    ensurePolecatResourceQuota(rig)

    // ── Phase 5: Status from beads ──────────────────────

    status := queryRigBeadsStatus(rig)
    rig.status = status
    UPDATE rig status

    return Result{RequeueAfter: 60s}


func ensureRigPVC(rig):
    // Shared PVC for the rig's git repo (used by witness, refinery, crew)
    // Polecats get their own ephemeral volumes (emptyDir or per-pod PVC)
    pvcName := "rig-" + rig.spec.name + "-workspace"
    existing := GET PVC pvcName
    if existing == nil:
        CREATE PVC{
            name:         pvcName,
            storageClass: rig.spec.storageClass or "gp3",
            size:         rig.spec.workspaceSize or "10Gi",
            accessModes:  [ReadWriteOnce],
        } (owner: rig)


func ensureWitnessPod(rig, gastown):
    if !rig.spec.witness.enabled (default true):
        deleteIfExists(Pod, "rig-" + rig.spec.name + "-witness")
        return
    ensureAgentPod(
        name:   "rig-" + rig.spec.name + "-witness",
        role:   "witness",
        rig:    rig.spec.name,
        image:  rig.spec.witness.image or gastown.spec.defaults.agentImage,
        resources: rig.spec.witness.resources,
        pvc:    "rig-" + rig.spec.name + "-workspace",
        env: {
            GT_ROLE:        "witness",
            GT_RIG:         rig.spec.name,
            BD_DAEMON_HOST: "gastown-bd-daemon",
            BD_DAEMON_PORT: "9876",
        },
    )


func ensureRefineryPod(rig, gastown):
    if !rig.spec.refinery.enabled (default true):
        deleteIfExists(Pod, "rig-" + rig.spec.name + "-refinery")
        return
    ensureAgentPod(
        name:   "rig-" + rig.spec.name + "-refinery",
        role:   "refinery",
        rig:    rig.spec.name,
        image:  rig.spec.refinery.image or gastown.spec.defaults.agentImage,
        resources: rig.spec.refinery.resources,
        pvc:    "rig-" + rig.spec.name + "-workspace",
        env: {
            GT_ROLE:        "refinery",
            GT_RIG:         rig.spec.name,
            BD_DAEMON_HOST: "gastown-bd-daemon",
            BD_DAEMON_PORT: "9876",
        },
    )


func reconcileCrewPods(rig, gastown):
    // Crew members declared in CRD spec. Each is a persistent worker.
    desiredCrew := set(rig.spec.crew[].name)
    existingCrew := listPods(label: gastown.io/role=crew, gastown.io/rig=rig.name)

    // Create missing crew pods
    for crew in rig.spec.crew:
        if crew.name not in existingCrew:
            ensureAgentPod(
                name:   "rig-" + rig.spec.name + "-crew-" + crew.name,
                role:   "crew",
                rig:    rig.spec.name,
                image:  crew.image or gastown.spec.defaults.agentImage,
                resources: crew.resources,
                // Crew get their own PVC (they clone the repo independently)
                pvc:    createCrewPVC(rig, crew),
                env: {
                    GT_ROLE:        "crew",
                    GT_RIG:         rig.spec.name,
                    GT_CREW_NAME:   crew.name,
                    BD_DAEMON_HOST: "gastown-bd-daemon",
                    BD_DAEMON_PORT: "9876",
                },
            )

    // Delete crew pods removed from spec
    for existing in existingCrew:
        if existing.name not in desiredCrew:
            DELETE Pod existing


func ensurePolecatResourceQuota(rig):
    // Polecats are demand-driven. The controller doesn't spawn them.
    // It just sets the ceiling via ResourceQuota.
    maxPolecats := rig.spec.polecats.maxCount or 5
    quota := ResourceQuota{
        name: "rig-" + rig.spec.name + "-polecats",
        spec: {
            hard: {
                // Each polecat gets ~500m CPU, 1Gi memory
                "requests.cpu":    str(maxPolecats * 500) + "m",
                "requests.memory": str(maxPolecats) + "Gi",
                "pods":            str(maxPolecats),
            },
            scopeSelector: matchLabels({
                "gastown.io/role": "polecat",
                "gastown.io/rig":  rig.spec.name,
            }),
        },
    }
    ensureResourceQuota(quota, owner: rig)


func queryRigBeadsStatus(rig) RigStatus:
    // RPC to bd-daemon for rig state — beads is source of truth
    rigInfo := RPC bd-daemon.GetRigSummary(rig.spec.name)
    return RigStatus{
        phase:            rigInfo.healthy ? "Running" : "Degraded",
        polecatCount:     rigInfo.polecatCount,
        activePolecats:   rigInfo.activePolecatNames,
        crewCount:        rigInfo.crewCount,
        witnessHealthy:   rigInfo.hasWitness && rigInfo.witnessAlive,
        refineryHealthy:  rigInfo.hasRefinery && rigInfo.refineryAlive,
        mergeQueueDepth:  rigInfo.mqDepth,
        lastReconciled:   now(),
    }
```

## Polecat Lifecycle: Beads-Driven, Not Controller-Driven

This is the critical design difference. Polecats are **never spawned by the controller**.

```
Traditional K8s Operator (WRONG for Gas Town):
    Controller watches CRD → creates polecat Pods directly

Beads-First (CORRECT):
    Agent (Witness/Mayor/Crew) calls gt sling
        → gt sling creates agent bead, sets hook
        → gt sling calls bd-daemon RPC: "spawn polecat"
        → bd-daemon (or agent-dispatcher) creates K8s Pod
        → Pod starts Claude Code, runs gt prime, reads hook from beads
        → Polecat works, runs gt done
        → gt done marks bead complete, pod terminates
        → Controller notices pod terminated, cleans up
```

### Polecat Pod Creation Flow

```
                         ┌─────────────────┐
                         │  Witness/Mayor/  │
                         │  Crew Agent      │
                         │  (inside pod)    │
                         └────────┬─────────┘
                                  │
                            gt sling <bead> <rig>
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  gt CLI          │
                         │  (in agent pod)  │
                         │                  │
                         │  1. Validate bead│
                         │  2. Alloc name   │
                         │  3. Set hook_bead│
                         └────────┬─────────┘
                                  │
                          RPC: SpawnPolecat(name, rig, bead)
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  Agent Dispatcher│
                         │  (or bd-daemon)  │
                         │                  │
                         │  Creates K8s Pod │
                         │  with labels:    │
                         │   role=polecat   │
                         │   rig=<rig>      │
                         │   bead=<bead-id> │
                         └────────┬─────────┘
                                  │
                            kubectl create pod
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  Polecat Pod     │
                         │                  │
                         │  Init:           │
                         │   git clone/     │
                         │   worktree setup │
                         │                  │
                         │  Main:           │
                         │   claude-code    │
                         │   → gt prime     │
                         │   → reads hook   │
                         │   → does work    │
                         │   → gt done      │
                         │   → pod exits    │
                         └─────────────────┘
```

### Why the Controller Stays Out of Polecat Spawning

1. **Beads already tracks demand**: When work is slung, a bead exists with `hook_bead`
   and `assignee`. This IS the demand signal.
2. **Agents make dispatch decisions**: Witness knows which polecats are stuck, Mayor
   knows priorities, Crew knows domain context. The controller has none of this.
3. **Name allocation is beads-side**: The themed NamePool (nux, furiosa, etc.) lives
   in the gt CLI and beads, not in K8s.
4. **Lifecycle is already correct**: gt sling → work → gt done is the complete lifecycle.
   K8s just needs to run the pod and let it terminate.

## Controller Awareness: What the Controller Watches (Passively)

The controller does have a **passive watch** on polecat pods for cleanup:

```
Polecat Cleanup Loop (lightweight)
═══════════════════════════════════

// Runs as part of Rig reconciliation, not as a separate controller.

func cleanupTerminatedPolecats(rig):
    pods := listPods(
        label: gastown.io/role=polecat, gastown.io/rig=rig.name,
        fieldSelector: status.phase in [Succeeded, Failed],
    )
    for pod in pods:
        age := now() - pod.status.completionTime
        if age > 5m:  // grace period for log collection
            DELETE pod
            // Optional: emit event for audit trail

func detectStuckPolecats(rig):
    // This is informational only. The Witness handles actual recovery.
    pods := listPods(
        label: gastown.io/role=polecat, gastown.io/rig=rig.name,
        fieldSelector: status.phase=Running,
    )
    for pod in pods:
        runtime := now() - pod.status.startTime
        if runtime > rig.spec.polecats.maxRuntime (default 2h):
            // Annotate pod for Witness to notice, but do NOT kill it.
            // Witness decides whether to nudge, escalate, or kill.
            ANNOTATE pod: gastown.io/overtime=true
            // Update rig status condition
            addCondition(rig, "PolecatOvertime", pod.name)
```

## Status Reporting: Beads → CRD Status

The controller's status reporting is a **read-only projection** of beads state.

```
Status Sync Loop
════════════════

// Triggered by periodic requeue (every 60s) or by pod events.

func syncStatus(gastown):
    // The controller queries bd-daemon via RPC for authoritative state.
    // It NEVER derives agent state from pod status alone.
    //
    // Pod running ≠ agent healthy (Claude might be crashed inside)
    // Pod terminated ≠ work failed (gt done causes clean termination)
    //
    // Beads knows the truth. Pods are just containers.

    daemonClient := connect(gastown-bd-daemon:9876)

    // Town-level status
    mayorBead  := daemonClient.GetAgentBead("mayor")
    deaconBead := daemonClient.GetAgentBead("deacon")

    gastown.status.mayorPhase   = mayorBead.state     // "running", "idle", etc.
    gastown.status.deaconPhase  = deaconBead.state

    // Per-rig status
    for rig in gastown.status.rigs:
        rigSummary := daemonClient.GetRigSummary(rig.name)
        rig.polecatCount    = rigSummary.polecatCount
        rig.crewCount       = rigSummary.crewCount
        rig.witnessHealthy  = rigSummary.witnessAlive
        rig.refineryHealthy = rigSummary.refineryAlive
        rig.mqDepth         = rigSummary.mergeQueueDepth
        rig.activeWork      = rigSummary.hookedBeadCount

    gastown.status.lastSynced = now()
    gastown.status.phase = deriveOverallPhase(gastown.status)
```

### Status Conditions

```yaml
status:
  phase: Running
  conditions:
    - type: DoltReady
      status: "True"
      lastTransitionTime: "2026-02-06T22:00:00Z"
    - type: DaemonReady
      status: "True"
    - type: MayorHealthy
      status: "True"
    - type: DeaconHealthy
      status: "True"
    - type: RigsReconciled
      status: "True"
      message: "2/2 rigs reconciled"
  rigs:
    - name: gastown
      phase: Running
      polecatCount: 3
      crewCount: 1
      witnessHealthy: true
      refineryHealthy: true
      mergeQueueDepth: 1
    - name: beads
      phase: Running
      polecatCount: 1
      crewCount: 0
      witnessHealthy: true
      refineryHealthy: true
      mergeQueueDepth: 0
```

## Deletion and Teardown

### GasTown Deletion

```
func handleGasTownDeletion(gastown):
    // Finalizer: gastown.io/cleanup
    //
    // Graceful teardown order matters:
    // 1. Stop spawning new polecats (annotate rigs)
    // 2. Wait for active polecats to finish (with timeout)
    // 3. Drain merge queues
    // 4. Stop Witness/Refinery/Crew
    // 5. Stop Mayor/Deacon
    // 6. Delete Rig CRs (cascades to rig resources)
    // 7. Delete infrastructure (daemon, dolt)
    // 8. Remove finalizer

    // Phase 1: Signal shutdown via beads
    for rig in gastown.spec.rigs:
        ANNOTATE Rig CR: gastown.io/draining=true

    // Phase 2: Wait for polecats with timeout
    deadline := now() + gastown.spec.terminationGracePeriod (default 10m)
    while activePolecats() > 0 && now() < deadline:
        wait(30s)

    // Phase 3: Force-kill remaining polecats
    deleteAllPods(label: gastown.io/role=polecat)

    // Phase 4: Delete Rig CRs (triggers rig cleanup)
    for rig in listRigCRs(owner: gastown):
        DELETE rig

    // Phase 5: Delete infrastructure
    DELETE StatefulSet "gastown-dolt"    // PVC retained by default
    DELETE Deployment "gastown-bd-daemon"
    DELETE Deployment "gastown-rpc"

    // Phase 6: Remove finalizer
    REMOVE finalizer "gastown.io/cleanup"
```

### Rig Deletion

```
func handleRigDeletion(rig):
    // Finalizer: gastown.io/rig-cleanup

    // 1. Kill polecats (already ephemeral, just accelerate)
    deleteAllPods(label: gastown.io/role=polecat, gastown.io/rig=rig.name)

    // 2. Stop crew, witness, refinery
    deleteAllPods(label: gastown.io/rig=rig.name)

    // 3. Clean up PVCs (if retention policy allows)
    if rig.spec.persistence.reclaimPolicy == "Delete":
        DELETE PVC "rig-" + rig.name + "-workspace"
        for crew in rig.spec.crew:
            DELETE PVC "rig-" + rig.name + "-crew-" + crew.name

    REMOVE finalizer "gastown.io/rig-cleanup"
```

## Key Design Decisions

### 1. tmux → K8s Pod Mapping

| tmux Concept | K8s Equivalent |
|-------------|----------------|
| tmux session | Pod |
| `remain-on-exit` | Pod restart policy: Never (polecats), Always (persistent agents) |
| `send-keys` / nudge | kubectl exec or bd-daemon RPC |
| `capture-pane` / peek | kubectl logs or bd-daemon RPC |
| `kill-session` | Delete pod |
| Session name (`gt-gastown-nux`) | Pod name (`rig-gastown-polecat-nux`) |

### 2. Pod Types by Agent Role

| Agent | K8s Kind | Restart | Storage | Replicas |
|-------|----------|---------|---------|----------|
| Mayor | Pod (bare) | Always | emptyDir | 1 |
| Deacon | Pod (bare) | Always | emptyDir | 1 |
| Witness | Pod (bare) | Always | Shared rig PVC | 1 per rig |
| Refinery | Pod (bare) | Always | Shared rig PVC | 1 per rig |
| Crew | Pod (bare) | Always | Own PVC | N (declared) |
| Polecat | Pod (bare) | Never | emptyDir | On-demand |

**Why bare Pods, not Deployments?** Agent pods are singletons managed by the controller.
Deployments add rollout complexity that conflicts with beads-driven lifecycle. The
controller directly creates/deletes pods and handles restarts itself.

**Alternative**: Use Deployments with `replicas: 1` for Witness/Refinery/Crew for
automatic restart. This is acceptable if the controller delegates restart entirely
to K8s. The tradeoff is less control over restart timing (beads might want to delay
restart until state is clean).

### 3. gt sling in K8s

In the current tmux model, `gt sling` calls `SpawnPolecatForSling()` which creates
a worktree and tmux session locally. In K8s:

```
gt sling <bead> <rig>
    │
    ├── Same: validate bead, allocate name, set hook_bead
    │
    └── Different: instead of tmux session, call dispatcher RPC
            │
            RPC: agent-dispatcher.SpawnPolecat(SpawnRequest{
                name:    "nux",
                rig:     "gastown",
                beadId:  "gt-abc123",
                image:   <from rig config>,
                resources: <from rig config>,
            })
            │
            └── Dispatcher creates K8s Pod with:
                  - Init container: git clone + worktree setup
                  - Main container: claude-code with gt prime
                  - Labels: gastown.io/role=polecat, gastown.io/rig=gastown
                  - Env: GT_ROLE=polecat, hook_bead=gt-abc123
```

**Detection**: `gt sling` checks `GT_RUNTIME=k8s` env var to choose between
tmux path and K8s dispatcher path. The beads operations are identical.

### 4. Mail Routing in K8s

Mail routing works through bd-daemon RPC — no change needed. All agents connect
to `gastown-bd-daemon:9876` and use the same mail protocol. The bd-daemon is
a centralized service (Deployment), so all agents in the namespace can reach it.

```
Agent A (pod) → bd-daemon RPC → stores in beads → Agent B (pod) reads via RPC
```

### 5. Peek/Nudge in K8s

| Operation | tmux Model | K8s Model |
|-----------|-----------|-----------|
| peek | `tmux capture-pane` | `kubectl logs` or bd-daemon RPC (agent status) |
| nudge | `tmux send-keys` | bd-daemon RPC: `NudgeAgent(name, message)` |
| attach | `tmux attach` | SSH into pod (Phase 5) or `kubectl exec` |

The bd-daemon RPC needs new endpoints:
- `NudgeAgent(rig, name, message)` — writes a nudge bead, agent picks it up
- `GetAgentLogs(rig, name, lines)` — reads recent Claude output
- `GetAgentStatus(rig, name)` — returns agent bead state

## Requeue Strategy

```
Event                          → Requeue
─────                             ──────
CRD spec change                → Immediate (0s)
Infrastructure pod ready       → Immediate (0s)
Agent pod terminated           → 5s (allow beads to update)
Periodic health sync           → 60s
Polecat cleanup                → 60s (as part of rig reconcile)
Deletion in progress           → 30s (check drain progress)
Error during reconcile         → 30s with exponential backoff
```

## Summary: What Each Component Owns

```
┌──────────────────────────────────────────────────────┐
│ Controller (thin bridge)                              │
│                                                      │
│ OWNS:                                                │
│   • K8s resource CRUD (pods, PVCs, configmaps, etc.) │
│   • CRD status updates (from beads data)             │
│   • Terminated pod cleanup                           │
│   • Resource quota enforcement                       │
│   • Graceful deletion orchestration                  │
│                                                      │
│ DOES NOT OWN:                                        │
│   • Agent behavior                                   │
│   • Work dispatch                                    │
│   • Health monitoring logic                          │
│   • Recovery decisions                               │
│   • Mail routing                                     │
│   • Merge queue processing                           │
│   • Polecat spawning decisions                       │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│ Beads (control plane)                                 │
│                                                      │
│ OWNS:                                                │
│   • Agent identity and lifecycle state               │
│   • Work assignment (hooks, molecules, convoys)      │
│   • Health monitoring (Witness patrols)              │
│   • Recovery (nudge, escalate, respawn)              │
│   • Mail routing                                     │
│   • Merge queue (Refinery)                           │
│   • Dependencies (bd dep, bd blocked)                │
│   • Configuration (bd config, advice)                │
│   • Polecat demand (via gt sling)                    │
└──────────────────────────────────────────────────────┘
```

The controller is the **hands**. Beads is the **brain**.
