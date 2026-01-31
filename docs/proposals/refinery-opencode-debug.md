# Refinery OpenCode Debug Log

> **Date**: 2026-01-18
> **Issue**: Refinery fails to start with opencode agent
> **Resolution**: Added runtime config with delay-based ready detection

---

## Timeline

### 1. Initial Error

```
Error: starting refinery: waiting for refinery to start: timeout waiting for runtime prompt
```

### 2. Investigation Steps

#### Step 2.1: Check Agent Configuration

```bash
cat gastown_ui/settings/config.json
```

```json
{
  "type": "rig-settings",
  "version": 1,
  "role_agents": {
    "refinery": "opencode-gpt52-codex",
    "witness": "claude-sonnet",
    "polecat": "claude-opus"
  }
}
```

**Finding**: No `runtime` section - using defaults.

#### Step 2.2: Trace Code Path

In `internal/config/loader.go`:
```go
func LoadRuntimeConfig(rigPath string) *RuntimeConfig {
    // ...
    if settings.Runtime == nil {
        return DefaultRuntimeConfig()  // <-- Falls back to claude
    }
}
```

In `internal/config/types.go`:
```go
func DefaultRuntimeConfig() *RuntimeConfig {
    return normalizeRuntimeConfig(&RuntimeConfig{Provider: "claude"})
}

func defaultReadyPromptPrefix(provider string) string {
    if provider == "claude" {
        return "> "  // <-- Claude's prompt
    }
    return ""
}
```

**Finding**: DefaultRuntimeConfig uses `provider: "claude"` which sets `ReadyPromptPrefix: "> "`.

#### Step 2.3: Capture OpenCode Prompt

Started opencode in tmux and captured output:

```
                     █▀▀█ █▀▀█ █▀▀█ █▀▀▄ █▀▀▀ █▀▀█ █▀▀█ █▀▀█
                     █  █ █  █ █▀▀▀ █  █ █    █  █ █  █ █▀▀▀
                     ▀▀▀▀ █▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀

   ┃
   ┃  Ask anything... "Fix a TODO in the codebase"
   ┃
   ┃  Sisyphus  GPT-5.2 Codex OpenAI
   ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
```

**Finding**: Lines have `┃` box-drawing character before text.

#### Step 2.4: Analyze WaitForRuntimeReady

In `internal/tmux/tmux.go`:
```go
func (t *Tmux) WaitForRuntimeReady(session string, rc *config.RuntimeConfig, timeout time.Duration) error {
    // ...
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        prefix := strings.TrimSpace(rc.Tmux.ReadyPromptPrefix)
        if strings.HasPrefix(trimmed, rc.Tmux.ReadyPromptPrefix) || (prefix != "" && trimmed == prefix) {
            return nil
        }
    }
    return fmt.Errorf("timeout waiting for runtime prompt")
}
```

**Analysis**:
- Line: `"   ┃  Ask anything..."`
- After TrimSpace: `"┃  Ask anything..."`
- Expected prefix: `"> "`
- `strings.HasPrefix("┃  Ask anything...", "> ")` = **false**

#### Step 2.5: First Fix Attempt

Added runtime config with prompt prefix:

```json
"runtime": {
  "provider": "opencode",
  "tmux": {
    "ready_prompt_prefix": "Ask anything",
    "ready_delay_ms": 5000
  }
}
```

**Result**: Still failed - `┃` character prevents prefix match.

#### Step 2.6: Final Fix

Use delay-based detection instead of prompt matching:

```json
"runtime": {
  "provider": "opencode",
  "tmux": {
    "ready_prompt_prefix": "",
    "ready_delay_ms": 8000
  }
}
```

**Result**: Success! Refinery starts and processes merges.

---

## Root Cause

1. **RuntimeConfig defaults to Claude provider** when no explicit config exists
2. **Claude's prompt prefix `"> "`** doesn't match opencode's `"┃  Ask anything..."`
3. **Box-drawing characters** in opencode UI break prefix detection even with correct prefix
4. **No built-in opencode preset** means users must configure manually

---

## Solution Applied

### Configuration Change

**File**: `/Users/amrit/Documents/Projects/Rust/mouchak/gastown_exp/gastown_ui/settings/config.json`

```json
{
  "type": "rig-settings",
  "version": 1,
  "role_agents": {
    "refinery": "opencode-gpt52-codex",
    "witness": "claude-sonnet",
    "polecat": "claude-opus"
  },
  "runtime": {
    "provider": "opencode",
    "tmux": {
      "ready_prompt_prefix": "",
      "ready_delay_ms": 8000
    }
  }
}
```

### Verification

```bash
gt refinery start gastown_ui
# ✓ Refinery started for gastown_ui
```

Merge test:
```bash
git checkout -b test/refinery-pr-test
echo "# Test" >> README.md
git commit -am "test: verify refinery merge flow"
git push -u origin test/refinery-pr-test
gt mq submit
# Refinery successfully merged branch
```

---

## Recommendations

### Short-term (Workaround)

Users configuring opencode agents should add this to their rig's `settings/config.json`:

```json
"runtime": {
  "provider": "opencode",
  "tmux": {
    "ready_prompt_prefix": "",
    "ready_delay_ms": 8000
  }
}
```

### Long-term (Code Changes)

1. **Add opencode to built-in presets** with correct defaults
2. **Create agent templates** for common configurations
3. **Document non-Claude agent setup** in AGENTS.md
4. **Consider alternative ready detection** for TUI-based agents

---

## Related Files

| File | Role |
|------|------|
| `internal/config/types.go:330` | DefaultRuntimeConfig() |
| `internal/config/types.go:553` | defaultReadyPromptPrefix() |
| `internal/tmux/tmux.go:962` | WaitForRuntimeReady() |
| `internal/refinery/manager.go:228` | Refinery startup call |

---

## Test Commands Used

```bash
# Check tmux sessions
/opt/homebrew/bin/tmux ls

# Capture opencode prompt
/opt/homebrew/bin/tmux capture-pane -t <session> -p

# Start refinery
gt refinery start gastown_ui

# Check refinery status
gt refinery status gastown_ui

# Check merge queue
gt mq list gastown_ui
```
