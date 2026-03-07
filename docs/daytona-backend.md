# Daytona Remote Backend

Gas Town can run polecats in [Daytona](https://www.daytona.io/) cloud workspaces
instead of local git worktrees. Each polecat gets an isolated container with its
own filesystem, network, and process tree. Git traffic routes through a host-side
mTLS proxy — containers never contact GitHub directly.

This guide covers setup, configuration, and operations.

**This requires a Tier 3+ Daytona account for egress to be available**

## Prerequisites

| Requirement | Notes |
|---|---|
| **`daytona` CLI** | >= 0.149.0, installed and authenticated (`daytona login`). See [installation docs](https://www.daytona.io/docs/installation/installation/). |
| **Gas Town installation** | A working `gt install` with at least one rig. |
| **`gt-proxy-server`** | Running on the host. Manages mTLS certs, proxies git, and relays `gt`/`bd` commands. See [proxy-server.md](proxy-server.md). |
| **Container image** | Must include `claude-code` (or your agent), `git`, and `gt-proxy-client` installed as both `/usr/local/bin/gt` and `/usr/local/bin/bd`. |

The proxy server generates a self-signed CA on first start at `~/.gt/.runtime/ca/`.
Polecat client certs are issued from this CA and injected into containers at spawn time.

Run `gt doctor` to verify all prerequisites including the Daytona CLI version
(see [Health Checks](#health-checks)).

## Quick Start

1. Start the proxy server:

   ```bash
   gt-proxy-server --town-root ~/gt
   ```

2. Configure the rig:

   ```bash
   # Edit <rig>/settings/config.json
   cat myrig/settings/config.json
   ```

   ```json
   {
     "remote_backend": {
       "provider": "daytona",
       "image": "docker.io/your-org/polecat-image:v1",
       "auto_stop": true,
       "auto_delete": false,
       "proxy_addr": "172.17.0.1:9876",
       "proxy_admin_addr": "127.0.0.1:9877"
     }
   }
   ```

3. Sling a bead:

   ```bash
   gt sling my-bead myrig
   ```

   Daytona mode is auto-detected from the rig config. No extra flags needed.

## Configuration Reference

The `remote_backend` block lives in `<rig>/settings/config.json` under the
`RigSettings` object. When present and non-null, all polecats for that rig
spawn as Daytona workspaces instead of local worktrees.

### Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `provider` | string | **yes** | — | Must be `"daytona"`. Only supported provider. |
| `image` | string | no | Daytona default | Container image for workspaces. Mutually exclusive with `snapshot`. latest tag not allowed. First sling will create a snapshot and update the config. ghcr is not supported on Daytona. |
| `dockerfile` | string | no | — | Path to Dockerfile for sandbox snapshot (passed as `--dockerfile`). Replaces the deprecated `--devcontainer-path`. |
| `snapshot` | string | no | — | Pre-built snapshot ID for warm-start creation (passed as `--snapshot`). Mutually exclusive with `image`. |
| `target` | string | no | — | Target region for workspace placement (passed as `--target`). E.g. `"us"`, `"eu"`. |
| `class` | string | no | — | Resource tier: `"small"`, `"medium"`, `"large"` (passed as `--class`). |
| `cpu` | int | no | — | CPU cores (passed as `--cpu`). Overrides `class` CPU allocation. |
| `memory` | int | no | — | Memory in MB (passed as `--memory`). Overrides `class` memory allocation. |
| `disk` | int | no | — | Disk size in GB (passed as `--disk`). Overrides `class` disk allocation. |
| `auto_stop` | bool | no | `false` | Stop the workspace when a polecat session ends. Preserves state for restart. |
| `auto_delete` | bool | no | `false` | Delete the workspace when the polecat is removed. Permanent. |
| `auto_stop_interval` | int | no | — | Idle minutes before auto-stop (passed as `--auto-stop-interval`). |
| `auto_archive_interval` | int | no | — | Minutes after stop before auto-archive (passed as `--auto-archive-interval`). |
| `auto_delete_interval` | int | no | — | Minutes after archive before auto-delete (passed as `--auto-delete-interval`). |
| `network_block_all` | bool | no | `false` | Block all outbound network access (passed as `--network-block-all`). |
| `network_allow_list` | string | no | — | Comma-separated CIDRs to allow when `network_block_all` is true (passed as `--network-allow-list`). |
| `sandboxed_network` | bool | no | `false` | Enable network isolation. Implies `network_block_all` and auto-adds `proxy_addr` and `allowed_ips` to the allow list. |
| `allowed_ips` | []string | no | `[]` | IPs or CIDRs that sandboxes may reach. Only used when `sandboxed_network` is true. Bare IPs get `/32` appended automatically. |
| `proxy_addr` | string | no | `localhost:8443` | Host:port of the mTLS proxy server (as reachable from containers). |
| `proxy_admin_addr` | string | no | `127.0.0.1:9877` | Host:port of the proxy admin API (localhost only, no TLS). |
| `env` | object | no | `{}` | Extra environment variables injected into every container for this rig. |

### Example: Minimal

```json
{
  "remote_backend": {
    "provider": "daytona"
  }
}
```

Uses Daytona defaults for everything. Proxy must be reachable at `localhost:8443`.

### Example: Full

```json
{
  "remote_backend": {
    "provider": "daytona",
    "image": "ghcr.io/your-org/gt-polecat:v2",
    "dockerfile": ".devcontainer/Dockerfile",
    "target": "us",
    "class": "medium",
    "cpu": 4,
    "memory": 8192,
    "disk": 100,
    "auto_stop": true,
    "auto_delete": false,
    "auto_stop_interval": 60,
    "auto_archive_interval": 1440,
    "auto_delete_interval": 10080,
    "network_block_all": false,
    "network_allow_list": "10.0.0.0/8,192.168.0.0/16",
    "sandboxed_network": false,
    "allowed_ips": [],
    "proxy_addr": "172.17.0.1:9876",
    "proxy_admin_addr": "127.0.0.1:9877",
    "env": {
      "ANTHROPIC_API_KEY": "sk-ant-...",
      "NODE_OPTIONS": "--max-old-space-size=4096"
    }
  }
}
```

### Example: Snapshot-Based (Warm Start)

```json
{
  "remote_backend": {
    "provider": "daytona",
    "snapshot": "snap-abc123",
    "target": "us",
    "auto_stop_interval": 30,
    "proxy_addr": "172.17.0.1:9876",
    "proxy_admin_addr": "127.0.0.1:9877"
  }
}
```

Snapshot-based creation skips image pull, cutting cold-start from minutes to
seconds. The `snapshot` field is mutually exclusive with `image`.

### Snapshot Lifecycle

When `image` is configured, the first sling creates a Daytona snapshot from the
image and caches the snapshot name in `settings/config.json`. Subsequent slings
reuse the cached snapshot for faster creation.

`EnsureSnapshot` handles the full async lifecycle:

- **Active snapshot** — reused immediately.
- **Transitional states** (pulling, creating) — polled every 5 seconds until
  active, with a 10-minute timeout.
- **Errored snapshot** — automatically deleted and re-created (up to 2 retries).
- **Cached but missing** — if the cached snapshot name in `settings/config.json`
  no longer exists in Daytona (e.g., deleted externally), it is cleared and
  re-created from the image.

> **Note:** `daytona snapshot create` returns immediately while the image pull
> runs asynchronously. Large images (multi-GB) can take several minutes to pull.
> The polling loop waits for the snapshot to reach `active` state before
> proceeding.

### Per-Rig Granularity

Configuration is per-rig. Some rigs can use Daytona while others use local
worktrees. This is determined by whether `remote_backend` is present in each
rig's `settings/config.json`.

## How It Works

### Workspace Naming

Each workspace is named:

```
gt-<installID>-<rig>--<polecat>
```

- `<installID>` — first 12 characters of the town's `installation_id` (UUID v4,
  auto-generated on `gt install`)
- `<rig>` — rig name
- `<polecat>` — polecat name
- `--` — double-hyphen delimiter (allows single hyphens in rig/polecat names)

Example: `gt-a1b2c3d4e5f6-myrig--Toast`

The install prefix scopes all operations to this Gas Town installation, so
multiple developers sharing the same Daytona provider never see each other's
workspaces.

### Spawn Flow

When `gt sling <bead> <rig>` runs with a Daytona-configured rig:

1. **Preflight checks** — verifies `daytona` CLI is on PATH (>= 0.149.0), proxy
   server is reachable (pings admin API), and CA cert/key exist.
2. **Workspace creation** — calls `daytona create` with the configured flags.
   A shared cert volume (`gt-certs-<installPrefix>`) is mounted at
   `/run/gt-proxy/` so certs persist across workspace restarts.

   Create flags emitted based on config:

   | Config field | CLI flag |
   |---|---|
   | `image` | `--image <value>` |
   | `dockerfile` | `--dockerfile <value>` |
   | `snapshot` | `--snapshot <value>` |
   | `target` | `--target <value>` |
   | `class` | `--class <value>` |
   | `cpu` | `--cpu <value>` |
   | `memory` | `--memory <value>` |
   | `disk` | `--disk <value>` |
   | `auto_stop_interval` | `--auto-stop-interval <value>` |
   | `auto_archive_interval` | `--auto-archive-interval <value>` |
   | `auto_delete_interval` | `--auto-delete-interval <value>` |
   | `network_block_all` | `--network-block-all` |
   | `network_allow_list` | `--network-allow-list <value>` |
   | `sandboxed_network` | `--network-block-all` + computed `--network-allow-list` |
   | volumes | `--volume <name:/path>` (repeated) |
   | labels | `--label <key=value>` (repeated) |

3. **Certificate injection** — issues an mTLS client cert from the proxy CA,
   then injects `client.crt`, `client.key`, and `ca.crt` into the container at
   `/run/gt-proxy/` via `daytona exec`. The cert volume ensures these files
   survive workspace stop/start cycles without re-injection.
4. **Session start** — launches `daytona exec <wsName> -- env K=V ... sh -c
   '<agent command>'` inside a local tmux pane. The local `daytona exec`
   process is the liveness signal.

### Removed and Changed Flags

The following `daytona create` flags have been removed or replaced compared to
earlier versions:

| Old flag | Status | Notes |
|---|---|---|
| `--devcontainer-path` | Replaced | Use `--dockerfile` instead |
| `--yes` | Removed | No longer needed in v0.149+ (commands are non-interactive by default) |

> **Note:** `Create()` still accepts `repoURL` (positional) and `--branch`
> (named flag) for backward compatibility with self-hosted Daytona deployments
> that support git-based workspace creation. The Daytona cloud CLI may not
> support these — verify against your target CLI version.

### Workspace Labels

Workspaces are tagged at creation with `--label` flags for identification and
filtering:

| Label | Value | Purpose |
|---|---|---|
| `gt-install-id` | Installation ID prefix | Scopes to this Gas Town installation |
| `gt-rig` | Rig name | Identifies owning rig |

Additional custom labels can be passed through the `Labels` field in
`CreateOptions`.

### Git Access

Containers use the proxy's git endpoint instead of GitHub:

```
https://<proxy_addr>/v1/git/<rig>
```

The proxy serves the rig's `.repo.git` bare repository over git smart-HTTP.
Branch-scoped push authorization is enforced: a polecat cert with CN
`gt-<rig>-<name>` may only push refs under `polecat/<name>-*`.

Git environment variables are injected via inline env prefix in `daytona exec`:

| Variable | Value | Purpose |
|---|---|---|
| `GIT_SSL_CERT` | `/run/gt-proxy/client.crt` | Client certificate for mTLS |
| `GIT_SSL_KEY` | `/run/gt-proxy/client.key` | Client private key |
| `GIT_SSL_CAINFO` | `/run/gt-proxy/ca.crt` | CA cert to verify proxy server |

### Command Execution (`daytona exec`)

The `daytona exec` command has two calling patterns:

**Standard exec** — runs a command with environment variables passed as an
inline `env K=V` prefix (not `--env` flags, which `daytona exec` does not
support):

```bash
daytona exec <workspace> -- env KEY1=VAL1 KEY2=VAL2 command args...
```

**Exec with working directory** — the `--cwd` flag sets the working directory
inside the container:

```bash
daytona exec <workspace> --cwd /home/daytona/project -- env KEY=VAL command args...
```

The Go client exposes this via `ExecWithOptions`:

```go
type ExecOptions struct {
    Env map[string]string  // inline env prefix (env K=V)
    Cwd string             // --cwd for working directory
}

func (c *Client) ExecWithOptions(ctx context.Context, name string, opts ExecOptions, cmd ...string) (string, string, int, error)
```

The original `Exec()` method remains for backward compatibility and delegates
to `ExecWithOptions`.

> **Known issue:** `daytona exec` splits all command arguments on whitespace
> (spaces), which breaks `sh -c` scripts that contain spaces. As a workaround,
> Gas Town uses tab characters (`\t`) instead of spaces in shell scripts passed
> to `daytona exec`. This only affects internal cert injection; operators do not
> need to account for this.

### Command Proxy

The container's `gt` and `bd` binaries are actually `gt-proxy-client` — a thin
shim that forwards commands to the host proxy via `POST /v1/exec`. The proxy
authenticates the request via the polecat's mTLS cert and runs the real binary
on the host.

Allowed commands:

| Binary | Subcommands |
|---|---|
| `gt` | `prime`, `hook`, `done`, `mail`, `nudge`, `mol`, `status`, `handoff`, `version`, `convoy`, `sling` |
| `bd` | `create`, `update`, `close`, `show`, `list`, `ready`, `dep`, `export`, `prime`, `stats`, `blocked`, `doctor` |

### Session Lifecycle

| Event | Action |
|---|---|
| **Spawn** | Create workspace, inject certs, start `daytona exec` session |
| **Session end** | Kill tmux pane; if `auto_stop: true`, stop workspace |
| **Idle (auto-stop)** | Workspace stops after `auto_stop_interval` minutes of inactivity |
| **Idle (auto-archive)** | Stopped workspace archived after `auto_archive_interval` minutes |
| **Idle (auto-delete)** | Archived workspace deleted after `auto_delete_interval` minutes |
| **Polecat removal** | Archive workspace, revoke mTLS cert via admin API; if `auto_delete: true`, delete workspace |
| **Warm idle** | Workspace stays running as a warm slot for the next sling (persistent polecat model) |

### Archive Lifecycle

The `Archive()` method moves stopped workspaces to object storage at reduced
cost:

```bash
daytona archive <workspace>
```

Archive is called:
- During reconciliation on orphaned or already-stopped workspaces
- On polecat removal, before deletion
- Archive failures are best-effort (non-fatal warnings)

The lifecycle progression is: **Running** → **Stopped** → **Archived** → **Deleted**.

### Volume-Based Certificate Storage

Certificates are stored on a shared Daytona volume rather than injected into
the container's ephemeral filesystem. The volume is named
`gt-certs-<installPrefix>` and mounted at `/run/gt-proxy/` inside the container.

Benefits:
- Certs persist across workspace stop/start cycles without re-injection
- Multiple `daytona exec` sessions share the same cert files
- Reduces spawn latency on restart (skip cert injection step)

The volume is created automatically on first workspace creation and reused for
subsequent workspaces in the same installation.

### Environment Variables in Containers

These are injected via inline `env` prefix in `daytona exec`:

| Variable | Value |
|---|---|
| `GT_PROXY_URL` | `https://<proxy_addr>` |
| `GT_PROXY_CERT` | `/run/gt-proxy/client.crt` |
| `GT_PROXY_KEY` | `/run/gt-proxy/client.key` |
| `GT_PROXY_CA` | `/run/gt-proxy/ca.crt` |
| `GIT_SSL_CERT` | `/run/gt-proxy/client.crt` |
| `GIT_SSL_KEY` | `/run/gt-proxy/client.key` |
| `GIT_SSL_CAINFO` | `/run/gt-proxy/ca.crt` |
| `GT_RIG` | Rig name |
| `GT_POLECAT` | Polecat name |
| `GT_ROLE` | `<rig>/polecats/<polecat>` |
| `GT_TOWN_ROOT` | Town root path |
| `GT_RUN` | Session run ID |
| `BD_DOLT_AUTO_COMMIT` | `off` |

Plus any entries from `remote_backend.env`.

## The `--daytona` Flag

You can force Daytona mode even without a `remote_backend` config:

```bash
gt sling my-bead myrig --daytona
```

This synthesizes a minimal `RemoteBackend{Provider: "daytona"}` with defaults.
Useful for one-off testing. The proxy must still be running.

When `remote_backend` is configured in the rig settings, the flag is not needed —
Daytona mode is auto-detected.

## Non-Interactive Operation

Daytona CLI v0.149+ no longer prompts for confirmation on mutating commands,
so no `--yes` flag is needed. All commands run non-interactively by default.

## Resource Sizing

Workspace resources can be configured at three levels of granularity:

### Resource Class

Set a predefined tier:

```json
{
  "remote_backend": {
    "provider": "daytona",
    "class": "medium"
  }
}
```

Available classes depend on your Daytona provider configuration (typically
`"small"`, `"medium"`, `"large"`).

### Explicit Resources

Override individual resource dimensions:

```json
{
  "remote_backend": {
    "provider": "daytona",
    "cpu": 4,
    "memory": 8192,
    "disk": 100
  }
}
```

- `cpu` — number of CPU cores
- `memory` — memory in MB
- `disk` — disk size in GB

Explicit values override the corresponding `class` defaults when both are set.

### Target Region

Place workspaces in a specific geographic region:

```json
{
  "remote_backend": {
    "provider": "daytona",
    "target": "us"
  }
}
```

Available targets depend on your Daytona provider. Common values: `"us"`, `"eu"`.

## Network Isolation

Restrict outbound network access from polecat containers.

### Manual Mode

Use `network_block_all` and `network_allow_list` for direct control:

```json
{
  "remote_backend": {
    "provider": "daytona",
    "network_block_all": true,
    "network_allow_list": "10.0.0.0/8,172.16.0.0/12"
  }
}
```

When `network_block_all` is `true`, the container can only reach CIDRs listed
in `network_allow_list` (comma-separated). The proxy address should be included
in the allow list so containers can reach the mTLS proxy for git and
control-plane operations.

### Sandboxed Mode (Recommended)

Use `sandboxed_network` for automatic network isolation:

```json
{
  "remote_backend": {
    "provider": "daytona",
    "proxy_addr": "172.91.106.250:9876",
    "sandboxed_network": true,
    "allowed_ips": ["203.0.113.10"]
  }
}
```

When `sandboxed_network` is `true`:

1. `network_block_all` is implied (set automatically).
2. The host from `proxy_addr` is auto-added to the allow list as a `/32` CIDR
   so containers can always reach the mTLS proxy.
3. Each entry in `allowed_ips` is added to the allow list. Bare IPs (without a
   CIDR suffix) get `/32` appended automatically.
4. Any existing `network_allow_list` value is preserved and merged.

This is the recommended approach because it derives the proxy allow-rule from
the existing `proxy_addr` config, reducing duplication and misconfiguration risk.

> **Note:** Daytona enforces platform-level network restrictions by subscription
> tier. Tier 1 and Tier 2 sandboxes have restricted outbound internet regardless
> of `--network-allow-list`. Tier 3+ is required for unrestricted (or
> selectively restricted) networking.

## Health Checks

### `gt doctor` Daytona Check

> **Status:** The `gt doctor` Daytona version check is planned but not yet
> implemented. The check will verify that `daytona` is on PATH and meets the
> minimum version requirement. Until then, manually verify your CLI version
> with `daytona version`.

Recommended minimum version: **0.49.0** (for `--auto-stop-interval`,
`--label`, `--volume`, and pagination support in `daytona list`).

## Discovery and Reconciliation

### Discovering Workspaces

List all Daytona workspaces owned by this Gas Town installation for a rig:

```bash
gt polecat discover myrig
```

This cross-references `daytona list` output (filtered by install prefix) against
polecat agent beads, producing a report:

| Status | Meaning |
|---|---|
| **healthy** | Workspace and bead are matched |
| **orphaned_workspace** | Workspace exists but no corresponding bead |
| **orphaned_bead** | Bead references a workspace that no longer exists |

### Auto-Reconciliation

```bash
gt polecat discover myrig --reconcile
```

- **Orphaned workspaces** are archived (and deleted if `auto_delete` is set)
- **Orphaned beads** have their `daytona_workspace` label cleared

Preview what would happen:

```bash
gt polecat discover myrig --reconcile --dry-run
```

### Periodic Reconcile Patrol

The daemon runs a periodic reconciliation patrol to catch workspace drift and
orphans automatically. Configure in `mayor/daemon.json`:

```json
{
  "patrols": {
    "daytona_reconcile": {
      "enabled": true,
      "interval": "30m"
    }
  }
}
```

| Field | Default | Description |
|---|---|---|
| `enabled` | `true` | Enable/disable the patrol |
| `interval` | `"30m"` | How often to run reconciliation |

The patrol also runs once at daemon startup to catch workspaces left running
after an unclean shutdown.

## Observability

### Create Duration Metric

Workspace creation latency is recorded as an OpenTelemetry histogram:

```
gastown.polecat.create_duration_seconds
```

Labels:

| Label | Values | Description |
|---|---|---|
| `start_type` | `"cold"`, `"warm"` | Cold = image pull, warm = snapshot-based |
| `status` | `"ok"`, `"error"` | Whether creation succeeded |

The metric is recorded at four points in the `addDaytona()` provisioning flow:
1. On `Create()` failure
2. On cert injection failure
3. On post-create setup failure
4. On successful completion

Use this metric to monitor provisioning latency and identify when snapshot-based
creation (`warm`) provides meaningful improvement over image-based (`cold`).

## Container Image Requirements

The container image must include:

1. **Your agent** — e.g., `claude-code` via `npm install -g @anthropic-ai/claude-code`
2. **`gt-proxy-client`** — installed as both `/usr/local/bin/gt` and
   `/usr/local/bin/bd` (symlinks to the same binary)
3. **`git`** — for repository operations through the proxy
4. **Standard POSIX tools** — `sh`, `tee`, `mkdir` (used during cert injection)

The proxy client binary detects proxy mode by checking for `GT_PROXY_URL`,
`GT_PROXY_CERT`, `GT_PROXY_KEY`, and `GT_PROXY_CA` environment variables. When
all four are set, it forwards commands to the proxy. Otherwise, it falls through
to the real binary at `/usr/local/bin/gt.real` (or `GT_REAL_BIN`).

## Proxy Server Setup

See [proxy-server.md](proxy-server.md) for full details. Quick reference:

```bash
# Start with defaults
gt-proxy-server --town-root ~/gt

# Custom listen addresses
gt-proxy-server \
  --town-root ~/gt \
  --listen 0.0.0.0:9876 \
  --admin-listen 127.0.0.1:9877
```

The proxy must be reachable from inside Daytona containers. If using Docker-based
Daytona, `172.17.0.1` (the Docker bridge gateway) is typically the right address
for `proxy_addr`. The `proxy_admin_addr` stays on localhost since only the host
needs admin access.

### TLS Details

- TLS 1.3, mutual authentication (`RequireAndVerifyClientCert`)
- CA: ECDSA P-256, self-signed, 10-year validity, CN `GasTown CA`
- Server cert: includes all local interface IPs as SANs
- Polecat certs: 30-day TTL, CN format `gt-<rig>-<name>`
- In-memory cert deny-list checked at TLS handshake

## Troubleshooting

### Preflight Failures

| Error | Fix |
|---|---|
| `daytona CLI not found` | Install from https://www.daytona.io/docs/installation/installation/ |
| `daytona version too old` | Upgrade to >= 0.149.0 |
| `proxy server not reachable` | Start `gt-proxy-server --town-root <path>` |
| `CA certificate not found` | The proxy server generates the CA on first start. Ensure it has started at least once. |

### Container Cannot Reach Proxy

- Check that `proxy_addr` is reachable from inside the container network
- For Docker-based Daytona, use the Docker bridge IP (usually `172.17.0.1`)
- Verify firewall rules allow the proxy port
- If using `network_block_all`, ensure the proxy address is in `network_allow_list`
- If using `sandboxed_network`, the proxy address is added automatically — check `allowed_ips` for any other required destinations
- Daytona Tier 1/2 sandboxes restrict outbound internet at the platform level; `--network-allow-list` does not override this. Upgrade to Tier 3+ for configurable network policies

### Workspace Stuck or Orphaned

```bash
# See what Daytona knows about
gt polecat discover myrig

# Clean up orphans
gt polecat discover myrig --reconcile

# Preview cleanup
gt polecat discover myrig --reconcile --dry-run

# Manual cleanup
daytona stop <workspace-name>
daytona delete <workspace-name>
```

### Certificate Issues

- Certs are stored on the shared volume at `/run/gt-proxy/` inside the container
- If a cert is revoked (polecat removed), the workspace's TLS connections will
  fail immediately at handshake
- The deny-list is in-memory only — restarting the proxy clears it

### Checking Workspace State

```bash
# List all workspaces for this installation
daytona list -o json | jq '.[] | select(.name | startswith("gt-"))'

# Check a specific workspace
daytona info <workspace-name>

# Execute a command in a workspace
daytona exec <workspace-name> -- whoami
```
