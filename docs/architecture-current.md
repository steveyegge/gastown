Docs generated against (gastown HEAD): `7cc2716b18b514572ce7488a4c5399caf02a38b7`

# Architecture (Current)

This document captures the current architecture surfaces and boundaries for the `gastown/` repo, with evidence-first citations to concrete files. If a claim cannot be backed by a file reference, it is marked `unknown/conflict`.

## Cross-Repo Glossary

- `setup-mode`: Workspace-bootstrap dashboard routing mode used when no workspace is detected.
- `dashboard-mode`: Operational dashboard routing mode used when a workspace is detected.
- `data plane`: Request paths that execute primary workload operations.
- `control plane`: Request paths that perform administrative/configuration control operations.
- `MCP`: Model Context Protocol integration surface and tooling.
- `npm wrapper`: Node-based launcher that forwards CLI arguments/environment to a packaged native binary.
- `Cobra \`Use:\` surface`: The command names/signatures declared in Cobra `Use:` fields that define the observable CLI surface.

## Local Glossary

- Mayor: Town-level coordinator role surfaced as `gt mayor ...` (evidence: `gastown/internal/cmd/mayor.go`, `gastown/internal/constants/constants.go`).
- Deacon: Town-level watchdog/supervisor role surfaced as `gt deacon ...` (evidence: `gastown/internal/cmd/deacon.go`, `gastown/internal/daemon/types.go`).
- Witness: Per-rig monitor role surfaced as `gt witness ...` (evidence: `gastown/internal/cmd/witness.go`).
- Refinery: Per-rig merge-queue processor role surfaced as `gt refinery ...` (evidence: `gastown/internal/cmd/refinery.go`).
- Polecat: Named, ephemeral worker identity surfaced as `gt polecat ...` (evidence: `gastown/internal/cmd/polecat.go`).
- Crew: Named, persistent workspace identity surfaced as `gt crew ...` (evidence: `gastown/internal/cmd/crew.go`).
- Dog: Deacon-managed, cross-rig worker with kennel/worktree model surfaced as `gt dog ...` (evidence: `gastown/internal/cmd/dog.go`).
- Dashboard: The long-running web server started by `gt dashboard`, exposing dashboard-mode routes under `/api/*` when a workspace is detected (evidence: `gastown/internal/cmd/dashboard.go`, `gastown/internal/web/handler.go`, `gastown/internal/web/api.go`).
- Setup Dashboard: The setup-mode web server surface used when no workspace is detected, also mounted under `/api/*` but focused on install/rig/bootstrap operations (evidence: `gastown/internal/cmd/dashboard.go`, `gastown/internal/web/setup.go`).
- Proxy: A separate HTTP server providing mTLS-protected data plane endpoints (`/v1/exec`, `/v1/git/*`) and an optional local-admin control plane (`/v1/admin/*`) (evidence: `gastown/internal/proxy/server.go`, `gastown/internal/proxy/exec.go`, `gastown/internal/proxy/git.go`).

## Architecture Boundaries

Gas Town exposes three primary execution surfaces, plus one mode switch inside the dashboard surface.

- CLI surface: `gt` is the primary entrypoint, it performs pre-run setup and then dispatches to subcommands (evidence: `gastown/internal/cmd/root.go`).
- Dashboard surface (dashboard-mode): `/api/*` routes that run validated local command subprocesses and serve operational JSON/SSE endpoints (evidence: `gastown/internal/web/handler.go`, `gastown/internal/web/api.go`, `gastown/internal/web/commands.go`).
- Dashboard surface (setup-mode): `/api/*` routes that install/validate/launch a workspace when no workspace is found (evidence: `gastown/internal/web/setup.go`, `gastown/internal/web/validate.go`).
- Proxy surface: `/v1/*` routes that accept authenticated RPC-like requests over mTLS, plus an optional non-TLS admin listener for local operations (evidence: `gastown/internal/proxy/server.go`, `gastown/internal/proxy/exec.go`, `gastown/internal/proxy/git.go`).

Mode selection for `gt dashboard` is a boundary worth calling out explicitly: the same CLI command chooses a different mux based on workspace detection, which means `/api/*` is not a single trust/behavior domain (evidence: `gastown/internal/cmd/dashboard.go`, `gastown/internal/web/setup.go`, `gastown/internal/web/handler.go`).

Evidence pointers:

