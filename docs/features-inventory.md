Docs generated against gastown@7cc2716b18b514572ce7488a4c5399caf02a38b7.

## Cross-Repo Glossary

- `setup-mode`: Workspace-bootstrap dashboard routing mode used when no workspace is detected.
- `dashboard-mode`: Operational dashboard routing mode used when a workspace is detected.
- `data plane`: Request paths that execute primary workload operations.
- `control plane`: Request paths that perform administrative/configuration control operations.
- `MCP`: Model Context Protocol integration surface and tooling.
- `npm wrapper`: Node-based launcher that forwards CLI arguments/environment to a packaged native binary.
- `Cobra \`Use:\` surface`: The command names/signatures declared in Cobra `Use:` fields that define the observable CLI surface.

## Feature Classification Model

# Feature Classification and Source-Precedence Contract

This document defines the locked status taxonomy, confidence levels, evidence rules, source precedence, and conflict handling used by later feature inventory documents.

This is a schema and governance contract only. It does not claim any specific feature is implemented.

## Status taxonomy (locked)

Only these status tokens are allowed:

- `implemented`
- `partially_implemented`
- `indicated`
- `roadmap`
- `deprecated`
- `unknown/conflict`

### Status definitions

`implemented`
- Meaning: User-visible behavior exists now.
- Hard rule: `implemented` requires non-doc evidence. Docs-only evidence is never sufficient.
- Typical evidence: code/handlers that execute the behavior, CLI wiring that reaches real implementation, HTTP handler code, tests that assert behavior, captured command output that proves behavior (not just help text).

`partially_implemented`
- Meaning: Some user-visible behavior exists, but parts are stubbed, no-op, gated, or explicitly "not yet implemented".
- Typical evidence: code paths present plus TODO/FIXME, placeholder prints, feature-flag off by default, missing side effects.

`indicated`
- Meaning: Strong signals exist that a feature is intended, but implementation is not verified.
- Typical evidence: design docs, README/CLI reference, command or route names with missing backing logic, TODOs that describe near-term behavior.

`roadmap`
- Meaning: Future intent with no usable behavior today.
- Typical evidence: design sections labeled future, TODOs describing new architecture, planned flags or subcommands not present in code.

`deprecated`
- Meaning: Feature exists or existed, but is flagged as deprecated, replaced, hidden, or scheduled for removal.
- Typical evidence: deprecation notes in docs, code comments, hidden commands, warnings in output.

`unknown/conflict`
- Meaning: Evidence is contradictory or incomplete in a way that cannot be resolved with the available sources.
- Typical evidence: code says one thing while docs say another, or two same-precedence sources disagree.

## Confidence levels

Confidence is a separate field from status.

High
- Evidence threshold: At least one high-precedence, non-doc source directly supports the claim.
- Examples of acceptable evidence: code/handler path, failing/passing tests tied to behavior, captured runtime output from executing the surface.

Medium
- Evidence threshold: Evidence supports the claim, but is indirect or missing one critical link.
- Examples: command is wired but side effects are unclear, tests cover only part of the behavior, output exists but inputs and failure modes are unknown.

Low
- Evidence threshold: Claim is based on low-precedence sources or weak signals.
- Examples: docs-only, design-only, TODO-only, or naming that suggests behavior without proof.

## Evidence types and source precedence

When sources conflict, higher precedence wins.

1. Code and handlers (implementation source)
   - Go code that executes behavior, HTTP handlers, routing tables, command wiring that reaches real logic.
2. Executable surfaces (runtime proof)
   - Captured output from actually running commands, listing routes, or hitting endpoints in a reproducible way.
3. Tests
   - Unit/integration tests that assert behavior, including validation and failure modes.
4. Docs (reference)
   - README, CLI reference, reference manuals.
5. Design docs (intent)
   - Design notes, proposals, "future" sections.
6. TODO/FIXME markers (intent signal)
   - TODO comments, stubs, placeholder strings.

## Conflict handling rules

If sources disagree:

