# Everything Is Beads

**Epic:** gt-pozvwr
**Status:** In Progress (Phase 1)
**Author:** gastown/crew/slack, gastown/crew/config_beads
**Date:** 2026-02-05

## Goal

Move all gastown state into beads, leaving nothing in filesystem configuration
except:

- **Secrets** — supplied as filename arguments to CLI or environment variables
- **Environment variables** — runtime control, debug flags, account selection
- **Git clones** — inherently filesystem (but metadata about them lives in beads)

**Principle:** Beads is the source of truth. Filesystem is a cache. If the
filesystem is blown away, `gt prime` (or `gt crew start`) rebuilds it from beads.

## Multi-Town Design

Multiple towns (gt11, gt12) can share a single Dolt database. Config beads use
a compound scope hierarchy to avoid collisions:

```
Scope hierarchy:
  global ("*")
    └── town (gt11)
          └── rig (gt11/gastown)
                └── role (crew)
                      └── agent (slack)
```

### Rig Field Convention

The `rig` field on config beads encodes scope:

| Scope | `rig` field | Labels |
|-------|-------------|--------|
| Global | `*` | `scope:global` |
| Town | `gt11` | `town:gt11` |
| Rig | `gt11/gastown` | `town:gt11`, `rig:gastown` |
| Role | `gt11/gastown` | `town:gt11`, `rig:gastown`, `role:crew` |
| Agent | `gt11/gastown` | `town:gt11`, `rig:gastown`, `agent:slack` |

The `rig` field is always non-empty. `"*"` is the global sentinel.

## Config Bead Type

### Schema

A config bead is a regular Issue with enforced mandatory fields:

| Field | Required | Purpose |
|-------|----------|---------|
| `id` | Yes | `hq-cfg-<slug>` — all config in HQ prefix for cross-town visibility |
| `type` | Yes | `config` |
| `title` | Yes | Human-readable name |
| `rig` | Yes | Scope: `"*"`, `"gt11"`, `"gt11/gastown"` |
| `metadata` | Yes | The config payload as valid JSON (non-empty) |
| `config:<category>` label | Yes | At least one — declares config kind |
| Scope label | Yes | At least one of `scope:global`, `town:*`, `rig:*` |
| `description` | Recommended | Documents what this config does |

### Categories

The `config:<category>` label organizes config beads:

- `config:identity` — town/rig identity (replaces town.json, rig entries in rigs.json)
- `config:claude-hooks` — Claude Code hook definitions (replaces .claude/settings.json)
- `config:mcp` — MCP server configurations (replaces mcp.json)
- `config:rig-registry` — rig registration (replaces rigs.json entries)
- `config:agent-preset` — agent runtime presets (replaces agent configs in settings)
- `config:role-definition` — role definitions (replaces role TOML files)
- `config:slack-routing` — Slack channel routing (replaces slack.json)
- `config:accounts` — non-secret account config (replaces accounts.json)
- `config:daemon` — daemon patrol settings (replaces daemon.json)
- `config:messaging` — mail routing, queues, announces
- `config:escalation` — escalation routes and contacts

### Type Enforcement

Beads enforces mandatory fields for `type=config` at the storage layer.
This prevents malformed config beads from entering the system regardless of
which tool creates them (gt, bd CLI, daemon RPC).

Registration during `gt init`:

```bash
bd type define config \
  --required-field rig \
  --required-field metadata \
  --required-label "config:*" \
  --description "Configuration beads require scope and payload"
```

## Materialization: Spawn Writes, Beads Is Truth

### The Bootstrap Problem

Claude Code needs `.claude/settings.json` on disk before it can execute hooks.
But hook config lives in beads. Solution: **the spawn command materializes
settings before launching Claude Code.**

### Materialization Flow

```
gt crew start gastown slack
│
├─ Query: bd list --type=config --label=config:claude-hooks --json
│  Returns all hook config beads
│
├─ Filter to applicable scopes:
│  Agent: gastown/crew/slack in town gt11
│  Matches: scope:global, town:gt11, rig:gastown, role:crew, agent:slack
│
├─ Score by specificity:
│  0: scope:global (no role)
│  1: scope:global + role:crew
│  2: town:gt11 + rig:gastown
│  3: town:gt11 + rig:gastown + role:crew
│  4: town:gt11 + rig:gastown + agent:slack
│
├─ Deep-merge (less specific → more specific)
│
├─ Write: gastown/crew/slack/.claude/settings.json
│
└─ Launch Claude Code
```

