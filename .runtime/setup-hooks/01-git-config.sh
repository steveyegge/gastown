#!/bin/bash
# Set git identity in polecat worktrees so commits are attributed to Ed on GitHub.
# Without this, commits fall back to system/global defaults which may vary.
set -euo pipefail

git -C "$GT_WORKTREE_PATH" config user.name "Ed Carrel"
git -C "$GT_WORKTREE_PATH" config user.email "ed@sazabi.ai"