- Prefer the highest-precedence source.
- If two sources at the same precedence disagree and you cannot prove which is correct, mark status as `unknown/conflict`.
- Record both claims in the row notes, and include evidence paths for each.
- Do not "average" by choosing `partially_implemented` unless you have direct evidence of partial behavior.

If evidence is missing:

- Use `indicated` or `roadmap` (not `implemented`).
- Use Low confidence unless there is supporting code structure that strongly implies imminent behavior.

## Default classification for common signals

TODO/FIXME
- TODO describing a missing behavior without any working path: default `roadmap` (Low).
- TODO in a code path that otherwise runs and does something user-visible: default `partially_implemented` (Medium or Low).

Design docs
- Design language describing intended behavior without corroborating code: default `indicated` (Low) or `roadmap` (Low) if explicitly future.

Docs and CLI references
- Docs-only description of a feature: default `indicated` (Low).
- Help text alone is not enough for `implemented` unless paired with non-doc evidence.

## Feature inventory row schema (required fields)

Every feature inventory row must include these fields:

- surface: Where the user interacts (CLI command, HTTP route, proxy endpoint, file format, integration tool).
- invocation: Exact example of how it is invoked (command line, request shape, API path).
- inputs: User-controlled inputs and required parameters.
- outputs: User-visible outputs (stdout, JSON, returned data).
- side effects: Persistent changes, network calls, file writes, state mutations.
- evidence path: Concrete pointer(s) to evidence (file paths, test names, or captured command output paths).

Strongly recommended additional fields:

- feature: Human name for the capability.
- status: One of the locked status tokens.
- confidence: High, Medium, Low.
- notes: Constraints, caveats, and conflict details.

## Evidence citation format

- Prefer repo-relative file paths (for example: `gastown/internal/web/api.go`).
- For tests, include the test name when possible.
- For captured runtime output, store it under `.sisyphus/evidence/` and cite that path.
- If a row has multiple evidence items, list them all.

## Feature Inventory

Evidence-driven inventory using the contract taxonomy. `implemented` is only used when backed by non-doc code paths.

