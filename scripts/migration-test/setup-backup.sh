#!/bin/bash
# setup-backup.sh - Create pristine v0.5.0 backup for reset capability
#
# Usage: ./scripts/migration-test/setup-backup.sh <town_root>
#
# Run this ONCE after seeding data but BEFORE migration.
# Creates a full backup that reset-vm.sh can restore from.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
fail() { echo -e "${RED}[X]${NC} $1"; exit 1; }

TOWN_ROOT="${1:?Usage: setup-backup.sh <town_root>}"
BACKUP_DIR="$TOWN_ROOT/.migration-test-backup"

if [[ -d "$BACKUP_DIR" ]]; then
    fail "Backup already exists at $BACKUP_DIR. Remove it first to re-create."
fi

log "Creating pristine v0.5.0 backup at $BACKUP_DIR"
mkdir -p "$BACKUP_DIR"

# Backup town-level .beads
if [[ -d "$TOWN_ROOT/.beads" ]]; then
    log "Backing up town-level .beads..."
    cp -a "$TOWN_ROOT/.beads" "$BACKUP_DIR/town-beads"
fi

# Backup each rig's .beads
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    rig_beads="$rig_dir/.beads"
    [[ -d "$rig_beads" && -f "$rig_beads/metadata.json" ]] || continue

    log "Backing up $rig_name/.beads..."
    mkdir -p "$BACKUP_DIR/rigs/$rig_name"
    cp -a "$rig_beads" "$BACKUP_DIR/rigs/$rig_name/.beads"
done

# Backup daemon config
if [[ -f "$TOWN_ROOT/mayor/daemon.json" ]]; then
    log "Backing up daemon.json..."
    cp "$TOWN_ROOT/mayor/daemon.json" "$BACKUP_DIR/daemon.json"
fi

# Record backup metadata
cat > "$BACKUP_DIR/metadata.json" <<EOF
{
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "town_root": "$TOWN_ROOT",
  "gt_version": "$(gt --version 2>/dev/null || echo 'unknown')",
  "bd_version": "$(bd --version 2>/dev/null || echo 'unknown')"
}
EOF

# Copy seed counts if available
if [[ -f "$TOWN_ROOT/.migration-test-counts.json" ]]; then
    cp "$TOWN_ROOT/.migration-test-counts.json" "$BACKUP_DIR/counts.json"
fi

log "Backup complete: $(du -sh "$BACKUP_DIR" | cut -f1)"
