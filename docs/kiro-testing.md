# Kiro CLI Test Suite and Usage

## Overview

Gas Town supports Kiro as a built-in agent preset alongside Claude, Gemini, Codex, and others. The Kiro test suite validates that `gt` can build, configure, and start sessions with Kiro CLI independently of Claude Code. The tests span five packages and cover build verification, settings generation, runtime provider selection, wrapper scripts, and end-to-end integration with zero Claude dependencies.

Total: **37 tests** across 5 test files (plus unit tests in `internal/kiro/settings_test.go`).

## Prerequisites

- **Go 1.24.2+** (see `go.mod`)
- **git** (for version ldflags during build)
- Standard POSIX environment (bash, coreutils) for wrapper script tests
- No Kiro CLI binary is needed to run the tests; they validate configuration and settings generation, not live Kiro sessions

## Running the Tests

### Run all project tests

```bash
make test
# or
go test ./...
```

### Run only the Kiro-related test files

```bash
# Build verification (compile, binary, --version, --help, go vet, race detector)
go test ./internal/cmd/ -run "TestGoBuild|TestBinaryIs|TestBinaryVersion|TestBinaryHelp|TestGoVet|TestGoTestRace" -v

# Kiro settings unit tests
go test ./internal/kiro/ -v

# Kiro session tests (preset, roles, hooks, beacon, permissions)
go test ./internal/kiro/ -run "TestKiro" -v

# Runtime provider selection
go test ./internal/runtime/ -run "TestKiro" -v

# Wrapper script validation
go test ./internal/wrappers/ -run "TestKiro|TestAllWrapper" -v

# Kiro-only integration (end-to-end config, session identities, startup commands)
go test ./internal/config/ -run "TestKiro" -v
```

### Run a single test

```bash
go test ./internal/kiro/ -run TestKiroAutonomousSettings -v
go test ./internal/config/ -run TestKiroOnlyRigConfiguration -v
```

### Skip slow build tests

Build verification tests compile the binary and are slower. Skip them with `-short`:

```bash
go test ./internal/cmd/ -short -v
```

## Test Coverage

### `internal/cmd/build_verification_test.go` (6 tests)

Validates that the `gt` binary compiles and functions correctly.

| Test | What it verifies |
|------|------------------|
| `TestGoBuildSucceeds` | `go build ./cmd/gt/` compiles without errors |
| `TestBinaryIsProduced` | Built binary exists, has non-zero size, is executable |
| `TestBinaryVersionFlag` | `gt --version` exits cleanly and prints version info |
| `TestBinaryHelpFlag` | `gt --help` prints usage with command groups |
| `TestGoVetPasses` | `go vet ./...` reports no issues |
| `TestGoTestRace` | `go build -race` compiles (CGO + race instrumentation check) |

**Why it matters:** Catches compilation regressions. The `buildGTWithLdflags` helper sets `-X ...cmd.BuiltProperly=1` to bypass the persistent-pre-run check, matching real build behavior.

### `internal/kiro/settings_test.go` (7 unit tests)

Unit tests for the `kiro` package's settings generation.

| Test | What it verifies |
|------|------------------|
| `TestRoleTypeFor` | Role classification: polecat/witness/refinery/deacon = autonomous; mayor/crew/unknown = interactive |
| `TestEnsureSettingsAt_CreatesFile` | Settings file is created with 0600 permissions and non-empty content |
| `TestEnsureSettingsAt_SkipsExisting` | Existing settings files are not overwritten |
| `TestEnsureSettingsAt_CreatesDirectory` | Nested directory structures are created as needed |
| `TestEnsureSettingsAt_AutonomousTemplate` | Autonomous template includes `gt mail check --inject` in SessionStart |
| `TestEnsureSettingsAt_InteractiveTemplate` | Interactive template includes `gt prime --hook` |
| `TestEnsureSettingsForRoleAt` | Convenience wrapper creates settings for both polecat and crew roles |