| feature | surface | invocation | inputs | outputs | side effects | status | confidence | evidence path | notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Convoy tracking workflow | CLI (GroupWork) | `gt convoy create <name> [issues...]` | convoy name, issue IDs | CLI status/output | creates/updates convoy issues in beads | implemented | High | `gastown/internal/cmd/convoy.go:157`, `gastown/internal/cmd/convoy.go:199` | Representative for Work Management command group (`GroupID: GroupWork`). |
| Polecat lifecycle | CLI (GroupAgents) | `gt polecat list [rig]` | optional rig, flags (`--all`, `--json`) | polecat state list | reads rig + tmux state | implemented | High | `gastown/internal/cmd/polecat.go:31`, `gastown/internal/cmd/polecat.go:62` | Representative for Agent Management command group (`GroupID: GroupAgents`). |
| Mail operations | CLI (GroupComm) | `gt mail send <address>` | destination, subject/body flags | send result text/json | creates mail records and optional queue activity | implemented | High | `gastown/internal/cmd/mail.go:55`, `gastown/internal/cmd/mail.go:98` | Representative for Communication command group (`GroupID: GroupComm`). |
| Service start/launch | CLI (GroupServices) | `gt start [path]` | optional path, service flags | startup status output | starts Mayor/Deacon/service sessions | implemented | High | `gastown/internal/cmd/start.go:59` | Representative for Services command group (`GroupID: GroupServices`). |
| Workspace install | CLI (GroupWorkspace) | `gt install [path]` | target path, `--name`, `--git` | install output | creates workspace directories/config/git | implemented | High | `gastown/internal/cmd/install.go:49` | Representative for Workspace command group (`GroupID: GroupWorkspace`). |
| Runtime/config settings | CLI (GroupConfig) | `gt config ...` | subcommand + values | config read/write output | updates settings files | implemented | High | `gastown/internal/cmd/config.go:22` | Representative for Configuration command group (`GroupID: GroupConfig`). |
| Doctor/diagnostics | CLI (GroupDiag) | `gt doctor` | optional `--fix` | health check report | may auto-fix stale config when requested | implemented | High | `gastown/internal/cmd/doctor.go:23` | Representative for Diagnostics command group (`GroupID: GroupDiag`). |
| Swarm command surface | CLI (deprecated) | `gt swarm ...` | swarm subcommands/flags | deprecated warning + legacy behavior | convoy-like tracking mutations | deprecated | High | `gastown/internal/cmd/swarm.go:40`, `gastown/internal/cmd/swarm.go:43` | Explicitly marked deprecated in Cobra metadata (`Deprecated:` field). |
| Escalation external notifications (email/SMS/Slack/log actions) | CLI + escalation backend | `gt escalate ...` with configured external actions | escalation severity/reason/source + routing actions | currently prints "Would send..." messages | creates escalation beads + mail routing; external channels are stubs | partially_implemented | High | `gastown/internal/cmd/escalate_impl.go:556`, `gastown/internal/cmd/escalate_impl.go:565`, `gastown/internal/cmd/escalate_impl.go:573`, `gastown/internal/cmd/escalate_impl.go:581`, `gastown/internal/cmd/escalate_impl.go:587` | Core escalation works, but external actions are TODO stubs. |
| Synthesis close notifications | CLI synthesis close | `gt synthesis close <convoy-id>` | convoy ID | close confirmation output | closes convoy bead | partially_implemented | Medium | `gastown/internal/cmd/synthesis.go:84`, `gastown/internal/cmd/synthesis.go:367` | Convoy close runs; configured notify path is TODO. |
| Dashboard command execution API | Dashboard `/api` route | `POST /api/run -> handleRun` | JSON `{command, timeout, confirmed}` | JSON `CommandResponse` | executes `gt` subprocesses with validation and timeout | implemented | High | `gastown/internal/web/api.go:110`, `gastown/internal/web/api.go:148` | Explicit path+handler from route switch. |
| Dashboard mail send API | Dashboard `/api` route | `POST /api/mail/send -> handleMailSend` | JSON `{to,subject,body,reply_to}` | JSON success/error payload | executes `gt mail send` | implemented | High | `gastown/internal/web/api.go:122`, `gastown/internal/web/api.go:529` | Includes validation + command execution. |
| Dashboard issue create API | Dashboard `/api` route | `POST /api/issues/create -> handleIssueCreate` | JSON `{title,description,priority}` | JSON `IssueCreateResponse` | executes `bd create` issue | implemented | High | `gastown/internal/web/api.go:126`, `gastown/internal/web/api.go:1063` | Non-doc handler evidence present. |
| Dashboard events stream API | Dashboard `/api` route | `GET /api/events -> handleSSE` | SSE request | event stream | opens stream for dashboard updates | implemented | Medium | `gastown/internal/web/api.go:138` | Route mapping confirms handler wiring; handler body outside captured slice. |
| Setup workspace install API | Setup `/api` route | `POST /api/install -> handleInstall` | JSON `{path,name,git}` | JSON `SetupResponse` | runs `gt install` | implemented | High | `gastown/internal/web/setup.go:66`, `gastown/internal/web/setup.go:122` | Explicit setup-mode route and handler. |
| Setup rig add API | Setup `/api` route | `POST /api/rig/add -> handleRigAdd` | JSON `{name,gitUrl}` | JSON `SetupResponse` | runs `gt rig add` | implemented | High | `gastown/internal/web/setup.go:69`, `gastown/internal/web/setup.go:177` | Explicit setup-mode route and handler. |
| Setup workspace check API | Setup `/api` route | `POST /api/check-workspace -> handleCheckWorkspace` | JSON `{path}` | JSON `CheckWorkspaceResponse` | probes workspace path and rig list | implemented | High | `gastown/internal/web/setup.go:70`, `gastown/internal/web/setup.go:290` | Explicit setup-mode route and handler. |
| Setup launch API | Setup `/api` route | `POST /api/launch -> handleLaunch` | JSON `{path,port}` | JSON `{success,redirect}` | starts new `gt dashboard` process on next port | implemented | High | `gastown/internal/web/setup.go:72`, `gastown/internal/web/setup.go:217`, `gastown/internal/web/setup.go:254` | Starts subprocess and readiness-check loop. |
| Setup status API | Setup `/api` route | `GET /api/status -> handleStatus` | none | JSON `SetupResponse` | checks workspace via `gt status` | implemented | High | `gastown/internal/web/setup.go:74`, `gastown/internal/web/setup.go:354` | Explicit setup-mode route and handler. |
| Proxy command execution | Proxy `/v1` route | `POST /v1/exec -> handleExec` | mTLS-authenticated exec payload | HTTP response from command execution | executes allowlisted binaries under proxy controls | implemented | High | `gastown/internal/proxy/server.go:230` | Route wired in proxy mux. |
| Proxy git passthrough | Proxy `/v1` route | `/v1/git/* -> handleGit` | git HTTP requests | git protocol responses | proxying git operations | implemented | High | `gastown/internal/proxy/server.go:231` | Explicit path prefix + handler. |
| Proxy admin cert revocation | Proxy `/v1` admin route | `POST /v1/admin/deny-cert -> handleDenyCert` | JSON `{serial}` | `204 No Content` or error | updates in-memory deny list for handshake rejection | implemented | High | `gastown/internal/proxy/server.go:286`, `gastown/internal/proxy/server.go:483` | Admin server route and concrete handler logic. |
| Proxy admin cert issuance | Proxy `/v1` admin route | `POST /v1/admin/issue-cert -> handleIssueCert` | JSON `{rig,name,ttl}` | JSON cert/key/CA metadata | issues new client certificate from proxy CA | implemented | High | `gastown/internal/proxy/server.go:287`, `gastown/internal/proxy/server.go:409` | Admin server route and concrete handler logic. |
| Plugin: github-sheriff | Plugin molecule | `gt plugin run github-sheriff` (plugin markdown workflow) | gh auth, repo, open PR checks | creates CI-failure beads + run beads | scans PR checks and emits tracking beads | indicated | Low | `gastown/plugins/github-sheriff/plugin.md:1`, `gastown/plugins/github-sheriff/plugin.md:20` | Capability is strongly declared in plugin molecule docs; execution engine not verified here. |
| Plugin: dolt-archive | Plugin molecule | `gt plugin run dolt-archive` (plugin markdown workflow) | Dolt DBs, backup repo/remotes | archive summaries + escalation on failure | exports JSONL, pushes Git/Dolt backups | indicated | Low | `gastown/plugins/dolt-archive/plugin.md:1`, `gastown/plugins/dolt-archive/plugin.md:20` | Intent is explicit in plugin molecule; runtime execution path not proven in this inventory pass. |
| Plugin: quality-review trend alerts | Plugin molecule | `gt plugin run quality-review` (plugin markdown workflow) | quality-review result wisps | breach mail/escalations + run summary | trend analysis and alerting pipeline | roadmap | Low | `gastown/plugins/quality-review/plugin.md:20`, `gastown/plugins/quality-review/plugin.md:66` | Defined as patrol workflow text; treated as roadmap-level until executed behavior is evidenced. |
| Convoy synthesis notify action | CLI TODO marker | `gt synthesis close <convoy-id>` | convoy ID + optional future notify target | currently no outgoing notification | convoy closes without notify integration | roadmap | Low | `gastown/internal/cmd/synthesis.go:367` | Explicit TODO marks planned-but-missing behavior. |
| Unknown/conflict audit result | Cross-source verification | Compared documented command surfaces vs code-backed routes/commands sampled in this task | docs + code | audit row only | none | unknown/conflict | Medium | `gastown/docs/reference.md:383`, `gastown/internal/cmd/root.go:314`, `gastown/internal/web/api.go:108`, `gastown/internal/web/setup.go:64`, `gastown/internal/proxy/server.go:229` | No concrete same-precedence contradiction was proven during this pass; placeholder row records that `unknown/conflict` was explicitly checked and none were confirmed. |
