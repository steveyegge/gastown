#!/bin/bash
# reset-vm.sh - Restore VM to pristine v0.5.0 state from backup
#
# Usage: ./scripts/migration-test/reset-vm.sh <town_root>
#
# Requires setup-backup.sh to have run first.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[X]${NC} $1"; exit 1; }

TOWN_ROOT="${1:?Usage: reset-vm.sh <town_root>}"
BACKUP_DIR="$TOWN_ROOT/.migration-test-backup"

if [[ ! -d "$BACKUP_DIR" ]]; then
    fail "No backup found at $BACKUP_DIR. Run setup-backup.sh first."
fi

log "Resetting $TOWN_ROOT to pre-migration state..."

# Stop Dolt server if running
if gt dolt status &>/dev/null 2>&1; then
    log "Stopping Dolt server..."
    gt dolt stop 2>/dev/null || true
fi

# Restore town-level .beads
if [[ -d "$BACKUP_DIR/town-beads" ]]; then
    log "Restoring town-level .beads..."
    rm -rf "$TOWN_ROOT/.beads"
    cp -a "$BACKUP_DIR/town-beads" "$TOWN_ROOT/.beads"
fi

# Restore each rig's .beads
for rig_backup in "$BACKUP_DIR"/rigs/*/; do
    [[ -d "$rig_backup" ]] || continue
    rig_name=$(basename "$rig_backup")
    rig_dir="$TOWN_ROOT/$rig_name"

    if [[ -d "$rig_dir" ]]; then
        log "Restoring $rig_name/.beads..."
        rm -rf "$rig_dir/.beads"
        cp -a "$rig_backup/.beads" "$rig_dir/.beads"
    else
        warn "Rig directory missing: $rig_dir (skipping)"
    fi
done

# Remove any Dolt data directories created during migration
for dolt_dir in "$TOWN_ROOT"/.dolt-data "$TOWN_ROOT"/*/.dolt-data; do
    if [[ -d "$dolt_dir" ]]; then
        log "Removing Dolt data: $dolt_dir"
        rm -rf "$dolt_dir"
    fi
done

# Restore daemon config
if [[ -f "$BACKUP_DIR/daemon.json" ]]; then
    log "Restoring daemon.json..."
    cp "$BACKUP_DIR/daemon.json" "$TOWN_ROOT/mayor/daemon.json"
fi

# Clear migration artifacts
rm -f "$TOWN_ROOT/.migration-test-counts.json"
if [[ -f "$BACKUP_DIR/counts.json" ]]; then
    cp "$BACKUP_DIR/counts.json" "$TOWN_ROOT/.migration-test-counts.json"
fi

log "Reset complete. Town is back to v0.5.0 (SQLite) state."