### Merge Strategy

Hook arrays use **APPEND** (more specific adds to less specific):

```
base.PreCompact = [A]
crew.PreCompact = [B]
result.PreCompact = [A, B]     ← both fire
```

Top-level keys use **OVERRIDE** (more specific wins):

```
base.editorMode = "normal"
crew.editorMode = "vim"
result.editorMode = "vim"      ← crew wins
```

Explicit **NULL suppresses** inherited hooks:

```
base.PostToolUse = [A]
crew.PostToolUse = null
result.PostToolUse = []         ← crew suppresses it
```

### What Gets Materialized

| Config Category | Materializes To | When |
|----------------|-----------------|------|
| `config:claude-hooks` | `.claude/settings.json` | Spawn time |
| `config:mcp` | `.mcp.json` | Spawn time |
| `config:role-definition` | In-memory (used by gt prime) | Session start |
| `config:identity` | In-memory (used by gt commands) | On demand |
| `config:rig-registry` | In-memory (used by gt rig commands) | On demand |

## Concrete Examples

### Town Identity (replaces town.json)

```json
{
  "id": "hq-cfg-town-gt11",
  "type": "config",
  "title": "Town: gt11",
  "rig": "gt11",
  "labels": ["town:gt11", "config:identity"],
  "metadata": {
    "name": "gt11",
    "owner": "steveyegge",
    "public_name": "Gas Town 11"
  }
}
```

### Rig Registry Entry (replaces one entry in rigs.json)

```json
{
  "id": "hq-cfg-rig-gt11-gastown",
  "type": "config",
  "title": "Rig: gastown",
  "rig": "gt11/gastown",
  "labels": ["town:gt11", "rig:gastown", "config:rig-registry"],
  "metadata": {
    "git_url": "git@gitlab.com:.../gastown.git",
    "prefix": "gt"
  }
}
```

### Claude Hooks - Global Base

```json
{
  "id": "hq-cfg-hooks-base",
  "type": "config",
  "title": "Claude Hooks: base",
  "rig": "*",
  "labels": ["scope:global", "config:claude-hooks"],
  "metadata": {
    "editorMode": "normal",
    "hooks": {
      "PreCompact": [{"command": "gt prime --hook"}]
    }
  }
}
```

### Claude Hooks - Crew Role

```json
{
  "id": "hq-cfg-hooks-crew",
  "type": "config",
  "title": "Claude Hooks: crew",
  "rig": "*",
  "labels": ["scope:global", "role:crew", "config:claude-hooks"],
  "metadata": {
    "hooks": {
      "SessionStart": [
        {"command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook && gt nudge deacon session-started"}
      ],
      "UserPromptSubmit": [
        {"command": "_stdin=$(cat) && echo \"$_stdin\" | gt nudge drain --quiet && echo \"$_stdin\" | gt mail check --inject && echo \"$_stdin\" | bd decision check --inject && echo \"$_stdin\" | gt decision turn-clear"}
      ],
      "Stop": [
        {"command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && _stdin=$(cat) && echo \"$_stdin\" | gt costs record && echo \"$_stdin\" | gt decision turn-check && gt advice run --trigger=session-end --quiet"}
      ]
    }
  }
}
```

### Claude Hooks - Polecat Role (simpler)

```json
{
  "id": "hq-cfg-hooks-polecat",
  "type": "config",
  "title": "Claude Hooks: polecat",
  "rig": "*",
  "labels": ["scope:global", "role:polecat", "config:claude-hooks"],
  "metadata": {
    "hooks": {
      "SessionStart": [
        {"command": "gt prime --hook && gt nudge deacon session-started"}
      ],
      "Stop": [
        {"command": "_stdin=$(cat) && echo \"$_stdin\" | gt costs record && echo \"$_stdin\" | gt decision turn-check --soft"}
      ]
    }
  }
}
```

### MCP Servers - Global

