#!/bin/bash
# =============================================================================
# COMMON SHELL FUNCTIONS FOR GAS TOWN SCRIPTS
# =============================================================================
#
# Source this file in other scripts:
#   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#   source "$SCRIPT_DIR/lib/common.sh"
#
# =============================================================================

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect darwin system sed; darwin may still be using GNU sed.
is_darwin_sed() {
    [[ "$OSTYPE" == "darwin"* && "$(which sed)" == "/usr/bin/sed" ]]
}

# Cross-platform sed -i wrapper that handles macOS BSD sed vs GNU sed differences
# Automatically adds '' after -i on macOS BSD sed
# Usage: sed_i <sed expression> <file>
sed_i() {
    if is_darwin_sed; then
        sed -i '' "$@"
    else
        sed -i "$@"
    fi
}

# Update a file with sed (cross-platform compatible)
# Usage: update_file <file> <old_pattern> <new_text>
update_file() {
    local file=$1
    local old_pattern=$2
    local new_text=$3

    sed_i "s|$old_pattern|$new_text|g" "$file"
}
