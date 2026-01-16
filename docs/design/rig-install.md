# Rig Add Shorthand And .gt/rig.toml

This document defines a shorthand for adding rigs to a town, along with a
repository manifest (`.gt/rig.toml`) that standardizes crew defaults, naming,
remotes, and setup/update behavior across towns.

## Goals

- Provide a one-liner to install a rig plus a crew workspace.
- Reuse rig conventions (crew/polecat settings, beads prefix, upstreams).
- Capture upstream vs origin (fork vs owner) and configure remotes correctly.
- Offer a reliable rig-level setup/update behavior (self-update).
- Keep current PR submission behavior unchanged by default.

## Non-Goals

- Replace `gt rig add` or `gt rig quick-add` (they remain supported).
- Change default PR submission workflows.
- Require forks for non-owners (prompt, don't enforce).

## Terminology

- **Upstream**: Canonical repo (e.g., `steveyegge/gastown`).
- **Origin**: User's writable fork or local remote.
- **Manifest**: `.gt/rig.toml` file in the repo.

## CLI Overview

### `gt rig add`

Adds a rig and applies manifest defaults when `.gt/rig.toml` is present (unless `--ignore-manifest`).

Examples:

```bash
gt rig add gastown https://github.com/steveyegge/gastown.git --crew $USER
gt rig add beads https://github.com/steveyegge/beads.git --crew $USER
gt rig add myproject https://github.com/acme/myproject.git --crew alice
gt rig add https://github.com/acme/myproject.git --crew alice
gt rig add . --crew alice                 # local repo (quick-add)
```

Core flags:

- `--crew <name>`: Create ONLY these crew workspaces (overrides manifest crew list).
- `--no-crew`: Skip crew creation entirely.
- `--start`: Autostart all created crew sessions.
- `--ignore-manifest`: Ignore `.gt/rig.toml` entirely (no defaults, no setup, no crew list).
- `--yes`: Non-interactive (accept defaults, skip prompts).
- `--prefix <p>`: Override beads prefix.
- `--branch <name>`: Override default branch.
- `--local-repo <path>`: Use local reference repo for cloning.
- `--upstream <url>`: Explicitly set upstream remote.
- `--origin <url>`: Explicitly set origin remote.
- `--fork <prompt|require|never>`: Fork handling policy (default: `prompt`).

`gt rig quick-add` should delegate to `gt rig add .` so it gains
the same manifest and fork logic.

### `gt rig update`

Check or update a rig against upstream/origin.

Examples:

```bash
gt rig update gastown --check
gt rig update gastown --pull
gt rig update gastown --pull --ignore-manifest
```

Behavior:
- `--check`: fetch and report ahead/behind vs configured default branch.
- `--pull`: update mayor/refinery clones (no crew updates by default).
- `--pull` runs manifest setup by default; use `--ignore-manifest` to skip.
- If neither flag is provided, default to `--check`.

## Presets

Built-in presets for common rigs:

| Preset | Upstream | Default Setup Command |
|--------|----------|-------------------------|
| gastown | `https://github.com/steveyegge/gastown.git` | `go install ./cmd/gt` |
| beads | `https://github.com/steveyegge/beads.git` | `go install ./cmd/bd` |

Presets are shorthand for a manifest (see below) and still honor CLI overrides.

## Manifest: `.gt/rig.toml`

The manifest is optional. If present, it provides rig defaults and the setup
command. TOML aligns with existing usage (formulas and recipes).

Example:

```toml
version = 1

[rig]
name = "gastown"
prefix = "gt"
default_branch = "main"

[git]
upstream = "https://github.com/steveyegge/gastown.git"
fork_policy = "prompt" # prompt|require|never

[setup]
command = "go install ./cmd/gt"
workdir = "."

[settings]
path = ".gt/rig-settings.json"

[[crew]]
name = "max"
agent = "codex"
model = "gpt-5"
account = "work"
branch = true

[[crew]]
name = "alex"
agent = "claude"
account = "personal"
```

### Field Semantics

- `rig.name`: default rig name (sanitized if needed).
- `rig.prefix`: default beads prefix, used only when no tracked `.beads/` exists.
- `rig.default_branch`: fallback if remote default can't be detected.
- `git.upstream`: canonical repo URL (used for update checks).
- `git.fork_policy`: how to prompt for fork (`prompt` default).
- `setup.command`: command run after `gt rig add` or `gt rig update --pull` unless `--ignore-manifest`.
- `setup.workdir`: relative path inside repo (default: repo root).
- `settings.path`: optional JSON file copied into `<rig>/settings/config.json`.
- `crew`: list of crew entries (created but not started).

Crew entry fields:

- `crew.name` (required): crew workspace name.
- `crew.agent` (optional): agent alias or built-in preset (e.g., `codex`, `claude`).
- `crew.model` (optional): model name for the agent (appended to args when supported).
- `crew.account` (optional): account handle for the agent runtime.
- `crew.branch` (optional): `true` to create `crew/<name>` branch or a string to set an explicit branch.
- `crew.args` (optional): extra CLI args appended when starting the crew session.
- `crew.env` (optional): environment variables to set when starting the session.

### Crew Behavior

- If the manifest includes `[[crew]]` entries, those crew are created (not started).
- If `--crew` is provided, only those crew are created. If a provided name
  matches a manifest entry, its config is used; otherwise defaults apply.
- `--no-crew` disables crew creation entirely.
- `--start` autostarts all crew created by the command.
- Model support is determined by the agent preset in `settings/agents.json`.
  If `supports_model` is true, append `model_flag` (or `--model` if unset).
- If `crew.model` is set but the agent does not support models, warn and skip it
  (users can still pass custom flags via `crew.args`).

## Fork Workflow

When upstream is `steveyegge/gastown` or `steveyegge/beads` and the GitHub
user is not `steveyegge`, `gt rig add` should:

1. Detect if a fork exists (`gh repo view <user>/<repo>`).
2. Prompt to use existing fork, create a fork, or continue read-only.
3. Configure remotes:
   - `upstream` = canonical repo
   - `origin` = user fork (or upstream if owner)

If forked, configure beads sync to pull from upstream:

```bash
bd config set sync.remote upstream
```

## Data Model Changes

Extend rig metadata so update/fork info persists:

- `<rig>/config.json`: store `git.upstream`, `git.origin`, `setup.command`.
- `mayor/rigs.json`: store upstream/origin for routing and future `gt crew add`.
- `crew/<name>/state.json`: store default crew start config (agent, model, account, args, env).
- `settings/agents.json`: add optional `model_flag` and `supports_model` for agent presets.

Back-compat: if these fields are missing, assume `origin` only.

## Error Handling

- If `gh` is missing, fork prompting should fall back to a read-only flow.
- If the manifest is missing or invalid, proceed with defaults and warn.
- If `setup.command` fails, print the command and exit non-zero.

## Testing

- Integration test: `gt rig add` with a local repo containing `.gt/rig.toml`.
- Integration test: fork flow using a local "upstream + fork" repo pair.
- Verify remotes, settings copy, crew creation, and update check output.

## Compatibility Notes

- `gt rig add` remains the primary command; manifest handling is additive.
- `gt rig quick-add` becomes a thin wrapper over `gt rig add`.
- PR submission defaults remain unchanged; optional `gt rig submit` can be added later.