```json
{
  "id": "hq-cfg-mcp-global",
  "type": "config",
  "title": "MCP Servers",
  "rig": "*",
  "labels": ["scope:global", "config:mcp"],
  "metadata": {
    "mcpServers": {
      "playwright": {"command": "npx", "args": ["-y", "@playwright/mcp@latest"]}
    }
  }
}
```

## Implementation Phases

### Phase 1: Foundation (In Progress)

| Task | Status | Description |
|------|--------|-------------|
| gt-pozvwr.6 | In progress (beads/obsidian) | Type schema enforcement in beads |
| gt-pozvwr.7 | Done (gastown/furiosa) | Register `config` bead type in gastown |
| gt-pozvwr.8 | In progress (gastown/nux) | Config bead CRUD helpers with merge |
| gt-pozvwr.22.1 | Closed | Label-AND queries (already supported) |
| gt-pozvwr.22.2 | In progress (beads/quartz) | `bd config` subcommand |

### Phase 2: Hooks Materialization

| Task | Description |
|------|-------------|
| gt-pozvwr.9 | Materialize Claude hooks from config beads at spawn |
| gt-pozvwr.10 | Materialize MCP config from config beads at spawn |
| gt-pozvwr.11 | Seed initial hook and MCP config beads from current files |

### Phase 3: Identity

| Task | Description |
|------|-------------|
| gt-pozvwr.12 | Town identity config beads |
| gt-pozvwr.13 | Rig registry config beads |
| gt-pozvwr.14 | Account config beads (non-secret parts) |
| gt-pozvwr.15 | Daemon patrol config beads |

### Phase 4: Roles

| Task | Description |
|------|-------------|
| gt-pozvwr.16 | Role definitions as config beads |
| gt-pozvwr.17 | Agent presets as config beads |
| gt-pozvwr.18 | Slack routing and messaging config beads |

### Phase 5: Cleanup

| Task | Description |
|------|-------------|
| gt-pozvwr.19 | Remove filesystem config files |
| gt-pozvwr.20 | Update gt init and gt rig add for beads-first bootstrap |
| gt-pozvwr.21 | gt config CLI for managing config beads |

## What Stays on Filesystem

| Item | Why |
|------|-----|
| Secrets (API tokens, Slack tokens) | Security — env vars and 0600 files are correct |
| Git clones / worktrees | Inherently filesystem |
| .claude-flow/ logs | Runtime artifact, not config |
| Agent runtime state (.runtime/, agent.lock) | Too transient for Dolt write frequency |
| CLAUDE.md files | Generated by `gt prime` from templates in code |

## What Stays as Environment Variables

| Variable | Why |
|----------|-----|
| `GT_ROLE`, `GT_RIG`, `GT_ROOT` | Runtime identity, set at spawn |
| `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL` | Secrets |
| `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` | Secrets |
| `GT_DEBUG`, `GT_DEBUG_*` | Ephemeral debug flags |
| `GASTOWN_ENABLED`, `GASTOWN_DISABLED` | Runtime overrides |
| `GT_ACCOUNT` | Account selection |
| `BEADS_*` | Beads runtime config |

## Multi-Town Sync

Config beads use the `hq-` prefix and live in town-level beads. When towns
share a Dolt database:

- Global config beads (`scope:global`) are visible to all towns
- Town-scoped config beads (`town:gt11`) are visible only to that town's agents
- Rig-scoped config beads are further filtered by rig name
- No prefix collisions — `hq-cfg-*` IDs include the town name for town-scoped beads
- Dolt cell-level merge prevents conflicts when different towns update different beads
- Same-bead conflicts resolved by last-write-wins (standard Dolt behavior)

Config beads MUST NOT be ephemeral (wisps). They are persistent issues that
survive the full JSONL export/import round-trip.

## Impact Summary

| Before | After |
|--------|-------|
| 21 hand-edited .claude/settings.json files | ~5 config beads (base + per-role + overrides) |
| 19 identical mcp.json files | 1 global config bead |
| 5 town-level JSON files | 5 config beads |
| Per-rig settings JSON | Config beads with rig labels |
| 7 role TOML files with 3-level override | Config beads with role labels + merge |
| Manual consistency across rigs | Automatic inheritance via specificity merge |
| New rig = copy config files | New rig = inherits global + role config automatically |
| New town = override only what differs | New town = override only what differs |
