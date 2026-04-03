+++
name = "rate-limit-watchdog"
description = "Rotate accounts on API rate limit, ESTOP only as last resort — no LLM needed"
version = 2

[gate]
type = "cooldown"
duration = "3m"

[tracking]
labels = ["plugin:rate-limit-watchdog", "category:safety"]
digest = true

[execution]
timeout = "60s"
notify_on_failure = true
severity = "high"
+++

# Rate Limit Watchdog v2

Monitors the Anthropic API for rate limiting (HTTP 429). When detected,
attempts account rotation first. Only triggers `gt estop` if all accounts
are exhausted. Periodically re-checks and runs `gt thaw` when the rate
limit clears, then verifies agents restarted.

This is a **shell-only plugin** — no LLM calls. It runs as a daemon
plugin on a 3-minute cooldown gate.

## How It Works

1. Send a malformed API request (empty messages array — zero token cost)
2. If 400 — API key works. If ESTOP was auto-triggered, thaw and verify restarts.
3. If 429 — Try `gt quota rotate` to swap to an available account.
4. If rotation succeeds — no ESTOP needed.
5. If rotation fails (all accounts limited) — `gt estop -r "All N accounts rate-limited"`.
6. Record result as tracking wisp.

The probe costs $0.00 per check (malformed request, no tokens consumed).

## Behavior

| API Status | ESTOP Active? | Rotation Available? | Action |
|------------|--------------|---------------------|--------|
| 400 (usable) | No | — | No-op (healthy) |
| 400 (usable) | Yes (rate-limit) | — | `gt thaw` + verify restarts |
| 400 (usable) | Yes (other) | — | No-op (manual ESTOP) |
| 429 | No | Yes | `gt quota rotate` (skip ESTOP) |
| 429 | No | No | `gt estop -r "All N accounts rate-limited"` |
| 429 | Yes | — | No-op (already frozen) |
| 000 | Any | — | Log warning, skip |
| 5xx | Any | — | Log warning, skip |

## Post-Thaw Verification

After thawing an auto-triggered ESTOP, the watchdog:
1. Waits 5 seconds for SIGCONT to propagate
2. Checks `gt rig list` for rigs with stopped agents
3. Runs `gt rig start <rig>` for any dead rigs

## Configuration

The plugin uses these environment variables:
- `ANTHROPIC_API_KEY` — required for the probe request
- `GT_ROOT` — town root for estop/thaw commands

Requires `gt quota status --probe --first-available` and `gt quota rotate`
commands (available in Gas Town v1.0+).

The 3-minute cooldown gate prevents rapid estop/thaw cycling.
Timeout is 60s to allow time for account probing and rotation.
