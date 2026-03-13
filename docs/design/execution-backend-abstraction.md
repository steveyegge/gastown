# Execution Backend Abstraction Layer

> Design document for pluggable remote execution in Gas Town.
> tmux remains the universal session multiplexer. The exec-wrapper plugin
> system (PR #2689) provides the injection point for remote backends like
> Daytona. This replaces the earlier proposal to abstract tmux away entirely.

## Motivation

Gas Town currently has tmux hardcoded throughout session management, dispatch,
and patrol systems. The Daytona integration (PR #2594, `feat/daytona-polecats`)
demonstrated that remote execution is viable — the engineering produced a
complete mTLS proxy, reconciliation state machine, and lifecycle manager across
~6,600 lines — but the integration threads through ~20 internal packages with
`isRemoteMode()` conditionals because there was no clean injection point.

The reviewer feedback identified three missing abstractions. The original design
proposed replacing tmux with an `ExecutionBackend` interface. However, examining
the actual Daytona code reveals that **tmux is already the session multiplexer
for remote mode** — `buildDaytonaCommand()` produces a `daytona exec` string
that runs as the tmux pane command. The Daytona branch doesn't replace tmux; it
wraps the agent command that tmux executes.

This insight, combined with the exec-wrapper plugin type landed in PR #2689,
yields a simpler architecture:

1. **tmux stays** as the universal session backend — no `ExecutionBackend` interface
2. **Exec-wrapper plugins** inject remote execution (Daytona, SSH, etc.) between
   env vars and the agent binary in the tmux pane command
3. **Sandbox lifecycle hooks** on exec-wrapper plugins handle workspace
   create/start/stop/destroy around session lifecycle
4. **DispatchTarget interface** still cleans up the sling system (unchanged)
5. **PatrolRegistry** still cleans up the daemon (unchanged)

### Why this is better than the original design

The original `ExecutionBackend` interface required reimplementing 11 methods
(`HasSession`, `CreateSession`, `KillSession`, `SetEnv`, `GetEnv`, `SendKeys`,
`CaptureOutput`, `CheckHealth`, `WaitReady`, `Attach`, `ListSessions`) for
every backend. But for Daytona, the answers are:

- `HasSession` → tmux `has-session` (the tmux session wrapping `daytona exec`)
- `KillSession` → tmux `kill-session` (kills the `daytona exec` process)
- `SetEnv` / `GetEnv` → tmux `set-environment` / `show-environment`
- `SendKeys` → tmux `send-keys` (stdin forwarded through `daytona exec --tty`)
- `CaptureOutput` → tmux `capture-pane` (stdout comes back through tty)
- `CheckHealth` → tmux pane process check (is `daytona` alive?)
- `Attach` → tmux `attach-session` (terminal attaches to the `daytona exec` pane)
- `ListSessions` → tmux `list-sessions`

Every single method delegates to tmux. The "DaytonaBackend" would be a thin
wrapper that calls tmux for 10 of 11 methods and only differs in `CreateSession`
(where it prepends `daytona exec` to the command). That's not an interface —
that's a command prefix. Which is exactly what exec-wrapper already is.

---

## Inventory: What the Daytona Branch Built

Before designing the integration, here is what exists in `feat/daytona-polecats`
and must be preserved:

### internal/daytona/ — Client, Reconciliation, Retry

| File | Lines | Purpose |
|------|-------|---------|
| `client.go` | 670 | CLI wrapper: workspace CRUD, snapshot lifecycle, exec, cert volumes |
| `reconcile.go` | 360 | Discovery of orphaned workspaces/beads, state machine cleanup |
| `retry.go` | 142 | Exponential backoff with transience classification |
| `client_test.go` | 1,041 | Workspace naming, options, lifecycle, pagination |
| `reconcile_test.go` | 1,091 | Discovery, state machine, zombie detection, cert revocation |
| `retry_test.go` | 417 | Transience classification, backoff, context cancellation |

**Key design decisions to preserve:**
- **Multi-tenancy scoping**: Workspace names `gt-<installID>-<rig>--<polecat>`
- **Transience classification**: Distinguishes retryable (OS, timeout) from
  permanent (auth, quota) failures — retry only on transient
- **Spawning bead protection**: Beads in "spawning" state skipped during
  reconciliation to prevent race with concurrent provisioning
- **Per-operation timeouts**: Each orphan gets independent deadline during
  reconciliation (30s default), preventing one slow op from starving others
- **Cert revocation ordering**: Revoke BEFORE bead reset (reset clears serial)
- **Idempotent operations**: Snapshot creation handles "already exists";
  deletion ignores "not found"

### internal/proxy/ — mTLS Proxy Server

| File | Lines | Purpose |
|------|-------|---------|
| `admin_client.go` | 137 | Cert issuance and revocation via admin API |
| `server.go` | ~500 | mTLS HTTP server: /v1/exec, /v1/git/, /v1/admin/ |
| `exec.go` | ~200 | Command execution with allowlisting |
| `git.go` | ~300 | Git smart-HTTP protocol bridge with ref authorization |

**Key design decisions to preserve:**
- **Command allowlisting**: Only pre-approved commands, resolved at startup
  via `exec.LookPath` to prevent PATH hijacking
- **Git push authorization**: Polecats can only push to
  `refs/heads/polecat/<name>-*` or `refs/heads/polecat/<name>/*`
- **Environment isolation**: Subprocesses get minimal env (HOME, PATH) plus
  identity injection (GT_PROXY_IDENTITY, GT_RIG, GT_POLECAT)
- **Nil-safe AdminClient**: Methods return nil on nil receiver, allowing
  graceful degradation when proxy isn't running

### internal/daemon/ — Integration Tests

| File | Lines | Purpose |
|------|-------|---------|
| `daytona_reconcile_test.go` | 306 | Workspace discovery, per-rig reconciliation |
| `daytona_restart_test.go` | 450 | Session restart, state machine, cert cleanup |
| `daytona_statemachine_test.go` | 592 | Full state transition testing with mock daytona |

### Dockerfile.daytona (106 lines)

Multi-stage build: Go builder → Debian Bookworm slim runtime with Node.js,
Go SDK, claude-code, and gt-proxy-client. Non-root user (uid 1000).

---

## Architecture: tmux + Exec-Wrapper

### The execution stack

Every agent session in Gas Town runs inside a tmux pane. What varies is the
command that the pane executes:

```
┌─────────────────────────────────────────────────────────────┐
│ tmux session (always present)                               │
│                                                             │
│  Local polecat:                                             │
│  exec env GT_RIG=foo GT_POLECAT=bar ... claude --prompt "…" │
│                                                             │
│  Daytona polecat:                                           │
│  exec env GT_RUN=abc ... \                                  │
│    daytona exec gt-inst-rig--bar --tty -- \                 │
│      env GT_RIG=foo GT_POLECAT=bar ... \                    │
│        claude --prompt "…"                                  │
│                                                             │
│  Future SSH polecat:                                        │
│  exec env GT_RUN=abc ... \                                  │
│    ssh -t user@host \                                       │
│      env GT_RIG=foo GT_POLECAT=bar ... \                    │
│        claude --prompt "…"                                  │
└─────────────────────────────────────────────────────────────┘
```

The exec-wrapper plugin system (PR #2689) already provides this injection
point. The startup command is assembled as:

```
exec env <outer-env> ... <exec-wrapper-args> <agent-command>
```

Where `<exec-wrapper-args>` is an optional command prefix loaded from rig
settings or a plugin definition.

### How tmux operations work through the wrapper

Every tmux operation continues to work because `daytona exec --tty` forwards
stdin/stdout/stderr as a PTY. From tmux's perspective, the pane contains a
process that produces terminal output — it doesn't matter that the process is
`daytona exec` tunneling to a remote container rather than a local `claude`
binary.

| tmux operation | How it works through Daytona |
|---|---|
| `has-session` | Checks if the tmux session exists (it does — wrapping `daytona exec`) |
| `kill-session` | Kills the tmux session → SIGHUP to `daytona exec` → container process terminates |
| `set-environment` | Sets vars in tmux's env table (used for `gt` metadata, not passed to container) |
| `send-keys` | Sends keystrokes to the pane → forwarded through `daytona exec --tty` to container stdin |
| `capture-pane` | Captures pane content → shows whatever `daytona exec --tty` renders (agent output) |
| `list-sessions` | Lists all tmux sessions (local and remote polecats appear identically) |
| `attach-session` | Attaches terminal to pane → interactive access through the `daytona exec` tunnel |

**Health checking** works because the tmux pane's foreground process is
`daytona exec`. If the daytona tunnel dies (network failure, workspace stopped),
the pane process exits and tmux reports the session as dead — the same signal
as a local agent crash. The witness detects this identically.

**Prompt detection** (`IsAtPrompt`) reads tmux pane content looking for
sentinel markers. Since `daytona exec --tty` faithfully relays terminal
output, the same markers appear in the pane buffer regardless of where the
agent actually runs.

### Where the model breaks: inner env vars

There is one important distinction in the command structure. The exec-wrapper
as landed in PR #2689 is a simple prefix — it does not distinguish between
"outer" env vars (visible to the wrapper process) and "inner" env vars
(visible only inside the container):

```
# PR #2689 exec-wrapper (flat):
exec env OUTER=1 INNER=2 wrapper-cmd -- agent-cmd

# What Daytona needs (nested):
exec env OUTER=1 daytona exec ws --tty -- env INNER=2 agent-cmd
```

The Daytona branch's `buildDaytonaCommand()` handles this by constructing
a nested `env` invocation: identity/proxy/cert vars go inside the `daytona
exec ... --` boundary (they must be visible inside the container), while
session-scoped vars like `GT_RUN` go outside (visible to the tmux process
for `gt` metadata queries).

This means the exec-wrapper system needs to be extended to support **env var
partitioning** — some vars are outer (pre-wrapper), some are inner
(post-wrapper, pre-agent). See [Exec-Wrapper Extensions](#exec-wrapper-extensions)
below.

---

## Exec-Wrapper Extensions for Sandbox Lifecycle

PR #2689 introduced exec-wrapper as a static command prefix. For Daytona
integration, we need three extensions:

### Extension 1: Template Variables in Wrapper Args

The wrapper args must include the workspace name, which varies per polecat:

```toml
# Current (static):
wrapper = ["exitbox", "run", "--profile=gastown-polecat", "--"]

# Needed (templated):
wrapper = ["daytona", "exec", "{{workspace}}", "--tty", "--"]
```

**Template variables available at session start time:**

| Variable | Expansion | Source |
|----------|-----------|--------|
| `{{workspace}}` | `gt-<installID>-<rig>--<polecat>` | `daytona.Client.WorkspaceName()` |
| `{{rig}}` | Rig name | `SessionStartOptions.Rig` |
| `{{polecat}}` | Polecat name | `SessionStartOptions.Polecat` |
| `{{install_prefix}}` | `gt-<installID>` | `TownConfig.ShortInstallationID()` |
| `{{work_dir}}` | Container working directory | Rig config or default `/home/user/project` |

**Implementation**: Template expansion happens in `resolveExecWrapper()` at
the point where it's already loading from rig settings. The expansion context
is a `WrapperContext` struct passed from `SessionManager.Start()`:

```go
// WrapperContext provides values for template expansion in exec-wrapper args.
type WrapperContext struct {
    Rig            string
    Polecat        string
    InstallPrefix  string
    WorkDir        string
    WorkspaceName  string // pre-computed: <installPrefix>-<rig>--<polecat>
}

// ExpandWrapper replaces {{var}} placeholders in wrapper args.
func ExpandWrapper(wrapper []string, ctx WrapperContext) []string {
    replacer := strings.NewReplacer(
        "{{workspace}}", ctx.WorkspaceName,
        "{{rig}}", ctx.Rig,
        "{{polecat}}", ctx.Polecat,
        "{{install_prefix}}", ctx.InstallPrefix,
        "{{work_dir}}", ctx.WorkDir,
    )
    expanded := make([]string, len(wrapper))
    for i, arg := range wrapper {
        expanded[i] = replacer.Replace(arg)
    }
    return expanded
}
```

### Extension 2: Inner Environment Variables

The exec-wrapper must support env vars that are injected _after_ the wrapper
command, inside the remote execution context. These are distinct from the
outer env vars that tmux's `exec env ...` sets.

**Why this matters**: When `daytona exec ws --tty --` tunnels into a container,
the outer env vars are NOT inherited by the container process. The container
has its own environment, set at workspace creation time. Per-session vars
(identity, proxy certs, branch overrides) must be passed inline after the
`--` delimiter.

**Config structure:**

```json
{
  "runtime": {
    "exec_wrapper": ["daytona", "exec", "{{workspace}}", "--tty", "--"],
    "exec_wrapper_inner_env": {
      "GT_PROXY_URL": "https://{{proxy_addr}}",
      "GT_PROXY_CERT": "/etc/gt/certs/client.crt",
      "GT_PROXY_KEY": "/etc/gt/certs/client.key",
      "GT_PROXY_CA": "/etc/gt/certs/ca.crt",
      "GIT_SSL_CERT": "/etc/gt/certs/client.crt",
      "GIT_SSL_KEY": "/etc/gt/certs/client.key",
      "GIT_SSL_CAINFO": "/etc/gt/certs/ca.crt"
    }
  }
}
```

**Command assembly** in `BuildStartupCommand()`:

```go
// Current (PR #2689):
// exec env OUTER=val ... <wrapper> agent-cmd
cmd = "exec env " + outerEnvStr + " " + wrapperStr + " " + agentCmd

// Extended:
// exec env OUTER=val ... <wrapper> env INNER=val ... agent-cmd
cmd = "exec env " + outerEnvStr + " " + wrapperStr + " "
if len(innerEnv) > 0 {
    cmd += "env " + innerEnvStr + " "
}
cmd += agentCmd
```

The inner env also supports template expansion (e.g., `{{proxy_addr}}` resolves
from `RemoteBackend.ProxyAddr`).

**Which vars go where:**

| Variable | Location | Reason |
|----------|----------|--------|
| `GT_RUN` | Outer | Session-scoped telemetry ID; read by `gt` on the host |
| `GT_RIG` | Both | Needed by host `gt` commands AND container `bd`/`gt` |
| `GT_POLECAT` | Both | Same — used in both contexts |
| `GT_ROLE` | Inner | Only the agent inside the container reads this |
| `GT_PROXY_URL` | Inner | Container connects to proxy; host doesn't need this |
| `GT_PROXY_CERT/KEY/CA` | Inner | mTLS certs are inside the container filesystem |
| `GIT_SSL_*` | Inner | Git operations happen inside the container |
| `GIT_AUTHOR_*` | Inner | Commits happen inside the container |
| `GT_REPO_BRANCH` | Inner | Branch override for workspace reuse |
| `BD_DOLT_AUTO_COMMIT` | Inner | Beads config for the agent process |
| `GT_BRANCH` | Outer | Used by `gt done` on the host for nuked-worktree fallback |
| `GT_POLECAT_PATH` | Outer | Used by `gt done` on the host |

### Extension 3: Sandbox Lifecycle Hooks

The exec-wrapper is a command prefix — it's stateless. But Daytona workspaces
have lifecycle requirements:

1. **Before session start**: Workspace must exist and be running; mTLS cert
   must be issued and injected into the workspace's cert volume
2. **After session stop**: Cert must be revoked; workspace optionally stopped
   or archived

These hooks run at the `SessionManager` level, not inside the wrapper command:

```go
// SandboxLifecycle is implemented by exec-wrapper plugins that manage
// external sandbox state (workspace creation, cert management, cleanup).
// SessionManager calls these hooks around session create/destroy.
type SandboxLifecycle interface {
    // PreStart is called before the tmux session is created.
    // For Daytona: creates/starts workspace, issues cert, injects cert volume.
    // Returns inner env vars to inject after the wrapper's -- delimiter.
    PreStart(ctx context.Context, opts SandboxOpts) (innerEnv map[string]string, err error)

    // PostStop is called after the tmux session is killed.
    // For Daytona: revokes cert, optionally stops/deletes workspace.
    PostStop(ctx context.Context, opts SandboxOpts) error

    // Reconcile is called periodically by the DaytonaReconcilePatrol.
    // Discovers orphaned workspaces/beads and cleans up.
    Reconcile(ctx context.Context, opts ReconcileOpts) error
}

type SandboxOpts struct {
    Rig           string
    Polecat       string
    InstallPrefix string
    WorkspaceName string
    RigSettings   *config.RigSettings
    ProxyCA       *proxy.CA  // for cert issuance
}

type ReconcileOpts struct {
    Rig           string
    InstallPrefix string
    RigSettings   *config.RigSettings
    BeadsClient   *beads.Beads
}
```

**Integration with SessionManager:**

```go
func (m *SessionManager) Start(polecat string, opts SessionStartOptions) error {
    // ... existing validation, config resolution ...

    // If this rig has a sandbox lifecycle, run PreStart.
    var innerEnv map[string]string
    if m.sandbox != nil {
        sandboxOpts := SandboxOpts{
            Rig:           m.rig.Name,
            Polecat:       polecat,
            InstallPrefix: m.installPrefix,
            WorkspaceName: m.sandbox.WorkspaceName(m.rig.Name, polecat),
            RigSettings:   m.rigSettings,
            ProxyCA:       m.proxyCA,
        }
        var err error
        innerEnv, err = m.sandbox.PreStart(ctx, sandboxOpts)
        if err != nil {
            return fmt.Errorf("sandbox pre-start failed: %w", err)
        }
    }

    // Build command with exec-wrapper and inner env.
    command := buildCommand(runtimeConfig, beacon, wrapperCtx, innerEnv)

    // Create tmux session (same as today).
    if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
        return err
    }
    // ...
}

func (m *SessionManager) Stop(polecat string, force bool) error {
    // Kill tmux session (same as today).
    m.tmux.KillSessionWithProcesses(sessionID, force)

    // If this rig has a sandbox lifecycle, run PostStop.
    if m.sandbox != nil {
        sandboxOpts := SandboxOpts{...}
        if err := m.sandbox.PostStop(ctx, sandboxOpts); err != nil {
            slog.Warn("sandbox post-stop failed", "polecat", polecat, "err", err)
            // Non-fatal — workspace cleanup can be retried by reconciliation.
        }
    }
}
```

---

## DaytonaSandbox Implementation

The existing `internal/daytona/` code maps directly to the `SandboxLifecycle`
interface:

```go
// DaytonaSandbox implements SandboxLifecycle for Daytona remote execution.
type DaytonaSandbox struct {
    client     *daytona.Client
    proxyAdmin *proxy.AdminClient
}

func NewDaytonaSandbox(installPrefix string, proxyAdminAddr string) *DaytonaSandbox {
    return &DaytonaSandbox{
        client:     daytona.NewClient(installPrefix),
        proxyAdmin: proxy.NewAdminClient(proxyAdminAddr),
    }
}

func (d *DaytonaSandbox) WorkspaceName(rig, polecat string) string {
    return d.client.WorkspaceName(rig, polecat)
}
```

### PreStart

Maps to the workspace creation + cert issuance logic currently in
`SpawnPolecatForSling()` and `buildDaytonaCommand()`:

```go
func (d *DaytonaSandbox) PreStart(ctx context.Context, opts SandboxOpts) (map[string]string, error) {
    wsName := opts.WorkspaceName

    // 1. Ensure workspace exists and is running.
    //    Reuses existing workspace if available (idempotent create).
    if !d.client.WorkspaceExists(ctx, wsName) {
        createOpts := daytona.CreateOptions{
            Image:      opts.RigSettings.RemoteBackend.Image,
            Snapshot:   opts.RigSettings.RemoteBackend.Snapshot,
            Dockerfile: opts.RigSettings.RemoteBackend.Dockerfile,
            Profile:    opts.RigSettings.RemoteBackend.Profile,
            EnvVars: map[string]string{
                "GT_RIG":     opts.Rig,
                "GT_POLECAT": opts.Polecat,
                "GT_ROLE":    fmt.Sprintf("%s/polecats/%s", opts.Rig, opts.Polecat),
            },
            AutoStopInterval: opts.RigSettings.RemoteBackend.AutoStopInterval,
        }
        if err := d.client.Create(ctx, wsName, createOpts); err != nil {
            return nil, fmt.Errorf("creating workspace %s: %w", wsName, err)
        }
    }

    // Start the workspace (no-op if already running).
    if err := d.client.Start(ctx, wsName); err != nil {
        return nil, fmt.Errorf("starting workspace %s: %w", wsName, err)
    }

    // 2. Issue mTLS cert for this polecat's proxy access.
    certPEM, keyPEM, err := d.proxyAdmin.IssueCert(ctx, proxy.CertRequest{
        Identity: fmt.Sprintf("%s/polecats/%s", opts.Rig, opts.Polecat),
        Rig:      opts.Rig,
        Polecat:  opts.Polecat,
    })
    if err != nil {
        return nil, fmt.Errorf("issuing proxy cert: %w", err)
    }

    // 3. Inject cert into workspace via daytona exec.
    certDir := constants.DefaultRemoteCertDir
    if err := d.client.InjectCerts(ctx, wsName, certDir, certPEM, keyPEM, opts.ProxyCA.CACertPEM()); err != nil {
        return nil, fmt.Errorf("injecting certs into workspace: %w", err)
    }

    // 4. Return inner env vars for the agent process inside the container.
    proxyAddr := constants.DefaultProxyAddr
    if opts.RigSettings.RemoteBackend.ProxyAddr != "" {
        proxyAddr = opts.RigSettings.RemoteBackend.ProxyAddr
    }

    innerEnv := map[string]string{
        "GT_RIG":            opts.Rig,
        "GT_POLECAT":        opts.Polecat,
        "GT_ROLE":           fmt.Sprintf("%s/polecats/%s", opts.Rig, opts.Polecat),
        "GT_PROXY_URL":      "https://" + proxyAddr,
        "GT_PROXY_CERT":     certDir + "/client.crt",
        "GT_PROXY_KEY":      certDir + "/client.key",
        "GT_PROXY_CA":       certDir + "/ca.crt",
        "GIT_SSL_CERT":      certDir + "/client.crt",
        "GIT_SSL_KEY":       certDir + "/client.key",
        "GIT_SSL_CAINFO":    certDir + "/ca.crt",
        "GIT_AUTHOR_NAME":   opts.Polecat,
        "GIT_AUTHOR_EMAIL":  opts.Polecat + "@gastown.local",
        "GIT_COMMITTER_NAME":  opts.Polecat,
        "GIT_COMMITTER_EMAIL": opts.Polecat + "@gastown.local",
        "BD_DOLT_AUTO_COMMIT": "off",
    }
    if opts.Branch != "" {
        innerEnv["GT_REPO_BRANCH"] = opts.Branch
    }

    return innerEnv, nil
}
```

### PostStop

Maps to cert revocation + optional workspace stop currently in
`SessionManager.Stop()`:

```go
func (d *DaytonaSandbox) PostStop(ctx context.Context, opts SandboxOpts) error {
    // 1. Revoke cert BEFORE any bead state changes.
    //    This ordering is critical: revoking first prevents the (now-dead)
    //    polecat's cert from being used by a rogue process.
    identity := fmt.Sprintf("%s/polecats/%s", opts.Rig, opts.Polecat)
    if err := d.proxyAdmin.RevokeCert(ctx, identity); err != nil {
        slog.Warn("cert revocation failed", "identity", identity, "err", err)
        // Non-fatal — reconciliation will catch orphaned certs.
    }

    // 2. Optionally stop the workspace.
    if opts.RigSettings.RemoteBackend.AutoStop {
        wsName := opts.WorkspaceName
        if err := d.client.Stop(ctx, wsName); err != nil {
            slog.Warn("workspace stop failed", "workspace", wsName, "err", err)
        }
    }

    // 3. Optionally delete the workspace.
    if opts.RigSettings.RemoteBackend.AutoDelete {
        wsName := opts.WorkspaceName
        if err := d.client.Delete(ctx, wsName); err != nil {
            slog.Warn("workspace delete failed", "workspace", wsName, "err", err)
        }
    }

    return nil
}
```

### Reconcile

Maps directly to the existing `internal/daytona/reconcile.go`:

```go
func (d *DaytonaSandbox) Reconcile(ctx context.Context, opts ReconcileOpts) error {
    return daytona.Reconcile(ctx, daytona.ReconcileOptions{
        Client:        d.client,
        ProxyAdmin:    d.proxyAdmin,
        Rig:           opts.Rig,
        InstallPrefix: opts.InstallPrefix,
        BeadsClient:   opts.BeadsClient,
        PerOpTimeout:  30 * time.Second,
    })
}
```

---

## Interface 2: DispatchTarget

### Problem

`executeSling()` (internal/cmd/sling_dispatch.go) has three hardcoded dispatch
paths with different types:

```go
if rigName, isRig := IsRigName(target); isRig {
    spawnInfo, err := spawnPolecatForSling(rigName, ...)  // → SpawnedPolecatInfo
}
if dogName, isDog := IsDogTarget(target); isDog {
    dispatchInfo, err := DispatchToDog(dogName, ...)      // → DogDispatchInfo
}
agentID, pane, workDir, err := resolveTargetAgentFn(target)  // → (string, string, string)
```

Each path has different return types, different session start methods, and
different rollback logic. Adding a new dispatch type requires modifying
the dispatch switch in multiple places.

### Proposed interface

```go
// DispatchTarget represents a destination for slung work.
type DispatchTarget interface {
    // Identity
    AgentID() string        // "gastown/polecats/Toast", "deacon/dogs/alpha"
    TargetType() string     // "rig", "dog", "agent"
    WorkDir() string        // where bead files live

    // Lifecycle
    Prepare(ctx context.Context) error
    StartSession(ctx context.Context, opts StartOpts) (paneID string, err error)
    Rollback(ctx context.Context) error

    // State
    IsSessionRunning(ctx context.Context) (bool, error)
}

type StartOpts struct {
    FormulaEnv   map[string]string
    AgentCommand string
    AgentArgs    []string
}
```

Note: `StartSession` no longer takes an `ExecutionBackend` parameter. It always
uses tmux. The exec-wrapper (if any) is resolved from rig settings and baked
into the command string before `tmux.NewSessionWithCommand()` is called.

### Implementations

```go
// RigTarget spawns a polecat in a rig.
type RigTarget struct {
    rigName   string
    tmux      *tmux.Tmux
    spawnInfo *SpawnedPolecatInfo  // populated by Prepare()
}

func (r *RigTarget) Prepare(ctx context.Context) error {
    info, err := spawnPolecatForSling(r.rigName, r.spawnOpts)
    r.spawnInfo = info
    return err
}

func (r *RigTarget) StartSession(ctx context.Context, opts StartOpts) (string, error) {
    // SessionManager.Start handles:
    //   1. Sandbox PreStart (if configured)
    //   2. Exec-wrapper template expansion
    //   3. Inner env var injection
    //   4. tmux session creation
    return r.spawnInfo.StartSession()
}

func (r *RigTarget) Rollback(ctx context.Context) error {
    return nukePolecatDir(r.spawnInfo)
}
```

```go
// DogTarget dispatches to a dog plugin worker
type DogTarget struct { ... }

// ExistingAgentTarget slings to an already-running agent
type ExistingAgentTarget struct { ... }
```

### Resolver

```go
// ResolveTarget creates the appropriate DispatchTarget from a target string.
func ResolveTarget(target string, t *tmux.Tmux) (DispatchTarget, error) {
    if rigName, ok := IsRigName(target); ok {
        return NewRigTarget(rigName, t), nil
    }
    if dogName, ok := IsDogTarget(target); ok {
        return NewDogTarget(dogName, t), nil
    }
    return NewExistingAgentTarget(target, t)
}
```

### Unified dispatch flow

```go
func executeSling(ctx context.Context, target DispatchTarget, bead string) error {
    if err := target.Prepare(ctx); err != nil {
        return err
    }
    defer func() {
        if err != nil { target.Rollback(ctx) }
    }()

    cookFormula(bead)
    hookBead(bead, target.AgentID())

    _, err = target.StartSession(ctx, StartOpts{...})
    return err
}
```

---

## Interface 3: PatrolRegistry

### Problem

The daemon (internal/daemon/daemon.go, 2776 lines) has hardcoded patrol
checks using `IsPatrolEnabled()` — a 50+ line function with string matching:

```go
if IsPatrolEnabled(config, "dolt_remotes") { ... }
if IsPatrolEnabled(config, "dolt_backup") { ... }
if IsPatrolEnabled(config, "daytona_reconcile") { ... }
// ... 10+ more
```

Each patrol has its own config struct field in `PatrolsConfig`, its own
interval function, and its own execution block in the heartbeat loop.
Adding a new patrol requires touching 3+ locations.

### Proposed interface

```go
// PatrolHandler is implemented by each patrol type.
type PatrolHandler interface {
    Name() string
    Run(ctx context.Context, env PatrolEnv) error
    DefaultInterval() time.Duration
    RequiresRig() bool  // true = called once per rig; false = once per town
}

// PatrolEnv provides context to patrol handlers.
type PatrolEnv struct {
    TownRoot string
    RigName  string          // non-empty only when RequiresRig() == true
    Sandbox  SandboxLifecycle // nil for rigs without remote backend
    Logger   *slog.Logger
}

// PatrolRegistry manages patrol registration and execution.
type PatrolRegistry struct {
    patrols map[string]registeredPatrol
}

type registeredPatrol struct {
    handler  PatrolHandler
    enabled  bool
    interval time.Duration
    rigs     []string  // optional rig filter
}

func (r *PatrolRegistry) Register(handler PatrolHandler, config *PatrolConfig) {
    r.patrols[handler.Name()] = registeredPatrol{
        handler:  handler,
        enabled:  config.Enabled,
        interval: config.IntervalOr(handler.DefaultInterval()),
    }
}

func (r *PatrolRegistry) RunEnabled(ctx context.Context, env PatrolEnv) {
    for _, p := range r.patrols {
        if !p.enabled { continue }
        p.handler.Run(ctx, env)
    }
}
```

Note: `PatrolEnv` now carries `SandboxLifecycle` instead of `ExecutionBackend`.
The `DaytonaReconcilePatrol` calls `env.Sandbox.Reconcile()` — the reconciliation
logic stays in `internal/daytona/reconcile.go` but is invoked through the
interface.

### Built-in patrol registrations

```go
func DefaultRegistry() *PatrolRegistry {
    r := &PatrolRegistry{}
    r.Register(&WitnessPatrol{}, &PatrolConfig{Enabled: true})
    r.Register(&RefineryPatrol{}, &PatrolConfig{Enabled: true})
    r.Register(&DeaconPatrol{}, &PatrolConfig{Enabled: true})
    r.Register(&DoltRemotesPatrol{}, &PatrolConfig{Enabled: false})
    r.Register(&DoltBackupPatrol{}, &PatrolConfig{Enabled: false})
    r.Register(&SandboxReconcilePatrol{}, &PatrolConfig{Enabled: false})
    // ... etc
    return r
}
```

The daemon heartbeat loop becomes:

```go
func (d *Daemon) heartbeat(ctx context.Context) {
    env := PatrolEnv{TownRoot: d.townRoot, Logger: d.logger}
    d.registry.RunEnabled(ctx, env)
}
```

---

## SessionManager Changes

The key structural change to `SessionManager` is replacing the Daytona-specific
fields with a single optional `SandboxLifecycle` interface:

```go
// Before (feat/daytona-polecats)
type SessionManager struct {
    tmux          *tmux.Tmux
    rig           *rig.Rig
    proxyAdmin    *proxy.AdminClient
    beads         *beads.Beads
    daytonaClient *daytona.Client
    rigSettings   *config.RigSettings
}

// After
type SessionManager struct {
    tmux     *tmux.Tmux            // always present — the universal session backend
    rig      *rig.Rig
    sandbox  SandboxLifecycle      // nil for local-only rigs
    settings *config.RigSettings   // for exec-wrapper resolution
}
```

### Construction

```go
func NewSessionManager(t *tmux.Tmux, r *rig.Rig) *SessionManager {
    sm := &SessionManager{tmux: t, rig: r}

    // Load rig settings for exec-wrapper and remote backend config.
    settingsPath := filepath.Join(r.Path, "settings", "config.json")
    settings, err := config.LoadRigSettings(settingsPath)
    if err == nil && settings != nil {
        sm.settings = settings

        // If remote backend is configured, construct the sandbox lifecycle.
        if settings.RemoteBackend != nil && settings.RemoteBackend.Provider == "daytona" {
            townRoot := filepath.Dir(filepath.Dir(r.Path))
            townConfig, _ := config.LoadTownConfig(filepath.Join(townRoot, "mayor", "town.json"))
            prefix := townConfig.ShortInstallationID()
            adminAddr := constants.DefaultProxyAdminAddr
            if settings.RemoteBackend.ProxyAdminAddr != "" {
                adminAddr = settings.RemoteBackend.ProxyAdminAddr
            }
            sm.sandbox = NewDaytonaSandbox(prefix, adminAddr)
        }
    }

    return sm
}
```

### isRemoteMode() replacement

The `isRemoteMode()` check becomes `m.sandbox != nil`:

```go
// Before
func (m *SessionManager) isRemoteMode() bool {
    return m.daytonaClient != nil && m.rigSettings != nil && m.rigSettings.RemoteBackend != nil
}

// After — no method needed, just check the field
if m.sandbox != nil {
    // remote sandbox path
}
```

### Start flow (unified)

```go
func (m *SessionManager) Start(polecat string, opts SessionStartOptions) error {
    // ... validation, config resolution (unchanged) ...

    // 1. Run sandbox PreStart if configured.
    var innerEnv map[string]string
    if m.sandbox != nil {
        sandboxOpts := SandboxOpts{
            Rig:           m.rig.Name,
            Polecat:       polecat,
            WorkspaceName: m.sandbox.WorkspaceName(m.rig.Name, polecat),
            RigSettings:   m.settings,
            ProxyCA:       m.proxyCA,
            Branch:        opts.Branch,
        }
        var err error
        innerEnv, err = m.sandbox.PreStart(ctx, sandboxOpts)
        if err != nil {
            return fmt.Errorf("sandbox pre-start: %w", err)
        }
    }

    // 2. Resolve exec-wrapper from rig settings (PR #2689 path).
    wrapper := resolveExecWrapper(m.rig.Path)
    if len(wrapper) > 0 && m.sandbox != nil {
        // Expand template variables in wrapper args.
        wrapper = ExpandWrapper(wrapper, WrapperContext{
            Rig:           m.rig.Name,
            Polecat:       polecat,
            InstallPrefix: m.installPrefix,
            WorkspaceName: m.sandbox.WorkspaceName(m.rig.Name, polecat),
        })
    }

    // 3. Build startup command.
    //    BuildStartupCommand already handles exec-wrapper insertion (PR #2689).
    //    We extend it to also insert inner env vars after the wrapper.
    command := config.BuildStartupCommand(envVars, m.rig.Path, beacon)

    // If inner env vars exist, inject them between wrapper and agent command.
    if len(innerEnv) > 0 {
        command = injectInnerEnv(command, innerEnv)
    }

    // 4. Determine working directory.
    //    Remote polecats have no local worktree — use the marker directory.
    workDir := opts.WorkDir
    if workDir == "" {
        if m.sandbox != nil {
            workDir = m.polecatDir(polecat)  // marker dir for tmux cwd
        } else {
            workDir = m.clonePath(polecat)   // local git worktree
        }
    }

    // 5. Create tmux session (same API for local and remote).
    if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
        if m.sandbox != nil {
            // Rollback: stop workspace if session creation failed.
            m.sandbox.PostStop(ctx, SandboxOpts{...})
        }
        return err
    }

    // 6. Set tmux environment variables (metadata for gt commands on the host).
    m.tmux.SetEnvironment(sessionID, "GT_RIG", m.rig.Name)
    m.tmux.SetEnvironment(sessionID, "GT_POLECAT", polecat)
    m.tmux.SetEnvironment(sessionID, "GT_RUN", runID)
    // ...

    return nil
}
```

### Stop flow (unified)

```go
func (m *SessionManager) Stop(polecat string, force bool) error {
    sessionID := m.sessionID(polecat)

    // 1. Kill tmux session (always tmux, even for remote polecats).
    if err := m.tmux.KillSessionWithProcesses(sessionID, force); err != nil {
        return err
    }

    // 2. Run sandbox PostStop if configured.
    if m.sandbox != nil {
        sandboxOpts := SandboxOpts{
            Rig:           m.rig.Name,
            Polecat:       polecat,
            WorkspaceName: m.sandbox.WorkspaceName(m.rig.Name, polecat),
            RigSettings:   m.settings,
        }
        if err := m.sandbox.PostStop(ctx, sandboxOpts); err != nil {
            slog.Warn("sandbox post-stop failed", "polecat", polecat, "err", err)
        }
    }

    return nil
}
```

---

## Rig Configuration

### Local rig (no changes)

```json
{
  "type": "rig-settings",
  "version": 1,
  "agent": "claude"
}
```

No exec-wrapper, no sandbox lifecycle. Polecats run locally in git worktrees
inside tmux sessions.

### Remote rig (Daytona via exec-wrapper)

```json
{
  "type": "rig-settings",
  "version": 1,
  "agent": "claude",
  "runtime": {
    "exec_wrapper": ["daytona", "exec", "{{workspace}}", "--tty", "--"]
  },
  "remote_backend": {
    "provider": "daytona",
    "image": "ghcr.io/anthropics/gas-town-polecat:latest",
    "snapshot": "gt-polecat-v0.12.0",
    "proxy_addr": "proxy.example.com:8443",
    "auto_stop": true,
    "auto_stop_interval": 30,
    "network_block_all": true
  }
}
```

The `remote_backend` triggers construction of `DaytonaSandbox`.
The `runtime.exec_wrapper` provides the command prefix for tmux pane commands.
Both are needed: the sandbox handles lifecycle, the wrapper handles command injection.

### Mixed-mode town

```
town/
├── rig-a/  →  local (no remote_backend)
│   └── settings/config.json: { "agent": "claude" }
├── rig-b/  →  daytona remote
│   └── settings/config.json: { "agent": "claude", "remote_backend": { "provider": "daytona", ... } }
└── rig-c/  →  local with exec-wrapper (sandboxed but not remote)
    └── settings/config.json: { "runtime": { "exec_wrapper": ["exitbox", "run", "--profile=gastown", "--"] } }
```

This demonstrates the three modes:
1. **Bare local** (rig-a): tmux → agent
2. **Remote sandbox** (rig-b): tmux → daytona exec → agent (with lifecycle hooks)
3. **Local sandbox** (rig-c): tmux → exitbox → agent (wrapper only, no lifecycle)

---

## Migration Plan

### Phase 1: Exec-Wrapper Extensions

Extend PR #2689's exec-wrapper with template variables and inner env support.

1. Add `WrapperContext` struct and `ExpandWrapper()` to `internal/config/`
2. Add `exec_wrapper_inner_env` to `RuntimeConfig`
3. Extend `BuildStartupCommand()` to inject inner env after wrapper args
4. Add `injectInnerEnv()` helper that inserts `env K=V ...` between the wrapper
   `--` delimiter and the agent command
5. **Tests**: Verify command assembly produces correct nested env structure

**Scope**: ~5 files. The exec-wrapper insertion point already exists; this
adds template expansion and a second env injection point.

**Key files:**
- `internal/config/loader.go` — `BuildStartupCommand`, `resolveExecWrapper`
- `internal/config/types.go` — `RuntimeConfig.ExecWrapperInnerEnv`
- `internal/config/wrapper.go` (new) — `WrapperContext`, `ExpandWrapper`
- `internal/config/loader_test.go` — command assembly tests

### Phase 2: SandboxLifecycle Interface

Define the lifecycle interface and implement DaytonaSandbox.

1. Define `SandboxLifecycle` interface in `internal/sandbox/`
2. Implement `DaytonaSandbox` wrapping existing `internal/daytona/` code
3. Update `SessionManager` to hold optional `SandboxLifecycle`
4. Wire `PreStart` into `SessionManager.Start()` before tmux session creation
5. Wire `PostStop` into `SessionManager.Stop()` after tmux session kill
6. Remove `daytonaClient`, `rigSettings`, `proxyAdmin`, `beads` fields from
   `SessionManager` (replaced by `sandbox SandboxLifecycle`)
7. Remove `isRemoteMode()` method
8. Remove `buildDaytonaCommand()` (replaced by exec-wrapper + inner env)
9. Remove `SetDaytona()` method (replaced by construction-time injection)

**Scope**: ~12 files. The Daytona logic already exists; this restructures
the integration points.

**Key files:**
- `internal/sandbox/lifecycle.go` (new) — `SandboxLifecycle` interface
- `internal/sandbox/daytona.go` (new) — `DaytonaSandbox` implementation
- `internal/polecat/session_manager.go` — structural changes
- `internal/cmd/polecat_spawn.go` — sandbox construction at spawn time

### Phase 3: DispatchTarget Interface

Clean up the sling dispatch system.

1. Define `DispatchTarget` interface in `internal/dispatch/`
2. Implement `RigTarget`, `DogTarget`, `ExistingAgentTarget`
3. Create `ResolveTarget()` function
4. Refactor `executeSling()` to use `DispatchTarget`
5. Move spawn/dispatch logic into target implementations

**Scope**: ~10 files. The dispatch logic already exists; this restructures
it behind an interface.

### Phase 4: PatrolRegistry

Clean up the daemon patrol system.

1. Define `PatrolHandler` interface and `PatrolRegistry` in `internal/patrol/`
2. Extract each patrol's logic into a handler implementation
3. Replace `IsPatrolEnabled()` with registry lookups
4. Refactor daemon heartbeat to iterate registry
5. `SandboxReconcilePatrol` calls `env.Sandbox.Reconcile()`

**Scope**: ~8 files. The patrol logic stays the same; the registry replaces
the switch/case dispatch.

### Phase 5: Cleanup

1. Remove `isRemoteMode()` checks from all packages
2. Remove Daytona-specific fields from SessionManager struct
3. Audit all direct `m.tmux` references that should go through the sandbox
4. Integration tests: local polecat, exitbox-wrapped polecat, Daytona polecat
5. Remove the Daytona-specific session start/stop paths in favour of the
   unified sandbox lifecycle flow

---

## What Stays, What Changes

### Preserved from feat/daytona-polecats

| Component | Status |
|-----------|--------|
| `internal/daytona/client.go` | Preserved, wrapped by DaytonaSandbox |
| `internal/daytona/reconcile.go` | Preserved, called by DaytonaSandbox.Reconcile() |
| `internal/daytona/retry.go` | Preserved, used by DaytonaSandbox |
| `internal/proxy/` (all) | Preserved, proxy is independent of session management |
| `Dockerfile.daytona` | Preserved as polecat container image |
| `docs/daytona-backend.md` | Updated to reference new architecture |
| All test files | Preserved, tests are self-contained |

### Changed

| Component | Change |
|-----------|--------|
| `SessionManager` | `daytonaClient`/`rigSettings`/`proxyAdmin` → `sandbox SandboxLifecycle` |
| `SessionManager.Start()` | Daytona if/else → unified flow with optional sandbox.PreStart |
| `SessionManager.Stop()` | Daytona cert revocation → sandbox.PostStop |
| `buildDaytonaCommand()` | Removed — replaced by exec-wrapper + inner env |
| `isRemoteMode()` | Removed — replaced by `sandbox != nil` |
| `SetDaytona()` | Removed — sandbox injected at construction |
| `executeSling()` | Three-path switch → `DispatchTarget.StartSession()` |
| `daemon.go` heartbeat | Hardcoded patrols → `PatrolRegistry.RunEnabled()` |
| `IsPatrolEnabled()` | Removed, replaced by registry |
| `BuildStartupCommand()` | Extended with inner env injection after wrapper |
| `resolveExecWrapper()` | Extended with template variable expansion |

### New

| Component | Purpose |
|-----------|---------|
| `internal/sandbox/` | SandboxLifecycle interface + DaytonaSandbox impl |
| `internal/config/wrapper.go` | WrapperContext, ExpandWrapper, template expansion |
| `internal/dispatch/` | DispatchTarget interface + implementations |
| `internal/patrol/` | PatrolRegistry + PatrolHandler interface |

### Removed (from feat/daytona-polecats)

| Component | Reason |
|-----------|--------|
| `SessionManager.buildDaytonaCommand()` | Replaced by exec-wrapper + inner env |
| `SessionManager.isRemoteMode()` | Replaced by `sandbox != nil` check |
| `SessionManager.SetDaytona()` | Construction-time injection instead |
| `SessionManager.daytonaClient` field | Moved into DaytonaSandbox |
| `SessionManager.rigSettings` field | Moved into DaytonaSandbox |
| `SessionManager.proxyAdmin` field | Moved into DaytonaSandbox |

---

## Resolved Design Questions

The original design had 5 open questions. With the tmux-stays architecture,
most are resolved:

1. **Pane concept**: ~~Should the interface expose pane IDs?~~ **Resolved.**
   tmux is always the pane provider. Pane IDs are tmux pane IDs. No abstraction
   needed.

2. **User attachment**: ~~Separate InteractiveBackend?~~ **Resolved.** `tmux
   attach-session` works for all backends. The `daytona exec --tty` tunnel
   provides full PTY forwarding, so the user sees the remote agent's terminal
   natively.

3. **Prompt detection**: ~~Remote backends need alternative readiness signal.~~
   **Resolved.** `IsAtPrompt()` reads tmux pane content, which shows the remote
   agent's output via `daytona exec --tty`. Same sentinel markers work. If the
   daytona tunnel fails before the agent reaches the prompt, the pane process
   exits and tmux reports it as dead — the standard crash detection path handles
   this.

4. **Volume management**: ~~Should ExecutionBackend expose volume operations?~~
   **Resolved.** Volume management (cert injection) is handled by
   `DaytonaSandbox.PreStart()` via `daytona exec` — it's a sandbox lifecycle
   concern, not a session management concern.

5. **Patrol interval ownership**: Unchanged — handler provides default, config
   overrides.

### New question: exec-wrapper vs remote_backend coupling

Should `remote_backend` automatically imply an exec-wrapper, or must both be
configured explicitly?

**Recommendation**: Explicit. The exec-wrapper is a general-purpose mechanism
(also used for local sandboxes like exitbox). The remote_backend triggers
sandbox lifecycle hooks. A rig could theoretically have a remote_backend with
no exec-wrapper (if the agent binary is pre-installed in the container and
accessible via PATH), though this is unlikely in practice. Keeping them separate
preserves the layering:

- Exec-wrapper = "how to invoke the agent process" (command wrapping)
- Remote backend = "where the sandbox lives and how to manage it" (lifecycle)

The configuration examples above show both fields set. A validation warning
should fire if `remote_backend` is set but `exec_wrapper` is empty, since
this almost certainly indicates a misconfiguration.
