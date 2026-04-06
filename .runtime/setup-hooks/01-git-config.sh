#!/bin/bash
# Propagate global git identity into polecat worktrees.
# Without this, commits may fall back to system defaults and be misattributed.
set -euo pipefail

name=$(git config --global user.name 2>/dev/null || true)
email=$(git config --global user.email 2>/dev/null || true)

[ -n "$name" ]  && git -C "$GT_WORKTREE_PATH" config user.name  "$name"
[ -n "$email" ] && git -C "$GT_WORKTREE_PATH" config user.email "$email"
