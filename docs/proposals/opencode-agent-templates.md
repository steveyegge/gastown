# Proposal: OpenCode Agent Templates System

> **Status**: Draft
> **Author**: Claude (Refinery Investigation)
> **Date**: 2026-01-18
> **Related Issue**: Refinery startup failures with opencode agents

---

## Executive Summary

This proposal introduces a **template system for non-Claude agents** (opencode, amp, codex, gemini) that addresses configuration gaps discovered during refinery debugging. The current system assumes Claude-like behavior for all agents, causing startup failures when agents have different:
- Ready prompt indicators
- Tmux process detection patterns
- Session management protocols
- Hook configurations

---

## Problem Statement

### Observed Issue

When configuring `refinery` to use `opencode-gpt52-codex`, the agent failed to start:

```
Error: starting refinery: waiting for refinery to start: timeout waiting for runtime prompt
```

### Root Cause Analysis

1. **Default RuntimeConfig assumes Claude**:
   ```go
   func DefaultRuntimeConfig() *RuntimeConfig {
       return normalizeRuntimeConfig(&RuntimeConfig{Provider: "claude"})
   }
   ```

2. **ReadyPromptPrefix mismatch**:
   - Claude expects: `"> "` at line start
   - OpenCode shows: `"┃  Ask anything..."` with box-drawing characters
   - After `TrimSpace`, line still starts with `┃`, not matching any prefix

3. **No built-in opencode preset**:
   - Only 6 presets exist: claude, gemini, codex, cursor, auggie, amp
   - OpenCode requires manual configuration in `agents.json`

4. **Missing runtime templates**:
   - No guidance for users configuring non-Claude agents
   - Trial-and-error required to find correct settings

### Current Workaround

Manual configuration in `rig/settings/config.json`:
```json
{
  "runtime": {
    "provider": "opencode",
    "tmux": {
      "ready_prompt_prefix": "",
      "ready_delay_ms": 8000
    }
  }
}
```

---

## Current Architecture

### Agent Resolution Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Agent Resolution                          │
├─────────────────────────────────────────────────────────────┤
│  1. Check explicit override (--agent flag)                  │
│  2. Check rig's role_agents[role]                           │
│  3. Check town's role_agents[role]                          │
│  4. Check rig's agent field                                 │
│  5. Check town's default_agent                              │
│  6. Fallback to "claude"                                    │
└─────────────────────────────────────────────────────────────┘
```

### Configuration Hierarchy

```
town/
├── settings/
│   ├── config.json      # TownSettings (default_agent, role_agents)
│   └── agents.json      # Custom agent definitions
└── <rig>/
    └── settings/
        ├── config.json  # RigSettings (agent override, role_agents)
        └── agents.json  # Rig-specific custom agents
