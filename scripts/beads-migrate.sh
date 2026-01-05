#!/bin/bash
# beads-migrate.sh - Migrate beads from one prefix to another
#
# Usage:
#   beads-migrate.sh <source-beads-dir> <old-prefix> <new-prefix> [dest-beads-dir]
#
# Examples:
#   # In-place migration (backup created)
#   beads-migrate.sh ~/gt/gastown/mayor/rig/.beads gt- ga-
#
#   # Migration to new location
#   beads-migrate.sh /old/repo/.beads old- new- /new/repo/.beads
#
# This script:
#   1. Creates a backup of issues.jsonl
#   2. Renames prefixes in issue IDs and dependency references
#   3. Renames and updates mq/ queue files
#   4. Removes SQLite database (will regenerate from JSONL)
#
# After running, use 'bd init --reimport' to rebuild the database.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

usage() {
    cat << EOF
Usage: $(basename "$0") <source-beads-dir> <old-prefix> <new-prefix> [dest-beads-dir]

Arguments:
  source-beads-dir  Path to source .beads/ directory
  old-prefix        Prefix to replace (e.g., 'gt-')
  new-prefix        New prefix (e.g., 'ga-')
  dest-beads-dir    Optional destination directory (defaults to source for in-place)

Examples:
  $(basename "$0") ~/project/.beads old- new-
  $(basename "$0") ~/old/.beads old- new- ~/new/.beads

EOF
    exit 1
}

