# Gastown Architecture (Full)

This document describes Gastown's runtime architecture as implemented in code. It focuses on explicit boundaries, runtime surfaces, lifecycle, trust boundaries, config precedence, persistence/state, and observability.

Primary evidence sources for this writeup:
- `/.sisyphus/evidence/task-3-gastown-architecture-surfaces.md`
- `/.sisyphus/evidence/task-4-gastown-features.md`
- `/.sisyphus/evidence/task-5-gastown-workflows.md`

Cross-repo interoperability evidence and unresolved mismatches (record as `unknown/conflict`): `.sisyphus/evidence/task-13-cross-repo-workflow-reconciliation.md`.

## Architecture Boundaries

- CLI entrypoint boundary: the `gt` process starts in `gastown/cmd/gt/main.go` and immediately delegates to the Cobra root via `cmd.Execute()` in `gastown/internal/cmd/root.go`.
- CLI naming boundary: the root command name is resolved once from `GT_COMMAND` (default `gt`) in `gastown/internal/cli/name.go` and applied to `rootCmd.Use` in `gastown/internal/cmd/root.go`.
- Dashboard mode boundary: `gt dashboard` selects between setup-mode HTTP (`web.NewSetupMux`) vs dashboard-mode HTTP (`web.NewDashboardMux`) based on workspace detection via `workspace.FindFromCwdOrError()` in `gastown/internal/cmd/dashboard.go`.
- Dashboard mux boundary: dashboard-mode HTTP multiplexes `/api/` (API handler), `/static/` (embedded assets), and `/` (HTML dashboard handler) inside `gastown/internal/web/handler.go`.
- Setup mux boundary: setup-mode HTTP multiplexes `/api/` (setup API) and `/` (setup HTML) inside `gastown/internal/web/setup.go`.
- Proxy surface boundary: `gt-proxy-server` serves mTLS data-plane endpoints (`/v1/exec`, `/v1/git/*`) on its TLS listener, and optionally serves a separate non-TLS admin plane (`/v1/admin/*`) on an independent listener in `gastown/internal/proxy/server.go`.
- Proxy allowlist discovery boundary: `gt-proxy-server` can derive allowed gt subcommands by running `gt proxy-subcmds` (hidden command that scans Cobra annotations) in `gastown/cmd/gt-proxy-server/main.go` and `gastown/internal/cmd/proxy_subcmds.go`.
- Proxy client boundary: `gt-proxy-client` decides between proxying over mTLS vs execing the "real" binary (default `/usr/local/bin/gt.real`) based on presence of `GT_PROXY_*` env vars in `gastown/cmd/gt-proxy-client/main.go`.

## Process and Lifecycle

- CLI boot: `main()` calls `cmd.Execute()` in `gastown/cmd/gt/main.go`, which initializes OTel best-effort (`telemetry.Init`) and optionally injects OTEL process env (`telemetry.SetProcessOTELAttrs`) before running the Cobra command tree (`rootCmd.Execute()`) in `gastown/internal/cmd/root.go`.
- CLI per-command pre-run: `rootCmd.PersistentPreRunE` initializes CLI theme (reads town settings when available), appends command-usage JSONL telemetry, initializes session registries from detected town root, and performs non-blocking warnings and best-effort heartbeat touching in `gastown/internal/cmd/root.go`, `gastown/internal/cmd/telemetry.go`, and `gastown/internal/workspace/find.go`.
- Dashboard start: `gt dashboard` chooses setup mode when no workspace is found, otherwise it loads town settings (when readable) to pass web timeout config into `web.NewDashboardMux` in `gastown/internal/cmd/dashboard.go`.
- Setup-mode onboarding workflow: setup API handlers validate and then run local `gt` subprocesses for install and rig add, and `/api/launch` starts a new `gt dashboard` process and probes readiness by polling `/api/commands` in `gastown/internal/web/setup.go`.
- Dashboard command execution workflow: `/api/run` validates CSRF token on POST, validates command whitelist and blocked patterns, enforces server-side confirmation, sanitizes args, limits concurrency, and runs a `gt` subprocess via `exec.CommandContext` in `gastown/internal/web/api.go` and `gastown/internal/web/commands.go`.
- Dashboard SSE workflow: `/api/events` is routed by `APIHandler.ServeHTTP` in `gastown/internal/web/api.go` (event emission and hash computation live in the same file).
- Proxy server boot: `gt-proxy-server` loads a JSON config file (default `~/gt/.runtime/proxy/config.json`), merges explicit flags over file values, resolves `town-root` from `GT_TOWN` or `~/gt`, loads or generates a CA under `~/gt/.runtime/ca`, then starts the mTLS server in `gastown/cmd/gt-proxy-server/main.go` and `gastown/internal/proxy/ca.go`.
- Proxy server request lifecycle: the TLS listener requires and verifies client certs, then checks the deny list during the TLS handshake, before routing to `/v1/exec` and `/v1/git/` handlers in `gastown/internal/proxy/server.go` and `gastown/internal/proxy/denylist.go`.
- Proxy exec lifecycle: `/v1/exec` enforces command allowlist and optional subcommand allowlists, per-client rate limiting (by cert CN), global concurrency caps, request body size bounds, and a restricted subprocess environment in `gastown/internal/proxy/exec.go`.
- Proxy git lifecycle: `/v1/git/<rig>/*` validates the rig segment, pins the repo path to `<townRoot>/<rig>/.repo.git`, and for pushes (receive-pack) enforces CN-scoped ref authorization before streaming into `git-receive-pack` in `gastown/internal/proxy/git.go`.
- Proxy client lifecycle: `gt-proxy-client` builds an mTLS client from `GT_PROXY_CERT`, `GT_PROXY_KEY`, and `GT_PROXY_CA`, then posts argv to `/v1/exec`; if any required env var is missing it `exec`s the real binary in `gastown/cmd/gt-proxy-client/main.go`.