```

### Built-in Agent Presets

| Preset | Command | Key Settings |
|--------|---------|--------------|
| claude | `claude --dangerously-skip-permissions` | ProcessNames: ["node"], ResumeStyle: "flag" |
| gemini | `gemini --approval-mode yolo` | ProcessNames: ["gemini"] |
| codex | `codex --yolo` | ResumeStyle: "subcommand" |
| cursor | `cursor-agent -f` | ProcessNames: ["cursor-agent"] |
| auggie | `auggie --allow-indexing` | ProcessNames: ["auggie"] |
| amp | `amp --dangerously-allow-all --no-ide` | ResumeStyle: "subcommand" |

### Missing: OpenCode Preset

OpenCode is a popular multi-model CLI that supports:
- OpenAI (gpt-5.2, gpt-5.2-codex, codex-1)
- Anthropic (via API)
- Google (gemini-3-pro)
- xAI (grok-code)
- Free models (glm-4.7, minimax, big-pickle)

---

## Proposed Solution

### 1. Add OpenCode Built-in Preset

**File**: `internal/config/agents.go`

```go
var AgentOpenCode = AgentPresetInfo{
    Name:         "opencode",
    Command:      "opencode",
    Args:         []string{},
    ProcessNames: []string{"opencode", "node"},
    SessionIDEnv: "OPENCODE_SESSION_ID",
    ResumeFlag:   "--session",
    ResumeStyle:  "flag",
    NonInteractiveConfig: &NonInteractiveConfig{
        Subcommand: "run",
        OutputFlag: "--format json",
    },
}
```

### 2. Add Agent Templates Directory

**New directory**: `templates/agents/`

```
templates/
├── agents/
│   ├── opencode.json.tmpl      # OpenCode configuration template
│   ├── opencode-models.json    # Available model presets
│   ├── amp.json.tmpl           # Amp configuration template
│   ├── codex.json.tmpl         # Codex configuration template
│   └── README.md               # Template documentation
└── ...existing templates...
```

### 3. OpenCode Template Structure

**File**: `templates/agents/opencode.json.tmpl`

```json
{
  "$schema": "https://gastown.dev/schemas/agent-template-v1.json",
  "name": "opencode-{{.Model}}",
  "description": "OpenCode agent using {{.Provider}}/{{.Model}}",
  "command": "{{.OpenCodePath | default \"/usr/local/bin/opencode\"}}",
  "args": ["-m", "{{.Provider}}/{{.Model}}"],
  "process_names": ["opencode", "node"],
  "session_id_env": "OPENCODE_SESSION_ID",
  "resume_flag": "--session",
  "resume_style": "flag",
  "non_interactive": {
    "subcommand": "run",
    "output_flag": "--format json"
  },
  "tmux": {
    "ready_prompt_prefix": "",
    "ready_delay_ms": {{.ReadyDelayMs | default 8000}},
    "process_names": ["opencode", "node"]
  },
  "hooks": {
    "provider": "none"
  }
}
```

### 4. OpenCode Model Presets

**File**: `templates/agents/opencode-models.json`

```json
{
  "version": 1,
  "models": {
    "openai": {
      "gpt-5.2": {
        "description": "GPT-5.2 chat model",
        "ready_delay_ms": 5000
      },
      "gpt-5.2-codex": {
        "description": "GPT-5.2 Codex for code tasks",
        "ready_delay_ms": 8000
      },
      "codex-1": {
        "description": "Codex-1 extended context",
        "ready_delay_ms": 10000
      }
    },
    "google": {
      "gemini-3-pro-high": {
        "description": "Gemini 3 Pro High quality",
        "ready_delay_ms": 6000
      }
    },
    "xai": {
      "grok-code-fast-1": {
        "description": "Grok Code Fast",
        "ready_delay_ms": 4000
      }
    },
    "github-copilot": {
      "gpt-5.2-codex": {
        "description": "GitHub Copilot GPT-5.2 Codex",
        "ready_delay_ms": 8000
      }
    },
    "opencode": {
      "glm-4.7-free": {
        "description": "GLM 4.7 Free tier (may timeout)",
        "ready_delay_ms": 15000
      },
      "minimax-m2.1-free": {
        "description": "MiniMax M2.1 Free tier",
        "ready_delay_ms": 10000
      },
      "big-pickle": {
        "description": "Big Pickle experimental",
        "ready_delay_ms": 12000
      }
    }
  }
}
```

### 5. Town Initialization Enhancement

**Modify**: `internal/cmd/init.go`

During `gt init`, generate template `agents.json`:

```go
func initAgentTemplates(townRoot string) error {
    templatesDir := filepath.Join(townRoot, "settings", "templates")
    if err := os.MkdirAll(templatesDir, 0755); err != nil {
        return err
    }

    // Copy embedded templates
    templates := []string{
        "agents/opencode.json.tmpl",
        "agents/opencode-models.json",
        "agents/amp.json.tmpl",
        "agents/codex.json.tmpl",
    }

    for _, tmpl := range templates {
        content, _ := templatesFS.ReadFile("templates/" + tmpl)
        dst := filepath.Join(templatesDir, filepath.Base(tmpl))
        os.WriteFile(dst, content, 0644)
    }

    return nil
}
```

### 6. New CLI Commands

#### `gt agents template`

```bash
# List available templates
gt agents template list

# Generate agent config from template
gt agents template opencode --model openai/gpt-5.2-codex --output settings/agents.json

# Preview generated config
gt agents template opencode --model openai/gpt-5.2-codex --dry-run
```

#### `gt agents add`

```bash
# Add opencode agent with specific model
gt agents add opencode-gpt52 --template opencode --model openai/gpt-5.2-codex

# Add with custom settings
gt agents add my-agent --template opencode --model google/gemini-3-pro-high --ready-delay 10000
```

---

## Implementation Plan

### Phase 1: Core Templates (MVP)

1. **Add OpenCode preset** to `internal/config/agents.go`
2. **Create template files** in `templates/agents/`
3. **Embed templates** in binary via `//go:embed`
4. **Add `gt agents template` command**

### Phase 2: Town Initialization

1. **Modify `gt init`** to copy templates to `settings/templates/`
2. **Generate example `agents.json`** with commented templates
3. **Add validation** for agent configurations

### Phase 3: Enhanced UX

1. **Interactive wizard**: `gt agents add --interactive`
2. **Model discovery**: Query opencode for available models
3. **Health check**: `gt agents check <name>` validates config works

### Phase 4: Documentation

1. **Update AGENTS.md** with template usage
2. **Add examples** for common configurations
3. **Create troubleshooting guide** for startup issues

---

## File Changes Summary

### New Files

