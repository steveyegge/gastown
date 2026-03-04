# Herald Agent

The Herald is a Wasteland event announcer. It polls the DoltHub SQL API for
changes to the `wl-commons` database and prints announcements for:

- **New wanted items** — tasks posted to the board
- **Completions** — work submitted or validated
- **New rigs** — contributors joining the Wasteland

## Requirements

Python 3.10+ (stdlib only, no external dependencies).

## Quick Start

```bash
# Run once — seeds state on first run, then shows new events
python3 herald.py

# Run in a continuous loop (default: poll every 120 seconds)
python3 herald.py --loop

# Custom poll interval
python3 herald.py --loop --interval 60

# Post announcements to Discord
python3 herald.py --loop --discord-webhook https://discord.com/api/webhooks/...
```

## How It Works

1. **Seed** — On first run, the herald fetches the current state of all tables
   and saves their IDs to `state.json`. This prevents announcing everything
   that already exists.

2. **Poll** — Each cycle, the herald queries the 50 most recent rows from
   `wanted`, `completions`, and `rigs`.

3. **Diff** — New rows (IDs not in `state.json`) are identified.

4. **Announce** — New events are printed to stdout (and optionally posted to
   a Discord webhook).

5. **Persist** — Updated state is saved to `state.json`.

## State File

`state.json` tracks which rows have been announced. It is gitignored.
Delete it to re-seed and start fresh.

## Example Output

```
[2026-03-04 20:15 UTC] [WANTED] New feature posted by @steveyegge in [gastown] (P1, open)
         w-gt-010: Add herald agent to announce events

[2026-03-04 20:15 UTC] [COMPLETION] @alice-rig completed w-com-003 (validated)
             Evidence: https://github.com/steveyegge/gastown/pull/2342
             Validated by @steveyegge

[2026-03-04 20:15 UTC] [NEW RIG] @bob-rig (Bob) joined the Wasteland — human, trust T0
```

## CLI Options

| Flag | Default | Description |
|------|---------|-------------|
| `--loop` | off | Run continuously instead of one-shot |
| `--interval N` | 120 | Seconds between polls (requires `--loop`) |
| `--discord-webhook URL` | none | Discord webhook URL for posting |
| `--no-seed` | off | Skip initial state seeding |

## Extending

To add new event types (e.g., trust tier changes, new stamps), add entries
to the `QUERIES` dict and corresponding formatter functions in `herald.py`.