# Validate arguments
if [[ $# -lt 3 ]]; then
    usage
fi

SOURCE_DIR="${1%/}"  # Remove trailing slash
OLD_PREFIX="$2"
NEW_PREFIX="$3"
DEST_DIR="${4:-$SOURCE_DIR}"
DEST_DIR="${DEST_DIR%/}"

# Validate source directory
if [[ ! -d "$SOURCE_DIR" ]]; then
    log_error "Source directory does not exist: $SOURCE_DIR"
    exit 1
fi

if [[ ! -f "$SOURCE_DIR/issues.jsonl" ]]; then
    log_error "No issues.jsonl found in $SOURCE_DIR"
    exit 1
fi

# Validate prefixes end with hyphen
if [[ ! "$OLD_PREFIX" =~ -$ ]]; then
    log_error "Old prefix must end with hyphen: $OLD_PREFIX"
    exit 1
fi

if [[ ! "$NEW_PREFIX" =~ -$ ]]; then
    log_error "New prefix must end with hyphen: $NEW_PREFIX"
    exit 1
fi

log_info "Migration settings:"
log_info "  Source:     $SOURCE_DIR"
log_info "  Dest:       $DEST_DIR"
log_info "  Old prefix: $OLD_PREFIX"
log_info "  New prefix: $NEW_PREFIX"

# Create destination if different from source
if [[ "$SOURCE_DIR" != "$DEST_DIR" ]]; then
    if [[ -d "$DEST_DIR" ]]; then
        log_warn "Destination exists, will overwrite JSONL files"
    else
        log_info "Creating destination directory: $DEST_DIR"
        mkdir -p "$DEST_DIR"
        mkdir -p "$DEST_DIR/mq"
    fi
else
    # In-place migration - create backup
    BACKUP_FILE="$SOURCE_DIR/issues.jsonl.backup.$(date +%Y%m%d%H%M%S)"
    log_info "Creating backup: $BACKUP_FILE"
    cp "$SOURCE_DIR/issues.jsonl" "$BACKUP_FILE"
fi

# Count issues before migration
TOTAL_ISSUES=$(wc -l < "$SOURCE_DIR/issues.jsonl" | tr -d ' ')
MATCHING_ISSUES=$(grep -c "\"id\":\"${OLD_PREFIX}" "$SOURCE_DIR/issues.jsonl" 2>/dev/null || echo "0")

log_info "Found $TOTAL_ISSUES total issues, $MATCHING_ISSUES with prefix '$OLD_PREFIX'"

if [[ "$MATCHING_ISSUES" -eq 0 ]]; then
    log_warn "No issues found with prefix '$OLD_PREFIX' - nothing to migrate"
    exit 0
fi

# Migrate issues.jsonl
# We need to replace the prefix in:
#   - "id":"<prefix>xxx"
#   - "issue_id":"<prefix>xxx"
#   - "depends_on_id":"<prefix>xxx"
log_info "Migrating issues.jsonl..."

# Use sed for reliable prefix replacement in JSON
# Escape special characters in prefixes for regex
OLD_PREFIX_ESC=$(printf '%s\n' "$OLD_PREFIX" | sed 's/[[\.*^$()+?{|]/\\&/g')
NEW_PREFIX_ESC=$(printf '%s\n' "$NEW_PREFIX" | sed 's/[&/\]/\\&/g')

# Create temp file for output
TEMP_FILE=$(mktemp)
trap "rm -f $TEMP_FILE" EXIT

# Replace prefixes in ID fields
# Pattern matches: "id":"<prefix>...", "issue_id":"<prefix>...", "depends_on_id":"<prefix>..."
sed -E \
    -e "s/\"id\":\"${OLD_PREFIX_ESC}/\"id\":\"${NEW_PREFIX_ESC}/g" \
    -e "s/\"issue_id\":\"${OLD_PREFIX_ESC}/\"issue_id\":\"${NEW_PREFIX_ESC}/g" \
    -e "s/\"depends_on_id\":\"${OLD_PREFIX_ESC}/\"depends_on_id\":\"${NEW_PREFIX_ESC}/g" \
    "$SOURCE_DIR/issues.jsonl" > "$TEMP_FILE"

# Move to destination
mv "$TEMP_FILE" "$DEST_DIR/issues.jsonl"
trap - EXIT  # Clear trap since file was moved

log_info "Migrated issues.jsonl"

# Migrate interactions.jsonl if it exists and has content
if [[ -f "$SOURCE_DIR/interactions.jsonl" && -s "$SOURCE_DIR/interactions.jsonl" ]]; then
    log_info "Migrating interactions.jsonl..."
    TEMP_FILE=$(mktemp)
    sed -E \
        -e "s/\"id\":\"${OLD_PREFIX_ESC}/\"id\":\"${NEW_PREFIX_ESC}/g" \
        -e "s/\"issue_id\":\"${OLD_PREFIX_ESC}/\"issue_id\":\"${NEW_PREFIX_ESC}/g" \
        "$SOURCE_DIR/interactions.jsonl" > "$TEMP_FILE"
    mv "$TEMP_FILE" "$DEST_DIR/interactions.jsonl"
    log_info "Migrated interactions.jsonl"
elif [[ "$SOURCE_DIR" != "$DEST_DIR" ]]; then
    # Copy empty file to dest
    touch "$DEST_DIR/interactions.jsonl"
fi

# Migrate mq/ directory
if [[ -d "$SOURCE_DIR/mq" ]]; then
    MQ_COUNT=$(find "$SOURCE_DIR/mq" -name "${OLD_PREFIX}*.json" 2>/dev/null | wc -l | tr -d ' ')
    if [[ "$MQ_COUNT" -gt 0 ]]; then
        log_info "Migrating $MQ_COUNT mq/ queue files..."

        # Ensure destination mq dir exists
        mkdir -p "$DEST_DIR/mq"

        # Process each matching file
        find "$SOURCE_DIR/mq" -name "${OLD_PREFIX}*.json" -print0 | while IFS= read -r -d '' file; do
            filename=$(basename "$file")
            # Replace prefix in filename
            new_filename="${filename/$OLD_PREFIX/$NEW_PREFIX}"

            # Update ID in file content and write to destination
            sed -E "s/\"id\":\"${OLD_PREFIX_ESC}/\"id\":\"${NEW_PREFIX_ESC}/g" "$file" > "$DEST_DIR/mq/$new_filename"

            # Remove original if in-place migration
            if [[ "$SOURCE_DIR" == "$DEST_DIR" ]]; then
                rm "$file"
            fi
        done

        log_info "Migrated mq/ files"
    else
        log_info "No mq/ files to migrate"
    fi
fi

# Handle database
if [[ -f "$SOURCE_DIR/beads.db" ]]; then
    if [[ "$SOURCE_DIR" == "$DEST_DIR" ]]; then
        log_info "Removing database (will regenerate from JSONL)..."
        rm -f "$SOURCE_DIR/beads.db" "$SOURCE_DIR/beads.db-shm" "$SOURCE_DIR/beads.db-wal"
    fi
    # For different dest, don't copy DB - it needs to be regenerated
fi

# Copy config files if migrating to new location
if [[ "$SOURCE_DIR" != "$DEST_DIR" ]]; then
    for config_file in config.yaml metadata.json README.md .gitignore; do
        if [[ -f "$SOURCE_DIR/$config_file" ]]; then
            cp "$SOURCE_DIR/$config_file" "$DEST_DIR/"
        fi
    done
    # Create formulas dir if exists
    if [[ -d "$SOURCE_DIR/formulas" ]]; then
        cp -r "$SOURCE_DIR/formulas" "$DEST_DIR/"
    fi
fi

# Verification
MIGRATED_COUNT=$(grep -c "\"id\":\"${NEW_PREFIX}" "$DEST_DIR/issues.jsonl" 2>/dev/null || echo "0")

log_info "Migration complete!"
log_info "  Issues migrated: $MATCHING_ISSUES -> $MIGRATED_COUNT with new prefix"

if [[ "$MATCHING_ISSUES" -ne "$MIGRATED_COUNT" ]]; then
    log_warn "Count mismatch! Expected $MATCHING_ISSUES, got $MIGRATED_COUNT"
fi

echo ""
log_info "Next steps:"
log_info "  1. cd $(dirname "$DEST_DIR")"
log_info "  2. bd init --reimport  # Rebuild database from JSONL"
log_info "  3. bd list             # Verify migration"

# If there's a routes.jsonl, remind about updating it
if [[ -f "$(dirname "$SOURCE_DIR")/../../../.beads/routes.jsonl" ]]; then
    log_info "  4. Update routes.jsonl if prefix routing needs to change"
fi