| File | Purpose |
|------|---------|
| `templates/agents/opencode.json.tmpl` | OpenCode agent template |
| `templates/agents/opencode-models.json` | Model presets for OpenCode |
| `templates/agents/amp.json.tmpl` | Amp agent template |
| `templates/agents/codex.json.tmpl` | Codex agent template |
| `templates/agents/README.md` | Template documentation |
| `internal/cmd/agents_template.go` | Template CLI commands |

### Modified Files

| File | Changes |
|------|---------|
| `internal/config/agents.go` | Add AgentOpenCode preset |
| `internal/config/types.go` | Add TemplateConfig struct |
| `internal/cmd/init.go` | Copy templates during init |
| `internal/cmd/agents.go` | Add template subcommands |
| `templates/embed.go` | Embed agent templates |

---

## Example Usage After Implementation

### Scenario 1: New Town with OpenCode Refinery

```bash
# Initialize town
gt init

# Generated settings/agents.json includes:
# - Commented template examples
# - Empty agents map ready for customization

# Add opencode agent for refinery
gt agents add opencode-gpt52-codex \
  --template opencode \
  --model openai/gpt-5.2-codex

# Configure refinery to use it
gt config role-agent refinery opencode-gpt52-codex

# Start refinery (works immediately)
gt refinery start my-rig
```

### Scenario 2: LLM-Assisted Configuration

User prompt to Claude/Mayor:
> "Configure the refinery to use OpenAI's GPT-5.2 Codex model via opencode"

Claude can now:
1. Read template from `settings/templates/opencode.json.tmpl`
2. Read model presets from `settings/templates/opencode-models.json`
3. Generate correct configuration
4. Write to `settings/agents.json`
5. Update `role_agents` in settings

### Scenario 3: Quick Model Switch

```bash
# Switch refinery from GPT-5.2 to Gemini
gt agents add opencode-gemini \
  --template opencode \
  --model google/gemini-3-pro-high

gt config role-agent refinery opencode-gemini
gt refinery restart my-rig
```

---

## Acceptance Criteria

- [ ] `gt init` creates `settings/templates/` with agent templates
- [ ] `gt agents template list` shows available templates
- [ ] `gt agents template opencode --model X` generates valid config
- [ ] OpenCode preset works without manual `runtime` configuration
- [ ] Refinery starts successfully with opencode agent
- [ ] Documentation updated with template usage examples
- [ ] Existing Claude workflows unaffected

---

## Appendix A: Current RuntimeConfig Defaults by Provider

| Provider | ReadyPromptPrefix | ReadyDelayMs | ProcessNames |
|----------|-------------------|--------------|--------------|
| claude | `"> "` | 10000 | ["node"] |
| codex | `""` | 3000 | ["codex"] |
| opencode | `""` (proposed) | 8000 (proposed) | ["opencode", "node"] |
| generic | `""` | 0 | [] |

---

## Appendix B: OpenCode CLI Reference

```bash
# Basic usage
opencode -m <provider>/<model>

# With session resume
opencode --session <session-id>

# Non-interactive mode
opencode run --format json "<prompt>"

# List available models
opencode models list
```

---

## Appendix C: Discovered Issues During Investigation

1. **Prompt detection assumes clean lines**: Box-drawing characters (`┃`) break prefix matching
2. **No provider-specific defaults for opencode**: Falls back to claude settings
3. **agents.json not auto-created**: Users must know to create it manually
4. **Model timeout variance**: Free models need longer delays than paid
5. **Missing validation**: Invalid agent configs fail silently at startup

---

---

## Appendix D: Bashrs Integration Proposal

### Problem: Shell Safety in Agent Execution

Gastown agents execute shell commands via tmux `send-keys`. Current issues:

1. **Shell injection risk**: Commands built from user/agent input
2. **Word splitting**: Unquoted variables can cause unexpected behavior
3. **Glob expansion**: Filenames with special chars can cause issues
4. **Inconsistent escaping**: Different shells handle escaping differently

### Solution: Integrate Bashrs (Rash)

**Repository**: https://github.com/paiml/bashrs

Bashrs is a Rust-to-Shell transpiler with:
- Automatic safety protections against shell injection
- AST-based semantic transformation
- Multi-shell compatibility (sh, dash, bash, ash, zsh, mksh)
- ShellCheck compliance for all output
- MCP Server for AI-assisted script generation

### Installation

```bash
cargo install bashrs
```

### Integration Points

#### 1. Command Execution Safety

**Current** (in `internal/tmux/tmux.go`):
```go
func (t *Tmux) SendKeys(session, keys string, enter bool) error {
    args := []string{"send-keys", "-t", session, keys}
    // Direct string concatenation - injection risk
}
```

**Proposed**: Use bashrs to sanitize commands:
```go
func (t *Tmux) SendKeysSafe(session, command string, enter bool) error {
    // Use bashrs to escape/validate command
    safeCmd, err := bashrs.Purify(command)
    if err != nil {
        return fmt.Errorf("unsafe command: %w", err)
    }
    args := []string{"send-keys", "-t", session, safeCmd}
}
```

