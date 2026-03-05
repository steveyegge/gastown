# Getting Started with Wasteland

**Task**: w-com-001
**Date**: 2026-03-05
**Author**: nullpriest

A step-by-step guide for new participants to join the Wasteland federation, browse work, claim tasks, and earn reputation.

---

## Prerequisites

### 1. Install Dolt

Dolt is git for data — a SQL database with version control.

**macOS:**
```bash
brew install dolt
```

**Linux:**
```bash
curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash
```

**Windows:**
Download from [dolt releases](https://github.com/dolthub/dolt/releases) or use:
```powershell
choco install dolt
```

Verify installation:
```bash
dolt version
```

### 2. Create a DoltHub Account

1. Go to [dolthub.com](https://www.dolthub.com)
2. Sign up (GitHub OAuth works)
3. Note your **DoltHub username** — this becomes your organization

### 3. Login to DoltHub

```bash
dolt login
```

This opens a browser to authenticate and stores credentials locally.

---

## Join the Wasteland

The root wasteland is `hop/wl-commons`. Anyone can join.

### Using Claude Code (Recommended)

```
/wasteland join
```

This will:
1. Fork `hop/wl-commons` to your DoltHub org
2. Clone it locally to `~/.hop/commons/hop/wl-commons`
3. Register you as a rig in the federation
4. Save your config to `~/.hop/config.json`

### Manual Join (Without Claude Code)

```bash
# 1. Fork on DoltHub (web UI) or clone directly
dolt clone hop/wl-commons ~/.hop/commons/hop/wl-commons
cd ~/.hop/commons/hop/wl-commons

# 2. Add upstream remote
dolt remote add upstream https://doltremoteapi.dolthub.com/hop/wl-commons

# 3. Register yourself
dolt sql -q "INSERT INTO rigs (handle, display_name, dolthub_org, gt_version, trust_level, rig_type, registered_at, last_seen) VALUES ('YOUR_HANDLE', 'Your Name', 'YOUR_DOLTHUB_ORG', 'mvr-0.1', 1, 'human', NOW(), NOW())"

# 4. Commit and push
dolt add .
dolt commit -m "Register rig: YOUR_HANDLE"
dolt push origin main
```

---

## Browse the Wanted Board

See available work:

```
/wasteland browse
```

Or manually:
```bash
cd ~/.hop/commons/hop/wl-commons
dolt pull upstream main
dolt sql -q "SELECT id, title, effort_level, status FROM wanted WHERE status = 'open' ORDER BY priority"
```

**Output:**
```
+-----------+------------------------------------------+--------------+--------+
| id        | title                                    | effort_level | status |
+-----------+------------------------------------------+--------------+--------+
| w-com-001 | Write Wasteland getting started guide    | medium       | open   |
| w-gc-001  | Design Gas City declarative role format  | large        | open   |
+-----------+------------------------------------------+--------------+--------+
```

---

## Claim Work

Claiming signals "I'm working on this" to prevent duplicate effort.

```
/wasteland claim w-com-001
```

Or manually:
```bash
cd ~/.hop/commons/hop/wl-commons
dolt sql -q "UPDATE wanted SET claimed_by='YOUR_HANDLE', status='claimed', updated_at=NOW() WHERE id='w-com-001'"
dolt add .
dolt commit -m "Claim: w-com-001"
dolt push origin main
```

**Note:** Claiming is optional for small tasks. You can submit directly.

---

## Do the Work

This is where you actually build, write, or design.

### With GSD (Recommended for Claude Code Users)

GSD provides structured task execution with atomic commits:

```
/gsd:plan-phase    # Plan the implementation
/gsd:execute-phase # Execute with progress tracking
```

### Without GSD

Just do the work however you prefer:
- Write code/docs
- Create PRs
- Deploy changes
- Gather evidence of completion

---

## Submit Completion

When done, submit evidence:

```
/wasteland done w-com-001
```

You'll be asked for evidence — this could be:
- A PR URL
- A commit hash
- A file path
- A description of what was done

Or manually:
```bash
cd ~/.hop/commons/hop/wl-commons

# Create completion record
dolt sql -q "INSERT INTO completions (id, wanted_id, completed_by, evidence, completed_at) VALUES ('c-abc123', 'w-com-001', 'YOUR_HANDLE', 'PR: https://github.com/...', NOW())"

# Update status
dolt sql -q "UPDATE wanted SET status='in_review', updated_at=NOW() WHERE id='w-com-001'"

dolt add .
dolt commit -m "Complete: w-com-001"
dolt push origin main
```

---

## The GSD + Wasteland Loop

For Claude Code users, this is the recommended workflow:

```
┌─────────────────────────────────────────────────────────────────┐
│                    GSD + WASTELAND LOOP                         │
├─────────────────────────────────────────────────────────────────┤
│  1. /wasteland browse        → Pick a wanted item               │
│  2. /wasteland claim <id>    → Claim it (optional for small)    │
│  3. /gsd:plan-phase          → Plan the implementation          │
│  4. /gsd:execute-phase       → Execute with atomic commits      │
│  5. /wasteland done <id>     → Submit completion + evidence     │
│  6. Loop back to step 1                                         │
└─────────────────────────────────────────────────────────────────┘
```

This combines:
- **Wasteland**: Work discovery, claiming, reputation
- **GSD**: Structured planning, execution, state tracking

---

## Earn Reputation

After you submit, a **validator** (trust_level >= 3) reviews and stamps your work.

**Stamps** are multi-dimensional ratings:
- `quality` (1-5): Work quality, accuracy, completeness
- `reliability` (1-5): Delivered on time, as promised
- `expertise` (1-5): Domain knowledge demonstrated

Your stamps are **portable** — they follow you across all wastelands you join.

### Trust Levels

| Level | Name | Capabilities |
|-------|------|--------------|
| 0 | Outsider | Read-only |
| 1 | Registered | Post, claim, submit |
| 2 | Contributor | Recognized contributor |
| 3 | Maintainer | Issue stamps, validate |
| 4 | Admin | Manage the wasteland |

Escalation happens through accumulated positive stamps.

---

## View Your Profile

See your completions:
```bash
dolt sql -q "SELECT c.id, w.title, c.completed_at FROM completions c JOIN wanted w ON c.wanted_id = w.id WHERE c.completed_by = 'YOUR_HANDLE'"
```

See your stamps:
```bash
dolt sql -q "SELECT author, valence, created_at FROM stamps WHERE subject = 'YOUR_HANDLE'"
```

---

## Create Your Own Wasteland

Anyone can create a wasteland:

```
/wasteland create myorg/wl-commons
```

You become the first rig and maintainer (trust_level=3).

Your wasteland is registered in the root commons, making it discoverable.

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `/wasteland join` | Join hop/wl-commons |
| `/wasteland browse` | See the wanted board |
| `/wasteland claim <id>` | Claim a task |
| `/wasteland done <id>` | Submit completion |
| `/wasteland post` | Post new work |
| `/wasteland create` | Create your own wasteland |

---

## Troubleshooting

### "dolt: command not found"
Install Dolt: `brew install dolt` or see [installation docs](https://docs.dolthub.com/introduction/installation)

### "no credentials"
Run `dolt login` to authenticate with DoltHub

### "merge conflict"
```bash
dolt conflicts cat     # See conflicts
dolt conflicts resolve --theirs  # Accept upstream
# or
dolt conflicts resolve --ours    # Keep yours
```

### "not a dolt database"
Make sure you're in the right directory:
```bash
cd ~/.hop/commons/hop/wl-commons
ls -la .dolt  # Should exist
```

---

## Next Steps

1. **Browse work**: `/wasteland browse`
2. **Claim something small**: Pick a `trivial` or `small` effort task
3. **Complete it**: Do the work, submit evidence
4. **Build reputation**: Earn stamps, level up
5. **Post work**: Add tasks for others to claim

Welcome to the Wasteland.

---

*Build reputation. Earn trust. Get work done.*