### `internal/kiro/session_test.go` (10 tests)

Higher-level tests for Kiro session configuration and lifecycle.

| Test | What it verifies |
|------|------------------|
| `TestKiroPresetConfiguration` | Kiro preset has command `kiro`, args `--trust-all-tools`, hooks support, `--resume` flag, process names `[kiro, node]` |
| `TestKiroAutonomousRoles` | polecat, witness, refinery, deacon are classified as autonomous |
| `TestKiroInteractiveRoles` | mayor, crew are classified as interactive |
| `TestKiroAutonomousSettings` | Autonomous settings have `gt mail check --inject` in SessionStart hook |
| `TestKiroInteractiveSettings` | Interactive settings have `gt mail check --inject` in UserPromptSubmit (not SessionStart) |
| `TestKiroSettingsNoClaudeReferences` | Neither template contains "claude" or ".claude" (case-insensitive) |
| `TestKiroHookLifecycle` | Both templates define all required hook events: SessionStart, UserPromptSubmit, PreCompact, Stop |
| `TestKiroSettingsFilePermissions` | Settings files are written with 0600 permissions |
| `TestKiroSessionBeacon` | Startup beacon messages format correctly for assigned/cold-start/start topics |
| `TestKiroNonInteractiveConfig` | Non-interactive config has `-p` prompt flag and `--output json` output flag |

**Why it matters:** Proves that Kiro operates independently of Claude. The `NoClaudeReferences` test is a guard rail preventing accidental coupling.

### `internal/runtime/kiro_runtime_test.go` (7 tests)

Tests the runtime layer's provider dispatch for Kiro.

| Test | What it verifies |
|------|------------------|
| `TestKiroHooksProviderSelection` | `EnsureSettingsForRole` with provider "kiro" creates `.kiro/settings.json` |
| `TestKiroHooksProviderSelection_NotClaude` | Kiro provider does NOT create `.claude/` directory |
| `TestKiroFallbackCommand` | When hooks provider is "kiro", `StartupFallbackCommands` returns nil (hooks handle it) |
| `TestKiroFallbackCommand_NoHooks` | Without hooks, fallback commands include `gt prime`, `gt mail check --inject` (for autonomous roles), and `gt nudge deacon` |
| `TestKiroEnsureSettings` | Per-role settings: autonomous roles get mail inject in SessionStart; interactive roles do not |
| `TestKiroSettingsDirectory` | Settings land in `.kiro/settings.json` with 0600 permissions |
| `TestKiroSettingsDirectory_CustomDir` | Custom directory/filename (e.g. `.custom-kiro/custom-settings.json`) is respected |

### `internal/wrappers/wrapper_kiro_test.go` (6 tests)

Validates the `gt-kiro` wrapper script.

| Test | What it verifies |
|------|------------------|
| `TestKiroWrapperScriptExists` | `gt-kiro` is present in the embedded scripts filesystem |
| `TestKiroWrapperScriptContent` | Script has bash shebang, `gastown_enabled` check, `GASTOWN_DISABLED`/`GASTOWN_ENABLED` env vars, calls `gt prime`, runs `exec kiro "$@"` |
| `TestKiroWrapperScriptContent_NotOtherAgents` | Script does not `exec codex`, `exec opencode`, or `exec claude` |
| `TestKiroWrapperInstallation` | Script installs to disk with executable permissions and matching content |
| `TestKiroWrapperInWrapperList` | `gt-kiro` appears in the embedded scripts directory listing |
| `TestAllWrapperScriptsHaveConsistentStructure` | `gt-kiro`, `gt-codex`, and `gt-opencode` all share the same structural pattern (shebang, `gastown_enabled`, `gt prime`, `set -e`) |

### `internal/config/kiro_integration_test.go` (8 tests)

End-to-end integration tests proving Gas Town works with Kiro as the sole agent.