#### 2. Environment Variable Setting

**Current** (in refinery manager):
```go
envVars := config.AgentEnv(config.AgentEnvConfig{...})
for k, v := range envVars {
    _ = t.SetEnvironment(sessionID, k, v)
}
```

**Proposed**: Validate env vars through bashrs:
```go
for k, v := range envVars {
    if !bashrs.IsValidEnvName(k) || !bashrs.IsValidEnvValue(v) {
        continue // Skip potentially dangerous values
    }
    _ = t.SetEnvironment(sessionID, k, v)
}
```

#### 3. MCP Server for Agent Scripts

Bashrs provides an MCP server (`rash-mcp`) that could be used by agents:

```json
{
  "mcpServers": {
    "rash": {
      "command": "rash-mcp",
      "args": []
    }
  }
}
```

This enables agents to:
- Generate safe shell scripts
- Lint scripts before execution
- Audit commands for security issues

#### 4. CI/CD Command Generation

The refinery sets many env vars for non-interactive execution:
```bash
export CI=true DEBIAN_FRONTEND=noninteractive GIT_TERMINAL_PROMPT=0 ...
```

Bashrs could generate these safely:
```rust
// Using bashrs Rust API
let script = bashrs::Script::new()
    .env("CI", "true")
    .env("DEBIAN_FRONTEND", "noninteractive")
    .env("GIT_TERMINAL_PROMPT", "0")
    .command("git fetch --prune origin")
    .build();
```

### Bashrs CLI Commands Useful for Gastown

| Command | Use Case |
|---------|----------|
| `bashrs lint` | Validate shell scripts in hooks |
| `bashrs purify` | Improve existing bash scripts |
| `bashrs audit` | Security review of agent commands |
| `bashrs test` | Test shell script behaviors |

### Go Integration Code

**File**: `internal/shell/safe.go`

```go
package shell

import (
    "fmt"
    "os/exec"
    "strings"
)

// bashrsAvailable caches whether bashrs is installed
var bashrsAvailable *bool

// IsBashrsAvailable checks if bashrs CLI is installed
func IsBashrsAvailable() bool {
    if bashrsAvailable != nil {
        return *bashrsAvailable
    }
    _, err := exec.LookPath("bashrs")
    available := err == nil
    bashrsAvailable = &available
    return available
}

// Purify sanitizes a shell command using bashrs
// Falls back to original if bashrs not available
func Purify(script string) (string, error) {
    if !IsBashrsAvailable() {
        return script, nil // Fallback: return unchanged
    }

    cmd := exec.Command("bashrs", "purify", "-")
    cmd.Stdin = strings.NewReader(script)

    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("bashrs purify failed: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}

// Lint validates a shell script using bashrs
func Lint(script string) error {
    if !IsBashrsAvailable() {
        return nil // Skip if not available
    }

    cmd := exec.Command("bashrs", "lint", "-")
    cmd.Stdin = strings.NewReader(script)

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("bashrs lint failed: %w", err)
    }
    return nil
}

// Audit performs security review of a command
func Audit(script string) (string, error) {
    if !IsBashrsAvailable() {
        return "", nil
    }

    cmd := exec.Command("bashrs", "audit", "-")
    cmd.Stdin = strings.NewReader(script)

    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("bashrs audit failed: %w", err)
    }
    return string(out), nil
}
```

**Usage in tmux.go**:
```go
import "github.com/gastown/internal/shell"

func (t *Tmux) NudgeSession(session, message string) error {
    // Optionally purify the message for shell safety
    safeMsg, err := shell.Purify(message)
    if err != nil {
        // Log warning but continue with original
        log.Printf("shell.Purify warning: %v", err)
        safeMsg = message
    }
    return t.SendKeys(session, safeMsg, true)
}
```

### Implementation Plan

1. **Add bashrs as optional dependency** (checked at runtime via PATH)
2. **Create `internal/shell/safe.go`** wrapper for bashrs CLI
3. **Modify tmux package** to use safe command execution (optional)
4. **Add MCP server config** for agents that support it
5. **Document shell safety** in AGENTS.md
6. **Add `gt doctor` check** for bashrs availability

### Benefits

- **Prevent injection attacks** from malicious prompts
- **Consistent behavior** across different shell environments
- **Audit trail** for executed commands
- **AI-assisted** script generation via MCP

---

## References

- [OpenCode Documentation](https://opencode.dev)
- [Bashrs (Rash) Repository](https://github.com/paiml/bashrs)
- [Gas Town Agent Architecture](../architecture/agents.md)
- [RuntimeConfig Specification](../specs/runtime-config.md)
- [Refinery Investigation Log](./refinery-opencode-debug.md)
