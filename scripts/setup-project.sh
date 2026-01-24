#!/usr/bin/env bash
#
# setup-project.sh - Set up Gastown for a new project
#
# Usage: ./setup-project.sh <github-url>
#

set -e

[[ -z "$1" ]] && { echo "Usage: $(basename "$0") <github-url>"; exit 1; }

GITHUB_URL="$1"
TOWN_ROOT="${GT_TOWN_ROOT:-$HOME/gt}"
RIG_NAME=$(echo "$GITHUB_URL" | sed -E 's/.*[\/:]([^\/]+)(\.git)?$/\1/' | sed 's/\.git$//')
CREW_NAME=$(git config user.name | tr ' ' '-' | tr '[:upper:]' '[:lower:]')

[[ -z "$CREW_NAME" ]] && { echo "Error: git config user.name not set"; exit 1; }

command -v gt &>/dev/null || { echo "Error: gt not installed"; exit 1; }
command -v bd &>/dev/null || { echo "Error: bd not installed"; exit 1; }

echo "Setting up: $RIG_NAME (crew: $CREW_NAME)"

cd "$TOWN_ROOT" 2>/dev/null || { gt install "$TOWN_ROOT" --git && cd "$TOWN_ROOT"; }
[[ -d "$RIG_NAME" ]] || gt rig add "$RIG_NAME" "$GITHUB_URL"
[[ -d "$RIG_NAME/crew/$CREW_NAME" ]] || gt crew add "$CREW_NAME" --rig "$RIG_NAME"

echo ""
echo "Done! Run:"
echo "  cd $TOWN_ROOT/$RIG_NAME/crew/$CREW_NAME && claude"