## Surfaces

### CLI (`gt`)

- Entry: `gastown/cmd/gt/main.go` exits with the code returned by `gastown/internal/cmd/root.go`.
- Command routing: `rootCmd` is a Cobra command with `PersistentPreRunE`, and subcommands register via `rootCmd.AddCommand(...)` in per-command `init()` functions (for example `gastown/internal/cmd/dashboard.go` and `gastown/internal/cmd/proxy_subcmds.go`, with the root defined in `gastown/internal/cmd/root.go`).
- Name override: `GT_COMMAND` overrides the CLI name for help/usage surfaces in `gastown/internal/cli/name.go` and `gastown/internal/cmd/root.go`.
- Proxy allowlist discovery: hidden `gt proxy-subcmds` prints a semicolon-separated allowlist string derived from Cobra annotations in `gastown/internal/cmd/proxy_subcmds.go`.

### Dashboard HTTP (`gt dashboard`)

- Mode selection boundary (setup vs dashboard): `gt dashboard` selects setup-mode mux when workspace lookup fails, otherwise dashboard-mode mux in `gastown/internal/cmd/dashboard.go`.
- Dashboard mux boundary: `web.NewDashboardMux` mounts `/api/` (dashboard API), `/static/` (embedded assets), and `/` (HTML handler) in `gastown/internal/web/handler.go`.
- Dashboard API surface: `APIHandler.ServeHTTP` routes `/api/run`, `/api/commands`, `/api/options`, mail endpoints, issue endpoints, PR endpoint, crew, ready, SSE (`/api/events`), and session preview in `gastown/internal/web/api.go`.
- Setup mux boundary: `web.NewSetupMux` mounts `/api/` (setup API) and `/` (setup HTML) in `gastown/internal/web/setup.go`.
- Setup API surface: `SetupAPIHandler.ServeHTTP` routes `/api/install`, `/api/rig/add`, `/api/check-workspace`, `/api/launch`, and `/api/status` in `gastown/internal/web/setup.go`.

### Proxy server (`gt-proxy-server`) and proxy client

- Proxy server endpoints: the mTLS listener mounts `/v1/exec` and `/v1/git/` in `gastown/internal/proxy/server.go`.
- Proxy admin endpoints: the optional non-TLS admin listener mounts `/v1/admin/deny-cert` and `/v1/admin/issue-cert` in `gastown/internal/proxy/server.go`.
- Proxy exec surface: `/v1/exec` accepts JSON `{"argv":[...]}` and returns JSON stdout/stderr/exitCode in `gastown/internal/proxy/exec.go`.
- Proxy git surface: `/v1/git/<rig>/info/refs`, `/v1/git/<rig>/git-upload-pack`, and `/v1/git/<rig>/git-receive-pack` are implemented in `gastown/internal/proxy/git.go`.
- Proxy client forwarding surface: `gt-proxy-client` is the container-side binary that forwards argv to `/v1/exec` when `GT_PROXY_URL`, `GT_PROXY_CERT`, `GT_PROXY_KEY`, and `GT_PROXY_CA` are set in `gastown/cmd/gt-proxy-client/main.go`.

## Trust Boundaries