- Command surface wiring and role trees: `gastown/internal/cmd/root.go`, `gastown/internal/cmd/dashboard.go`, `gastown/internal/cmd/mayor.go`, `gastown/internal/cmd/deacon.go`, `gastown/internal/cmd/witness.go`, `gastown/internal/cmd/refinery.go`, `gastown/internal/cmd/polecat.go`, `gastown/internal/cmd/crew.go`, `gastown/internal/cmd/dog.go`.
- Web mode split: `gastown/internal/web/setup.go`, `gastown/internal/web/handler.go`.
- Proxy surface split: `gastown/internal/proxy/server.go`.

## Process and Lifecycle

Role processes are represented as explicit CLI command trees and as session identity primitives.

- Role vocabulary is centralized and reused as routing and identity data (evidence: `gastown/internal/constants/constants.go`, `gastown/internal/session/identity.go`).
- Mayor and Deacon are town-level processes, while Witness and Refinery are per-rig services (evidence: `gastown/internal/cmd/mayor.go`, `gastown/internal/cmd/deacon.go`, `gastown/internal/cmd/witness.go`, `gastown/internal/cmd/refinery.go`).
- Polecat and Crew are named worker identities with different lifecycle models (ephemeral vs persistent) (evidence: `gastown/internal/cmd/polecat.go`, `gastown/internal/cmd/crew.go`).

Runtime lifecycle mechanics show up as pid/heartbeat/keepalive and pause/lock artifacts.

- Session PID tracking and process bookkeeping are implemented under session and runtime helpers (evidence: `gastown/internal/session/pidtrack.go`).
- Polecat heartbeats and liveness touchpoints appear as runtime files (evidence: `gastown/internal/polecat/heartbeat.go`).
- Keepalive and lock primitives are persisted as runtime artifacts (evidence: `gastown/internal/keepalive/keepalive.go`, `gastown/internal/lock/lock.go`).
- Deacon pause state is treated as a runtime control artifact (evidence: `gastown/internal/deacon/pause.go`).

Evidence pointers:

- Role boundaries and responsibilities: `gastown/internal/daemon/types.go`, `gastown/internal/cmd/deacon.go`.
- Runtime state mechanics: `gastown/internal/session/pidtrack.go`, `gastown/internal/polecat/heartbeat.go`, `gastown/internal/keepalive/keepalive.go`, `gastown/internal/lock/lock.go`, `gastown/internal/deacon/pause.go`.

## Trust Boundaries

The trust model differs by surface. Do not treat the dashboard, setup dashboard, and proxy as the same boundary just because they are all HTTP servers.

Dashboard API (dashboard-mode) trust boundaries:

- `POST /api/run` executes local `gt` subprocesses, but only after validation steps like allowlist and blocked-pattern checks, argument sanitization, timeout clamping, and concurrency gating (evidence: `gastown/internal/web/api.go`, `gastown/internal/web/commands.go`).
- CSRF protection on POST routes uses `X-Dashboard-Token`, and CORS is not enabled under same-origin deployment assumptions (evidence: `gastown/internal/web/api.go`, `gastown/internal/web/setup.go`).
- Command failures are represented in JSON payloads rather than by crashing the server process (evidence: `gastown/internal/web/api.go`).

Setup API (setup-mode) trust boundaries:

- Setup-mode endpoints call local `gt` subprocesses (`gt install`, `gt rig add`, `gt status`) with explicit input validation (evidence: `gastown/internal/web/setup.go`, `gastown/internal/web/validate.go`).
- `/api/launch` starts a new dashboard process and probes readiness before redirecting (evidence: `gastown/internal/web/setup.go`).

Proxy API trust boundaries:

- Data plane endpoints require verified client certificates (`RequireAndVerifyClientCert`) and deny revoked serials at handshake time (evidence: `gastown/internal/proxy/server.go`).
- `/v1/exec` is constrained by allowlisted binaries and optional allowlisted subcommands, plus per-client rate limits, global concurrency caps, bounded request bodies, and restricted subprocess environments (evidence: `gastown/internal/proxy/server.go`, `gastown/internal/proxy/exec.go`).
- Git smart-HTTP bridging enforces CN-scoped ref rules for push, rejecting unauthorized refs before forwarding pack streams (evidence: `gastown/internal/proxy/git.go`).
- Admin endpoints are exposed on an optional non-TLS listener intended for same-host operations, and are recommended to bind to loopback only (evidence: `gastown/internal/proxy/server.go`).

Evidence pointers:

- Dashboard command execution guardrails: `gastown/internal/web/api.go`, `gastown/internal/web/commands.go`.
- Setup validation and launch flow: `gastown/internal/web/setup.go`, `gastown/internal/web/validate.go`.
- Proxy TLS and admin listener: `gastown/internal/proxy/server.go`.

## Config Precedence