| Test | What it verifies |
|------|------------------|
| `TestKiroOnlyRigConfiguration` | Town with `default_agent: kiro` loads correctly; resolved config has command `kiro`, args `--trust-all-tools`, no Claude references in serialized JSON |
| `TestKiroRigRuntimeConfig` | `normalizeRuntimeConfig` fills kiro defaults: provider "kiro", hooks in `.kiro/settings.json`, tmux process names `[kiro, node]`, ready delay 8000ms, prompt mode "none", instructions file `AGENTS.md` |
| `TestKiroSessionIdentitiesAllRoles` | Mail addresses and tmux session names resolve correctly for all 6 roles (mayor, deacon, witness, refinery, crew, polecat), no Claude references |
| `TestKiroSettingsGenerationAllRoles` | Settings generation for all roles produces valid JSON with correct hooks, `gt costs record` in Stop, no Claude references |
| `TestKiroAgentResolutionWithoutClaude` | With only `kiro` in PATH (no `claude`), agent resolution picks kiro |
| `TestKiroCustomAgentOverride` | Custom `kiro-fast` agent with extra flags (`--turbo`, custom path) resolves correctly; built-in kiro preset remains accessible |
| `TestKiroAgentEnvAllRoles` | `AgentEnv` generates correct `GT_*` variables per role; `CLAUDE_CONFIG_DIR` never appears |
| `TestKiroStartupCommandNoClaudeArtifacts` | Startup commands for all rig-level roles contain `kiro` and `--trust-all-tools` but zero Claude artifacts (`CLAUDE_SESSION_ID`, `CLAUDE_CONFIG_DIR`, `--dangerously-skip-permissions`) |

## Building gt for Kiro

### Standard build with ldflags

```bash
make build
```

This runs:

```bash
go build -ldflags "\
  -X github.com/steveyegge/gastown/internal/cmd.Version=$(git describe --tags --always --dirty) \
  -X github.com/steveyegge/gastown/internal/cmd.Commit=$(git rev-parse --short HEAD) \
  -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1" \
  -o gt ./cmd/gt
```

The `BuiltProperly=1` ldflag is required. Without it, `gt` refuses to run (persistent-pre-run check).

### Install to `~/.local/bin`

```bash
make install
```

### Build with race detector

```bash
go build -race -o gt-race ./cmd/gt
```

## Starting a Kiro Session

### Configure a rig to use Kiro

Set the town or rig default agent to `kiro`:

```json
// town settings (settings/config.json)
{
  "type": "town-settings",
  "version": 1,
  "default_agent": "kiro"
}

// rig settings (rig/settings/config.json)
{
  "type": "rig-settings",
  "version": 1,
  "agent": "kiro"
}
```

### The gt-kiro wrapper

The `gt-kiro` wrapper script (installed alongside `gt`) intercepts `kiro` invocations to inject Gas Town context:

1. Checks if Gas Town is enabled (`GASTOWN_ENABLED` env var or `~/.local/state/gastown/state.json`)
2. Runs `gt prime` to sync context (hooks, instructions, mail)
3. Execs `kiro "$@"` to hand off to the real Kiro CLI

The wrapper is embedded in the binary at `internal/wrappers/scripts/gt-kiro` and installed by `gt install`.

### Kiro preset defaults

When `gt` resolves agent config for Kiro, it uses these defaults (defined in `internal/config/agents.go`):

| Setting | Value |
|---------|-------|
| Command | `kiro` |
| Args | `--trust-all-tools` |
| Process names | `kiro`, `node` |
| Resume flag | `--resume` (flag style) |
| Hooks provider | `kiro` |
| Hooks dir | `.kiro` |
| Settings file | `settings.json` |
| Tmux ready delay | 8000ms |
| Prompt mode | `none` (delay-based detection; Kiro TUI uses box-drawing characters that break prompt prefix matching) |
| Non-interactive | `-p` for prompt, `--output json` for output |

### Custom Kiro agents

Override defaults in town settings:

