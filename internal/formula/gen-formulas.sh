#!/bin/sh
# Populate internal/formula/formulas/ from canonical .beads/formulas/ source.
# Supports .beads/redirect (Gas Town local workspace pattern): if .beads/ contains
# a redirect file, the actual beads directory is resolved relative to the rig root.
set -e

# This script runs from the directory of embed.go (internal/formula/).
RIG=$(cd ../.. && pwd)
BEADS="${RIG}/.beads"

if [ -f "${BEADS}/redirect" ]; then
    REDIR=$(cat "${BEADS}/redirect")
    BEADS="${RIG}/${REDIR}"
fi

rm -rf formulas
mkdir -p formulas
cp "${BEADS}/formulas/"*.formula.toml formulas/