Configuration and settings resolution has explicit precedence rules in the loader and in role-specific wiring.

- Account config directory precedence is `GT_ACCOUNT` env, then `--account` flag, then default account from `mayor/accounts.json` (evidence: `gastown/internal/config/loader.go`).
- Role-agent resolution precedence is rig `role_agents`, then town `role_agents`, then the default agent chain, with special-case override semantics (evidence: `gastown/internal/config/loader.go`).
- Dashboard timeouts are parsed from town settings with per-field fallback to compiled defaults, and if settings load fails the dashboard mux defaults apply (evidence: `gastown/internal/cmd/dashboard.go`, `gastown/internal/web/handler.go`, `gastown/internal/config/types.go`, `gastown/internal/config/loader.go`).
- Dolt runtime connection settings prefer env overrides (`GT_DOLT_*`), with fallback port selection when env is absent (evidence: `gastown/internal/doltserver/doltserver.go`, `gastown/internal/doltserver/doltserver.go`).

Documentation also defines additional configuration layers used operationally (docs-as-reference, not code-as-truth) (evidence: `gastown/docs/reference.md`).

Evidence pointers:

- Loader precedence implementation: `gastown/internal/config/loader.go`.
- Timeout wiring: `gastown/internal/cmd/dashboard.go`, `gastown/internal/web/handler.go`, `gastown/internal/config/types.go`.
- Settings file examples and merge queue config structure: `gastown/docs/reference.md`.

## Persistence

State splits into town control files, runtime ephemeral artifacts, and durable identity and data roots.

- Town control/state files live under `daemon/` and include logs, pid files, and daemon state JSON (evidence: `gastown/internal/daemon/types.go`, `gastown/internal/doltserver/doltserver.go`).
- Ephemeral runtime state lives under `.runtime/` and includes session pids, heartbeats, keepalive, locks, and deacon pause state (evidence: `gastown/internal/session/pidtrack.go`, `gastown/internal/polecat/heartbeat.go`, `gastown/internal/keepalive/keepalive.go`, `gastown/internal/lock/lock.go`, `gastown/internal/deacon/pause.go`).
- Durable identity/config anchors include `mayor/town.json`, `mayor/rigs.json`, and `settings/config.json` (evidence: `gastown/internal/constants/constants.go`, `gastown/internal/config/loader.go`, `gastown/internal/config/types.go`).
- Rig data durability includes `.beads` data (local or redirected under `mayor/rig/.beads`) and the `.dolt-data` database root (evidence: `gastown/internal/rig/types.go`, `gastown/internal/doctor/rig_check.go`, `gastown/internal/doltserver/doltserver.go`).

Retention and cleanup policy is not fully centralized in one global policy document (`unknown/conflict`), so safe-to-delete guidance is inferred from usage patterns rather than a single authoritative source (evidence: `gastown/internal/daemon/types.go`, `gastown/internal/keepalive/keepalive.go`, `gastown/internal/session/pidtrack.go`, `gastown/internal/polecat/heartbeat.go`).

Evidence pointers:

- Daemon state files: `gastown/internal/daemon/types.go`.
- Rig data roots: `gastown/internal/rig/types.go`, `gastown/internal/doltserver/doltserver.go`.
- Doctor checks for persistent directories: `gastown/internal/doctor/rig_check.go`.

## Observability

Observability is a mix of lightweight local logging and optional OpenTelemetry export.

- CLI command usage is appended as JSONL under a gt home directory (evidence: `gastown/internal/cmd/telemetry.go`, `gastown/internal/cmd/paths.go`).
- OTel metrics/log exporters are optional and best-effort, with resource attribute propagation into subprocess environments (evidence: `gastown/internal/telemetry/telemetry.go`, `gastown/internal/telemetry/subprocess.go`, `gastown/internal/cmd/root.go`).
- Dashboard errors and timeouts are logged server-side, and SSE emits update events for client refresh (evidence: `gastown/internal/web/handler.go`, `gastown/internal/web/api.go`).
- Proxy exec and git operations emit structured audit logs including identity and deny/limit events (evidence: `gastown/internal/proxy/exec.go`, `gastown/internal/proxy/git.go`, `gastown/internal/proxy/server.go`).

Evidence pointers:

- CLI telemetry logging: `gastown/internal/cmd/telemetry.go`, `gastown/internal/cmd/paths.go`.
- OTel wiring: `gastown/internal/telemetry/telemetry.go`, `gastown/internal/telemetry/subprocess.go`.
- Proxy audit logs: `gastown/internal/proxy/exec.go`, `gastown/internal/proxy/git.go`.