```json
{
  "type": "town-settings",
  "version": 1,
  "default_agent": "kiro",
  "agents": {
    "kiro-fast": {
      "provider": "kiro",
      "command": "/opt/kiro/bin/kiro",
      "args": ["--trust-all-tools", "--turbo"]
    }
  }
}
```

Then reference it in rig settings: `"agent": "kiro-fast"`.

## Architecture

### Autonomous vs. Interactive Roles

Gas Town classifies roles into two types that determine how hooks fire:

**Autonomous** (polecat, witness, refinery, deacon): These roles operate without human input. `gt mail check --inject` runs in the **SessionStart** hook so the agent receives work assignments immediately on startup.

**Interactive** (mayor, crew): These roles are human-guided. `gt mail check --inject` runs in the **UserPromptSubmit** hook instead, so mail is checked each time the user submits a prompt rather than flooding the session on startup.

The classification is implemented in `internal/kiro/settings.go`:

```go
func RoleTypeFor(role string) RoleType {
    switch role {
    case "polecat", "witness", "refinery", "deacon":
        return Autonomous
    default:
        return Interactive
    }
}
```

### Hook Lifecycle

Both autonomous and interactive settings define four hook events:

| Event | Purpose |
|-------|---------|
| **SessionStart** | Runs once when the Kiro session starts. Autonomous roles: `gt prime --hook && gt mail check --inject && gt nudge deacon session-started`. Interactive roles: `gt prime --hook && gt nudge deacon session-started`. |
| **UserPromptSubmit** | Runs each time the user submits a prompt. Both roles: `gt mail check --inject`. |
| **PreCompact** | Runs before context compaction. Both roles: `gt prime --hook` (re-syncs context). |
| **Stop** | Runs when the session ends. Both roles: `gt costs record` (records token usage). |

All hook commands prepend `export PATH="$HOME/go/bin:$HOME/bin:$PATH"` to ensure `gt` is found.

### Settings Templates

Settings are stored as embedded JSON files in `internal/kiro/config/`:

- `settings-autonomous.json` -- used for polecat, witness, refinery, deacon
- `settings-interactive.json` -- used for mayor, crew

The `EnsureSettingsAt` function copies the appropriate template to `.kiro/settings.json` in the working directory. If the file already exists, it is not overwritten. Files are created with 0600 permissions.

### Runtime Provider Selection

The runtime layer (`internal/runtime/runtime.go`) dispatches settings generation based on the hooks provider:

```go
switch rc.Hooks.Provider {
case "claude":
    return claude.EnsureSettingsForRoleAt(...)
case "opencode":
    return opencode.EnsurePluginAt(...)
case "kiro":
    return kiro.EnsureSettingsForRoleAt(...)
}
```

When the hooks provider is active (not "none"), `StartupFallbackCommands` returns nil because hooks handle the lifecycle. When hooks are unavailable, fallback commands approximate the same behavior by sending `gt prime`, `gt mail check --inject` (for autonomous roles), and `gt nudge deacon session-started` via tmux.

### Key Source Files

| File | Purpose |
|------|---------|
| `internal/kiro/settings.go` | Role classification, settings template selection, `EnsureSettings*` functions |
| `internal/kiro/config/settings-autonomous.json` | Hook definitions for autonomous roles |
| `internal/kiro/config/settings-interactive.json` | Hook definitions for interactive roles |
| `internal/config/agents.go` | `AgentKiro` preset definition (command, args, process names, etc.) |
| `internal/runtime/runtime.go` | Provider dispatch (`EnsureSettingsForRole`, `StartupFallbackCommands`) |
| `internal/wrappers/scripts/gt-kiro` | Wrapper script for intercepting `kiro` invocations |
| `templates/agents/kiro.json.tmpl` | Agent template for kiro (used by template provisioning) |
| `templates/agents/kiro-models.json` | Model presets stub (model selection TBD; delay-based detection) |