- Dashboard CSRF gate (required): all dashboard POST requests require the `X-Dashboard-Token` header to match a per-process CSRF token, enforced before routing in `APIHandler.ServeHTTP` in `gastown/internal/web/api.go`, and the token is generated in `gastown/internal/web/handler.go`.
- Dashboard `/api/run` command whitelist gate (required): `/api/run` rejects commands not present in `AllowedCommands` and rejects commands matching `BlockedPatterns`, before subprocess execution in `gastown/internal/web/api.go` and `gastown/internal/web/commands.go`.
- Dashboard `/api/run` defense-in-depth: server-side confirmation is enforced for commands marked `Confirm`, and args are sanitized to remove shell metacharacters before `exec.CommandContext` is invoked in `gastown/internal/web/api.go` and `gastown/internal/web/commands.go`.
- Same-origin assumption: dashboard and setup API handlers intentionally omit CORS headers to avoid enabling cross-origin requests, relying on same-origin serving plus CSRF token checks on POST in `gastown/internal/web/api.go` and `gastown/internal/web/setup.go`.
- Setup-mode mutating endpoints: setup-mode `/api/install` and `/api/rig/add` validate inputs and then run `gt` subprocesses, so the setup UI is a privileged local surface guarded by the same CSRF mechanism in `gastown/internal/web/setup.go`.
- Proxy mTLS boundary (required): `gt-proxy-server` requires and verifies client certificates (`tls.RequireAndVerifyClientCert`) with a CA trust pool and TLS 1.3 minimum in `gastown/internal/proxy/server.go`.
- Proxy denylist boundary (required): revoked client cert serials are denied during TLS handshake via `VerifyPeerCertificate`, using an in-memory deny list in `gastown/internal/proxy/server.go` and `gastown/internal/proxy/denylist.go`.
- Proxy allowlist boundary (required): `/v1/exec` rejects any argv[0] not present in the configured allowlist and can additionally enforce subcommand allowlists per tool in `gastown/internal/proxy/exec.go` and `gastown/internal/proxy/server.go`.
- Proxy subprocess environment boundary: proxy-executed commands run with a restricted environment (minimal env, optional `GT_PROXY_IDENTITY`) to reduce credential leakage in `gastown/internal/proxy/exec.go`.
- Proxy git push authorization boundary: pushes are constrained so every updated ref must be under `refs/heads/polecat/<cnName>-*`, enforced before `git-receive-pack` sees the body in `gastown/internal/proxy/git.go`.
- Proxy admin plane boundary: the admin server is intentionally non-TLS and documented as intended for same-host operator tools, with loopback binding recommended in `gastown/internal/proxy/server.go`.

## Config Precedence

- CLI name: `GT_COMMAND` overrides the default command name (`gt`) in `gastown/internal/cli/name.go` and is applied to the Cobra root in `gastown/internal/cmd/root.go`.
- CLI theme mode: `GT_THEME` env takes precedence over the configured value, falling back to `auto` when neither is set in `gastown/internal/ui/terminal.go`.
- CLI theme config source: the CLI attempts to load town settings from `<townRoot>/settings/config.json` to obtain a config theme value, and then initializes theme with env taking precedence in `gastown/internal/cmd/root.go` and `gastown/internal/config/loader.go`.
- Account selection: `GT_ACCOUNT` env overrides `--account` flag, which overrides the default account in the accounts config in `gastown/internal/config/loader.go`.
- Dashboard web timeouts: `gt dashboard` loads `<townRoot>/settings/config.json` when present and passes `WebTimeouts` into `web.NewDashboardMux`; if nil, dashboard mux applies defaults in `gastown/internal/cmd/dashboard.go`, `gastown/internal/config/loader.go`, and `gastown/internal/web/handler.go`.
- Proxy server config file: `gt-proxy-server` reads a JSON config file at `~/gt/.runtime/proxy/config.json` by default, and only overrides file values for flags explicitly set on the command line in `gastown/cmd/gt-proxy-server/main.go`.
- Proxy `town-root`: when not explicitly set, proxy server resolves town root from `GT_TOWN` env, otherwise defaults to `~/gt` in `gastown/cmd/gt-proxy-server/main.go`.
- Proxy allowed subcommands: proxy server attempts to discover the gt subcommand allowlist by running `gt proxy-subcmds`, otherwise it uses a built-in default string in `gastown/cmd/gt-proxy-server/main.go` and `gastown/internal/cmd/proxy_subcmds.go`.

## Persistence and State

