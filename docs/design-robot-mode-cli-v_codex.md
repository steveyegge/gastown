# Robot Mode CLI Design for `gt` (Gas Town) v2 (Codex Variant)

> **Status**: Design Document
> **Repo**: `steveyegge/gastown`
> **Updated**: 2026-01-31
> **Version**: 2.1-codex
> **Contract Version**: 1
> **API Version**: 1
> **Primary audience**: AI coding agents + automation scripts
> **Secondary audience**: humans in a terminal
> **Reference**: Base spec in `docs/design-robot-mode-cli.md`

This document is a Codex-optimized variant of the robot mode design. It keeps the same contract and envelopes, while adding explicit guidance for:

- **Agent-first CLI help**: deterministic, concise, and easy to parse.
- **Intent-tolerant parsing**: honor commands when intent is clear.
- **Diagnostic errors**: detailed errors with examples when intent is unclear.

---

## 0. Goals and Non-Goals

### Goals (MUST)
1. **JSON output everywhere**: a `--json` flag on *every* command.
2. **Auto JSON when piped**: TTY detection flips to robot output when stdout is not a terminal.
3. **Token-efficient**: compact envelopes, abbreviated keys, no decorative text in robot mode.
4. **Structured errors**: error responses include `code`, `msg`, and ordered `hint[]`.
5. **Meaningful exit codes**: stable, semantic codes for automation.
6. **Discoverability**: schema/commands introspection so agents can explore without docs.
7. **Performance under concurrency**: safe for high fan-out agent usage.
8. **Intent-tolerant parsing**: accept commands with minor syntax issues when intent is legible.
9. **Corrective feedback**: when we accept a corrected command, emit a note on the preferred syntax.
10. **Helpful failures**: when intent is unclear, return a precise error with example fixes.

### Non-Goals (for now)
- Replacing human output styling/formatting.
- YAML output.
- Full OpenAPI spec (export path deferred).
- Interactive prompts in robot mode.

---

## 1. Design Principles

### 1.1 Output Philosophy

| Audience | Needs | Human Mode | Robot Mode |
|----------|-------|------------|------------|
| Human | Visual hierarchy, colors, progress | Lipgloss styling, emojis | Disabled |
| AI Agent | Parseable, minimal, structured | N/A | Default |
| Scripts | Exit codes, quiet mode | `--quiet` | Enhanced |

### 1.2 Core Tenets

1. **Stderr for progress, stdout for results** - Agents can ignore stderr
2. **Exit codes are semantic** - Every failure mode has a unique code
3. **JSON is the contract** - Human output is a "pretty-print" of JSON
4. **Fail fast, fail informatively** - Errors include recovery paths
5. **Idempotency declared** - Commands expose side effects explicitly

### 1.3 Intent-Tolerant Commands

Robot mode is designed for AI agents. If intent is clear, prefer doing the right thing rather than refusing.

**Rule**: Accept commands with minor syntax errors when the intent is unambiguous.

Examples of acceptable fixes:
- Single-character typos in flags (`--robt` -> `--robot`)
- Common alias replacements (`--json` -> `--robot`)
- Flag order normalization
- Missing leading dashes for known flags (`robot` -> `--robot` when used after command)

If auto-correction happens, respond normally **and** include a note instructing the preferred syntax.

If intent is **not** reliably clear, **do not guess**: return a detailed error with correction examples.

---

## 2. Activation Modes

### 2.1 Flag Hierarchy (highest wins)

```
--human         Force human mode (styled, decorated)
--robot         Force robot mode (JSON envelope + robot-friendly behavior)
--json          Alias for --robot (for "always JSON" expectation)
--quiet         Suppress non-error output (still sets exit codes)
--robot-format  Output format: json (pretty), jsonl (streaming), compact (single-line)
--robot-meta    Include extended metadata (_meta block)
--robot-help    Deterministic machine-first help (no TUI, no ANSI)
--request-id    Echo ID in response for correlation/tracing
```

### 2.2 TTY Auto-Detection

Robot mode is auto-enabled when stdout is not a terminal:

```bash
gt status | jq         # Robot mode
gt status > out.json   # Robot mode
gt status              # Human mode (TTY)
```

### 2.3 Environment Variable Override

```bash
GT_OUTPUT_MODE=robot gt status   # Force robot mode
GT_OUTPUT_MODE=human gt status   # Force human mode
GT_OUTPUT_MODE=quiet gt status   # Suppress non-error output
```

---

## 3. Response Envelope (Same Contract)

Robot mode output is always a single JSON object per command invocation (unless streaming).

### 3.1 Success Response

```json
{
  "ok": true,
  "cmd": "gt rig list",
  "data": { "items": [] },
  "meta": { "ts": "2026-01-31T10:30:00Z", "ms": 42, "v": "0.9.0" }
}
```

### 3.2 Error Response

