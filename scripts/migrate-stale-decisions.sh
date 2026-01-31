#!/bin/bash
# migrate-stale-decisions.sh
# Migration script to clean up stale pending decisions
#
# Background: When a decision bead was closed with `bd close`, the decision_points
# table's responded_at field was not updated. This left closed decisions appearing
# in `bd decision list` as pending.
#
# This script:
# 1. Finds all decision_points where responded_at IS NULL AND issue is closed
# 2. Updates responded_at to match the issue's closed_at timestamp
#
# Fixed in: gt-bug-bd_decision_list_shows_closed_decisions
# Tracked in: gt-tsk-clean_up_stale_pending_decisions

set -e

# Default database path (central dolt server location)
BEADS_DOLT_PATH="${BEADS_DOLT_PATH:-$HOME/.beads-dolt/beads}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=============================================="
echo "Stale Decision Migration Script"
echo "=============================================="
echo ""

# Check if dolt is available
if ! command -v dolt &> /dev/null; then
    echo -e "${RED}Error: dolt command not found${NC}"
    exit 1
fi

# Check if the database directory exists
if [ ! -d "$BEADS_DOLT_PATH" ]; then
    echo -e "${RED}Error: Database directory not found: $BEADS_DOLT_PATH${NC}"
    exit 1
fi

cd "$BEADS_DOLT_PATH"

echo "Database path: $BEADS_DOLT_PATH"
echo ""

# Count stale decisions before migration
echo "Analyzing stale decisions..."
stale_count=$(dolt sql -r csv -q "
SELECT COUNT(*) as cnt
FROM decision_points dp
JOIN issues i ON dp.issue_id = i.id
WHERE dp.responded_at IS NULL AND i.status = 'closed'
" | tail -n 1)

echo -e "Found ${YELLOW}${stale_count}${NC} stale decisions (pending with closed issues)"
echo ""

if [ "$stale_count" -eq 0 ]; then
    echo -e "${GREEN}No stale decisions found. Nothing to migrate.${NC}"
    exit 0
fi

# Show sample of what will be updated
echo "Sample of decisions to be updated:"
echo ""
dolt sql -q "
SELECT dp.issue_id, i.closed_at, dp.created_at
FROM decision_points dp
JOIN issues i ON dp.issue_id = i.id
WHERE dp.responded_at IS NULL AND i.status = 'closed'
ORDER BY i.closed_at DESC
LIMIT 5
"
echo ""

# Prompt for confirmation (unless --yes flag is passed)
if [ "$1" != "--yes" ] && [ "$1" != "-y" ]; then
    read -p "Do you want to proceed with the migration? [y/N] " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Migration cancelled."
        exit 0
    fi
fi

echo ""
echo "Running migration..."

# Perform the update
# Set responded_at to the issue's closed_at timestamp
# Set responded_by to 'migration' to indicate this was a cleanup
result=$(dolt sql -q "
UPDATE decision_points dp
SET
    responded_at = (
        SELECT i.closed_at
        FROM issues i
        WHERE i.id = dp.issue_id
    ),
    responded_by = 'migration:stale-cleanup'
WHERE dp.responded_at IS NULL
AND EXISTS (
    SELECT 1 FROM issues i
    WHERE i.id = dp.issue_id
    AND i.status = 'closed'
);
" 2>&1)

echo "$result"

# Commit the changes
echo ""
echo "Committing changes to dolt..."
dolt add .
dolt commit -m "migration: clean up stale pending decisions

Updated responded_at for ${stale_count} decision_points where:
- responded_at was NULL (pending)
- corresponding issue was closed

Set responded_at to issue's closed_at timestamp.
Set responded_by to 'migration:stale-cleanup'.

Tracking: gt-tsk-clean_up_stale_pending_decisions
Root cause fix: gt-bug-bd_decision_list_shows_closed_decisions"

# Verify the migration
echo ""
echo "Verifying migration..."
remaining=$(dolt sql -r csv -q "
SELECT COUNT(*) as cnt
FROM decision_points dp
JOIN issues i ON dp.issue_id = i.id
WHERE dp.responded_at IS NULL AND i.status = 'closed'
" | tail -n 1)

echo ""
if [ "$remaining" -eq 0 ]; then
    echo -e "${GREEN}Migration complete!${NC}"
    echo -e "Updated: ${stale_count} decisions"
    echo -e "Remaining stale: 0"
else
    echo -e "${YELLOW}Migration completed with some remaining stale decisions${NC}"
    echo -e "Updated: $((stale_count - remaining)) decisions"
    echo -e "Remaining stale: ${remaining}"
fi

# Show current pending decision count
pending_count=$(dolt sql -r csv -q "SELECT COUNT(*) FROM decision_points WHERE responded_at IS NULL" | tail -n 1)
echo ""
echo "Current pending decisions (genuinely awaiting response): ${pending_count}"
