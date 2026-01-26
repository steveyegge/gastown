# Gas Town Codebase Structure

## Overview
The Gas Town project is a Go‑based multi‑agent orchestration platform. Its source tree is organized into a handful of high‑level packages under `internal/` and a small set of command‑line tools in `cmd/gt`.

The core concepts that the code implements are:

| Concept | Directory | Responsibility |
|---------|-----------|----------------|
| **Mayor** – the AI coordinator | `internal/mayor` | High‑level orchestration, creates convoys, spawns agents.
| **Polecat** – worker agents | `internal/polecat` | Ephemeral workers that run tasks in hooks.
| **Hook** – persistent state | `internal/daemon` | Git worktree helpers, lifecycle hooks.
| **Convoy** – work tracking | `internal/convoy` | Groups beads (issues) and tracks progress.
| **Beads** – issue tracking | `internal/beads` | Stores bead data in a git‑backed ledger.
| **Rig** – project container | `internal/rig` | Wraps a git repo and manages its agents.
| **Session** – runtime context | `internal/session` | Manages identity, startup, state.
| **Runtime** – AI runtime abstraction | `internal/runtime` | Interfaces to Claude/Codex/others.
| **Daemon** – system services | `internal/daemon` | Signal handling, lifecycle.
| **Doctor** – health checks | `internal/doctor` | Checks for stale binaries, branches, etc.
| **Shell** – command execution | `internal/shell` | Executes shell commands for agents.
| **Web** – HTTP helpers | `internal/web` | Used for templates and fetchers.
| **UI/TUI** – terminal UI | `internal/ui`, `internal/tui` | Provides pager, markdown rendering, feed UI.
| **Swarm** – agent pooling | `internal/swarm` | Pooling of worker instances.
| **Suggest** – command suggestion | `internal/suggest` | Generates suggestions for incomplete commands.

## Key Directories & Files

### `cmd/gt`
The CLI entry point. Exposes commands such as `gt install`, `gt rig`, `gt sling`, `gt mayor`, etc. The tests under `cmd/gt` validate CLI behavior.

### `internal/beads`
Implements the Beads ledger, bead types, and helper functions. Key files:
- `beads.go`, `beads_channel.go`, `beads_queue.go` – core bead structures.
- `molecule.go`, `molecule_test.go` – handle grouping of beads.
- `catalog.go`, `catalog_test.go` – manage bead catalog.

### `internal/convoy`
Handles convoys: grouping of beads, state transitions, and reporting. Notable files: `convoy.go`, `convoy_test.go`, `convoy_lifecycle.md`.

### `internal/polecat`
Spawns, monitors, and cleans up polecat workers. Contains `manager.go` and `namepool.go`.

### `internal/rig`
Wraps a git repository and exposes hooks for agent interaction. Key logic lives in `types.go`, `setuphooks.go`, `overlay.go`.

### `internal/session`
Provides session identity, startup logic, and state persistence. Core files: `identity.go`, `startup.go`, `town.go`.

### `internal/runtime`
Abstraction over AI runtimes (Claude, Codex, etc.). `runtime.go` defines the interface and helper constructors.

### `internal/daemon`
Signal handling, hook lifecycle, and other background tasks. Includes `signals_unix.go` and `convoy_watcher_test.go`.

### `internal/doctor`
Health‑check utilities: stale binary checks, branch checks, etc.

### `internal/tui`
Terminal UI components – feeds, convoys, and markdown rendering. Files: `feed/`, `convoy/`, `feed/view.go`.

## Build & Test
Run `make test` or `go test ./...` to execute the full test suite. The Makefile includes common targets:
- `make install` – install CLI
- `make build` – compile binary
- `make lint` – run linters
- `make vet` – static analysis

## Extending the System
When adding new functionality, follow these guidelines:
1. **Core logic**: place in `internal/<package>/` following existing patterns.
2. **CLI**: add commands under `cmd/gt` and corresponding tests.
3. **Bead types**: define new bead structs in `internal/beads/` and register them via `beads.Register`.
4. **Convoy handling**: update convoy lifecycle if new states are required.
5. **Testing**: write unit tests for new packages; use mocks for external dependencies.
6. **Documentation**: update relevant `docs/` files and README.

---
For a deep dive into each package, refer to the corresponding design docs under `docs/design/`.
