#!/bin/bash
set -e

# =============================================================================
# UPDATE FLAKE VENDOR HASH SCRIPT FOR GAS TOWN
# =============================================================================
#
# This script computes and updates the vendorHash in flake.nix for the Go module.
# It uses nix to build a vendor directory and compute the hash.
#
# USAGE:
#   ./scripts/update-nix-flake.sh
#
# =============================================================================

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

# Main function
main() {
    # Check if we're in the repo root
    if [ ! -f "flake.nix" ]; then
        echo -e "${RED}Error: Must run from repository root (flake.nix not found)${NC}"
        exit 1
    fi

    if ! command -v nix &> /dev/null; then
        echo -e "${RED}Error: nix not found${NC}"
        exit 1
    fi

    echo "Computing vendorHash..."

    # Get current vendor hash
    local current_hash
    current_hash=$(grep 'vendorHash = ' flake.nix | sed 's/.*"\(.*\)".*/\1/')
    echo "  Current: $current_hash"

    # Create a temporary directory for the vendor output
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf $tmp_dir" EXIT

    # Use go mod vendor to create a vendor directory
    echo "  Downloading Go modules..."
    GOFLAGS="-mod=readonly" go mod vendor -o "$tmp_dir/vendor" 2>/dev/null || go mod vendor -o "$tmp_dir/vendor"

    # Compute the hash using nix hash path
    echo "  Computing hash..."
    local new_hash
    new_hash=$(nix hash path "$tmp_dir/vendor" 2>/dev/null)

    if [ -z "$new_hash" ]; then
        echo -e "${RED}Error: Could not compute vendorHash${NC}"
        exit 1
    fi

    # Update flake.nix with the new hash
    update_file "flake.nix" \
        "vendorHash = \"$current_hash\"" \
        "vendorHash = \"$new_hash\""

    if [ "$current_hash" = "$new_hash" ]; then
        echo -e "${GREEN}✓ vendorHash unchanged: $new_hash${NC}"
    else
        echo -e "${GREEN}✓ vendorHash updated${NC}"
        echo "  Old: $current_hash"
        echo "  New: $new_hash"
    fi
}

main "$@"
