#!/bin/sh
# Toolchain sidecar entrypoint: export tool binaries to the shared workspace
# volume so the agent container can use them directly (e.g., as LSP servers).
#
# Copies binaries to $WORKSPACE/.toolchain/bin/ which the agent entrypoint
# adds to PATH. Only copies binaries that exist in this image.

set -e

WORKSPACE="${GT_WORKSPACE:-/home/agent/gt}"
TOOLBIN="${WORKSPACE}/.toolchain/bin"

mkdir -p "${TOOLBIN}"

# List of binaries to export to the shared volume.
# These are tools the agent container needs to run directly (not via kubectl exec).
EXPORT_BINS="gopls rust-analyzer"

for bin in ${EXPORT_BINS}; do
    src=$(command -v "${bin}" 2>/dev/null) || continue
    if [ ! -f "${TOOLBIN}/${bin}" ] || [ "${src}" -nt "${TOOLBIN}/${bin}" ]; then
        cp "${src}" "${TOOLBIN}/${bin}"
        echo "[toolchain] Exported ${bin} to ${TOOLBIN}/"
    fi
done

echo "[toolchain] Ready ($(ls "${TOOLBIN}" | wc -l | tr -d ' ') binaries exported)"

# Stay alive for kubectl exec access.
exec sleep infinity
