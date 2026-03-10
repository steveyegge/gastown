---
description: Run Dolt database backup — sync production DBs to backup remotes and iCloud
allowed-tools: Bash(gt dolt status:*), Bash(dolt backup:*), Bash(dolt log:*), Bash(rsync:*), Bash(du:*), Bash(cat:*), Bash(mkdir:*), Bash(timeout:*), Bash(gt escalate:*), Bash(ls:*)
argument-hint: [--skip-offsite]
---

# Dolt Backup

Sync production Dolt databases to filesystem backups with smart change detection.
Runs the same cycle as `mol-dog-backup` / the `dolt-backup` plugin.

Arguments: $ARGUMENTS
If `--skip-offsite` is passed, skip the iCloud rsync step.

## Configuration

```
DOLT_DATA_DIR=~/gt/.dolt-data
BACKUP_DIR=~/gt/.dolt-backup
ICLOUD_DIR=~/Library/Mobile Documents/com~apple~CloudDocs/gt-dolt-backup
PROD_DBS: auto-discovered (databases with <name>-backup remotes)
BACKUP_TIMEOUT=120s per database
HANG_THRESHOLD=30s for server ping
DOLT_PORT=3307
```

## Execution Steps

### Step 1: Verify Dolt server is responsive

```bash
gt dolt status
```

Also ping directly to detect hangs:

```bash
timeout 30 dolt sql --host 127.0.0.1 --port 3307 --user root --password "" \
  --no-tls -q "SELECT 1" --result-format csv
```

If the ping hangs (takes > 30s) or fails:
```bash
gt escalate "dolt-backup: Dolt server hung or unreachable" -s CRITICAL
```
STOP — do not attempt backups against a hung server.

### Step 2: Discover databases with backup remotes

List directories in `~/gt/.dolt-data/` and check each for a `<name>-backup` remote:

```bash
ls ~/gt/.dolt-data/
```

For each directory (skip dotfiles):
```bash
cd ~/gt/.dolt-data/<name> && dolt backup
```

If the output contains `<name>-backup`, include it in the backup list.
Expected databases: `hq`, `beads`, `gastown`.

### Step 3: Check each DB for changes and sync

For each database with a backup remote:

**3a. Get current HEAD hash:**
```bash
cd ~/gt/.dolt-data/<name> && dolt log -n 1 --oneline | head -1 | cut -d' ' -f1
```

**3b. Compare against last backed-up hash:**
```bash
cat ~/gt/.dolt-backup/<name>/.last-backup-hash 2>/dev/null
```

If hashes match and current hash is not "unknown", skip (unchanged).

**3c. Sync if changed:**
```bash
cd ~/gt/.dolt-data/<name> && timeout 120 dolt backup sync <name>-backup
```

**3d. Record successful hash:**
```bash
mkdir -p ~/gt/.dolt-backup/<name>
echo "<current_hash>" > ~/gt/.dolt-backup/<name>/.last-backup-hash
```

**3e. Get backup size:**
```bash
du -sh ~/gt/.dolt-backup/<name> 2>/dev/null | cut -f1
```

### Step 4: Offsite sync to iCloud Drive

Unless `--skip-offsite` was specified:

```bash
mkdir -p ~/Library/Mobile\ Documents/com~apple~CloudDocs/gt-dolt-backup
rsync -a --delete ~/gt/.dolt-backup/ ~/Library/Mobile\ Documents/com~apple~CloudDocs/gt-dolt-backup/
```

If iCloud is unavailable, log and continue — not fatal.

### Step 5: Report

Print a summary in this format:

```
## Backup Report

**Databases synced**: N/M
**Databases skipped** (unchanged): N
**Databases failed**: N
**Offsite sync**: ok | failed | skipped

### Per Database
- <name>: synced in Ns (<size>) | unchanged (<hash>) | FAILED: <error>

### Failures
<list or "None">
```

If any database failed or server was hung:
```bash
gt escalate "dolt-backup FAILED: <summary>" -s HIGH
```

If server was hung, use `-s CRITICAL` instead.
