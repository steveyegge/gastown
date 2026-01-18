# Cost Optimization Guide

Gas Town lets you run different LLM models for different roles, enabling significant cost savings without sacrificing quality where it matters.

## Overview

By default, all agents use the same model (Claude Sonnet). But not all roles need the same level of reasoning:

| Role | Typical Tasks | Model Requirements |
|------|---------------|-------------------|
| **Mayor** | Complex planning, multi-agent coordination | High reasoning |
| **Crew** | Feature implementation, code review | High reasoning |
| **Polecat** | Scoped task execution | Medium reasoning |
| **Witness** | Health checks, status monitoring | Low reasoning |
| **Refinery** | Merge operations, conflict resolution | Low-medium reasoning |
| **Deacon** | Routine patrol, scheduling | Low reasoning |

Using cheaper models for simpler roles can reduce costs by 60-80% while maintaining output quality.

## Configuration

Role-to-model mapping is configured via `role_agents` in your settings.

### Town-Level Settings

Configure in `~/gt/settings/config.json` to apply to all rigs:

```json
{
  "type": "town-settings",
  "version": 1,
  "default_agent": "claude",
  "role_agents": {
    "mayor": "claude",
    "crew": "claude",
    "polecat": "claude",
    "witness": "claude-haiku",
    "refinery": "claude-haiku",
    "deacon": "claude-haiku"
  }
}
```

### Rig-Level Override

Configure in `~/gt/<rig>/settings/config.json` to override for a specific rig:

```json
{
  "type": "rig-settings",
  "version": 1,
  "role_agents": {
    "polecat": "claude-haiku"
  }
}
```

Rig settings take precedence over town settings.

## Built-in Agent Presets

| Preset | Provider | Use Case |
|--------|----------|----------|
| `claude` | Anthropic | Default, balanced |
| `gemini` | Google | Alternative provider |
| `codex` | OpenAI | Code-focused tasks |
| `cursor` | Cursor | Cursor IDE integration |
| `auggie` | Augment | Augment Code |
| `amp` | Sourcegraph | Amp coding assistant |

## Defining Model-Specific Agents

To use different models (like Haiku or Opus), define custom agents:

```json
{
  "agents": {
    "claude-haiku": {
      "command": "claude",
      "args": ["--model", "haiku", "--dangerously-skip-permissions"]
    },
    "claude-opus": {
      "command": "claude",
      "args": ["--model", "opus", "--dangerously-skip-permissions"]
    }
  }
}
```

Then reference them in `role_agents`.

## Recommended Configurations

### Balanced (Recommended)

Good balance of quality and cost. Uses Sonnet for human-facing work, Haiku for background tasks.

```json
{
  "role_agents": {
    "mayor": "claude",
    "crew": "claude",
    "polecat": "claude",
    "witness": "claude-haiku",
    "refinery": "claude-haiku",
    "deacon": "claude-haiku"
  }
}
```

**Expected savings**: ~40% vs all-Sonnet

### Aggressive Cost Reduction

Maximum savings. Uses Haiku for all automated roles.

```json
{
  "role_agents": {
    "mayor": "claude",
    "crew": "claude",
    "polecat": "claude-haiku",
    "witness": "claude-haiku",
    "refinery": "claude-haiku",
    "deacon": "claude-haiku"
  }
}
```

**Expected savings**: ~70% vs all-Sonnet

**Trade-off**: Polecat task quality may decrease for complex tasks.

### Quality-First

When cost is less important than output quality.

```json
{
  "role_agents": {
    "mayor": "claude-opus",
    "crew": "claude",
    "polecat": "claude",
    "witness": "claude",
    "refinery": "claude",
    "deacon": "claude-haiku"
  }
}
```

**Expected savings**: Minimal (may increase costs)

## Cost Comparison

Approximate per-role costs based on typical usage patterns:

| Role | Tokens/hour | Sonnet Cost | Haiku Cost | Savings |
|------|-------------|-------------|------------|---------|
| Mayor | 50K | $0.45 | $0.06 | 87% |
| Crew | 100K | $0.90 | $0.12 | 87% |
| Polecat (x5) | 200K | $1.80 | $0.24 | 87% |
| Witness | 10K | $0.09 | $0.01 | 89% |
| Refinery | 20K | $0.18 | $0.02 | 89% |

*Costs are illustrative. Actual costs depend on model pricing and usage.*

## Verifying Configuration

Check which agent a role will use:

```bash
# View town settings
cat ~/gt/settings/config.json | jq '.role_agents'

# View rig settings
cat ~/gt/<rig>/settings/config.json | jq '.role_agents'
```

The resolution order is:
1. Rig `role_agents` (if role is defined)
2. Town `role_agents` (if role is defined)
3. Rig `agent` (rig default)
4. Town `default_agent`
5. Built-in default (`claude`)

## Troubleshooting

### Agent not using expected model

Check the resolution order above. Rig settings override town settings.

```bash
# Debug: see effective config for a role
GT_ROLE=witness gt config show
```

### Custom agent not found

Custom agents must be defined in the `agents` field before referencing them in `role_agents`:

```json
{
  "agents": {
    "my-haiku": {
      "provider": "claude",
      "command": "claude",
      "args": ["--model", "haiku", "--dangerously-skip-permissions"]
    }
  },
  "role_agents": {
    "witness": "my-haiku"
  }
}
```

## See Also

- [Reference](../reference.md) - Full command and configuration reference
- [Overview](../overview.md) - Gas Town architecture overview