- Workspace identity markers: workspace detection uses filesystem markers `mayor/town.json` (primary) and `mayor/` (secondary) in `gastown/internal/workspace/find.go`.
- Town settings persistence: town settings live at `<townRoot>/settings/config.json`; when absent, defaults are created in memory (not written) by `LoadOrCreateTownSettings` in `gastown/internal/config/loader.go`.
- Dashboard runtime state: CSRF tokens are generated on mux construction and stored in-memory inside the handler instances created by `NewDashboardMux` and `NewSetupMux` in `gastown/internal/web/handler.go` and `gastown/internal/web/setup.go`.
- CLI command usage telemetry persistence: command usage is appended as JSONL at `$GT_HOME/.gt/cmd-usage.jsonl` when `GT_HOME` is set, otherwise `~/.gt/cmd-usage.jsonl`, in `gastown/internal/cmd/paths.go` and `gastown/internal/cmd/telemetry.go`.
- Proxy config persistence: `gt-proxy-server` reads config from `~/gt/.runtime/proxy/config.json` by default in `gastown/cmd/gt-proxy-server/main.go`.
- Proxy CA persistence: the proxy CA cert and key are stored as `ca.crt` and `ca.key` under the CA directory (default `~/gt/.runtime/ca`) in `gastown/internal/proxy/ca.go` and `gastown/cmd/gt-proxy-server/main.go`.
- Proxy denylist state: revoked cert serials are stored in an in-memory deny list with no persistence and no removals in `gastown/internal/proxy/denylist.go`.
- Proxy git data-plane storage: bare repos are addressed as `<townRoot>/<rig>/.repo.git` in `gastown/internal/proxy/git.go`.

## Observability

- CLI OpenTelemetry: telemetry is opt-in via `GT_OTEL_METRICS_URL` and/or `GT_OTEL_LOGS_URL`; when active, defaults are filled for unset endpoints and providers are initialized best-effort in `gastown/internal/telemetry/telemetry.go` and `gastown/internal/cmd/root.go`.
- OTel context propagation: when telemetry is active, `gt` sets `OTEL_RESOURCE_ATTRIBUTES` and mirrors GT endpoints into bd env vars so bd subprocesses inherit context, in `gastown/internal/telemetry/subprocess.go` and `gastown/internal/cmd/root.go`.
- Proxy structured logs: proxy server uses structured `slog` logs for listen events, admin listen events, and request/audit events (exec and git) in `gastown/internal/proxy/server.go`, `gastown/internal/proxy/exec.go`, and `gastown/internal/proxy/git.go`.
- CLI usage logs: per-command JSONL usage records are appended to `cmd-usage.jsonl` in `gastown/internal/cmd/telemetry.go`.
- Dashboard command logging is indicated only: `/api/run` includes a placeholder comment for structured logging rather than a full logging implementation in `gastown/internal/web/api.go`.

## Known Gaps / Indicated Only

- unknown/conflict: dashboard command execution logging is not implemented beyond a placeholder comment, so production observability for `/api/run` relies on external logging (or future structured logs) in `gastown/internal/web/api.go`.
- unknown/conflict: setup-mode and dashboard-mode both omit CORS headers and rely on same-origin serving plus CSRF header checks, but the code does not document a deployment requirement that the dashboard must remain loopback-only, even though the CLI supports binding to `0.0.0.0` in `gastown/internal/cmd/dashboard.go` and CSRF enforcement lives in `gastown/internal/web/api.go`.
- unknown/conflict (Task 13 cross-repo): Dolt server PID file location mismatch. Gastown stale PID cleanup deletes `<beadsDir>/dolt/dolt-server.pid`, but Beads state files include `<beadsDir>/dolt-server.pid`. Evidence: `.sisyphus/evidence/task-13-cross-repo-workflow-reconciliation.md:34`, `gastown/internal/beads/stale_pid.go:21`, `beads/internal/doltserver/doltserver.go:17`, `beads/internal/doltserver/doltserver.go:110`.
- unknown/conflict (Task 13 cross-repo): Dolt auto-commit knob mismatch. Gastown sets `BD_DOLT_AUTO_COMMIT=on`, but Beads exposes `--dolt-auto-commit` (flag/config) and no `BD_DOLT_AUTO_COMMIT` usage is evidenced in Task 13. Evidence: `.sisyphus/evidence/task-13-cross-repo-workflow-reconciliation.md:35`, `gastown/internal/cmd/bd_helpers.go:103`, `beads/cmd/bd/main.go:189`.
- unknown/conflict (Task 13 cross-repo): Telemetry resource attribution interoperability is unclear. Gastown sets `OTEL_RESOURCE_ATTRIBUTES` for `bd` subprocesses, but Beads telemetry init constructs its resource without an explicit env detector, so GT labels may be ignored. Evidence: `.sisyphus/evidence/task-13-cross-repo-workflow-reconciliation.md:36`, `gastown/internal/telemetry/subprocess.go:64`, `beads/internal/telemetry/telemetry.go:71`.