```json
{
  "ok": false,
  "cmd": "gt rig show nope",
  "exit": 3,
  "error": {
    "code": "E_RIG_NOT_FOUND",
    "msg": "Rig 'nope' not found",
    "hint": [
      "Run 'gt rig list' to see available rigs",
      "Check if the rig was removed or renamed"
    ],
    "ctx": { "searched": "nope", "similar": ["node", "notes"] }
  },
  "meta": { "ts": "2026-01-31T10:30:00Z", "ms": 12, "v": "0.9.0" }
}
```

### 3.3 Correction Note (When Intent Was Auto-Fixed)

When a request is accepted with a correction, include a short `_note` field:

```json
{
  "ok": true,
  "cmd": "gt status",
  "data": { "agents": 3 },
  "_note": "Interpreted '--robt' as '--robot'. Prefer: gt status --robot"
}
```

The `_note` field is only used when we auto-correct input. It is omitted otherwise.

---

## 4. CLI Help: Agent-First and Deterministic

### 4.1 Help Requirements

Robot mode help must be:
- Deterministic (stable ordering and wording)
- Minimal (only essential content)
- Explicit about robot mode flags and JSON output

### 4.2 Robot Help Output Example

```bash
gt --robot-help
```

```json
{
  "ok": true,
  "data": {
    "summary": "gt - Gas Town multi-agent orchestration",
    "usage": "gt <command> [args] [flags]",
    "commands": [
      {"name": "status", "desc": "Show system state"},
      {"name": "rig", "desc": "Manage rigs"},
      {"name": "polecat", "desc": "Manage worker agents"}
    ],
    "flags": [
      {"name": "robot", "type": "bool", "desc": "Machine-readable output"},
      {"name": "json", "type": "bool", "desc": "Alias for --robot"},
      {"name": "robot-format", "type": "string", "desc": "json|jsonl|compact"},
      {"name": "robot-meta", "type": "bool", "desc": "Include _meta"},
      {"name": "robot-help", "type": "bool", "desc": "Machine-first help"}
    ],
    "next": [
      "gt --commands --robot",
      "gt <command> --schema --robot"
    ]
  }
}
```

### 4.3 Human Help (Short Blurb)

Human help should include a short robot-mode blurb:

```
Robot mode (for AI agents): add --robot (or --json) for machine-readable output.
Use --robot-help for deterministic, parseable help.
```

---

## 5. Intent Recovery Rules

### 5.1 Acceptable Corrections (Examples)

| Input | Interpreted As | Reason |
|-------|----------------|--------|
| `gt status --robt` | `gt status --robot` | Single-character typo |
| `gt status robot` | `gt status --robot` | Missing dashes for known flag |
| `gt --json status` | `gt status --robot` | Alias and order normalization |

### 5.2 When to Reject

Reject when **intent is not reliable**:
- Multiple plausible commands match equally
- Unknown flag could map to multiple known flags
- Unknown subcommand with multiple close matches

### 5.3 Error Response for Ambiguity

Return a structured error with examples that match the likely intent:

```json
{
  "ok": false,
  "exit": 2,
  "error": {
    "code": "E_USAGE_AMBIGUOUS",
    "msg": "Unknown subcommand 'rg' for 'gt'.",
    "hint": [
      "Did you mean 'rig' or 'rigs'?",
      "Run 'gt --commands --robot' to list commands."
    ],
    "examples": [
      "gt rig list",
      "gt rig show <name>",
      "gt rig add <name> <repo-url>"
    ]
  }
}
```

The `examples` field is only used for usage errors and should contain 2-4 command examples.

---

## 6. Error Codes (Additions)

Add an explicit usage ambiguity code:

| Code | Exit | Description |
|------|------|-------------|
| `E_USAGE_AMBIGUOUS` | 2 | Input could map to multiple commands/flags |

Existing exit code mapping remains unchanged.

---

## 7. Token Efficiency Rules (Same Contract)

- Use abbreviated keys: `ok`, `cmd`, `data`, `meta.ts`, `meta.ms`, `meta.v`, `msg`, `hint`, `ctx`, `ev`, `pct`.
- Omit null/empty fields.
- Prefer stable enums/strings over verbose paragraphs.

---

## 8. Agent-Focused Quick Start (≤100 Tokens)

**Robot mode (agent):**

```json
{"ok":true,"data":{"hint":"Run 'gt --commands --robot' for command list","v":"0.9.0"}}
```

**Preferred agent flow:**

```bash
gt --robot                # minimal intro
gt --commands --robot     # discover commands
gt <cmd> --schema --robot # discover schema
```

---

## 9. Notes for Implementers

- All corrections must be deterministic and logged in the response `_note`.
- Correction note should be one sentence, no ANSI, no emojis.
- Do not attempt corrections that change command intent or side effects.
- When in doubt, fail with `E_USAGE_AMBIGUOUS` and provide examples.

---

*End of Design Document v2.1-codex*
