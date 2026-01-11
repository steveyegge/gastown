#!/usr/bin/env python3
"""
Generate Towers of Hanoi formula with pre-computed moves.

Usage:
  python3 gen_hanoi.py [n_disks] > formula.toml

Examples:
  python3 gen_hanoi.py 7   # 127 moves (~19KB)
  python3 gen_hanoi.py 10  # 1023 moves (~149KB)
  python3 gen_hanoi.py 15  # 32767 moves (~4.7MB)
  python3 gen_hanoi.py 20  # 1048575 moves (~163MB)

The generated formula creates a sequential workflow where each move
depends on the previous one. This tests Gas Town's ability to:
- Create large molecule hierarchies
- Execute sequential workflows across session boundaries
- Maintain state through crash recovery (idempotence)
"""

import sys

def hanoi_moves(n, source='A', target='C', auxiliary='B'):
    """Generate all moves for n-disk Hanoi using iterative algorithm."""
    moves = []
    def solve(n, src, tgt, aux):
        if n == 1:
            moves.append((1, src, tgt))
        else:
            solve(n - 1, src, aux, tgt)
            moves.append((n, src, tgt))
            solve(n - 1, aux, tgt, src)
    solve(n, source, target, auxiliary)
    return moves

def generate_formula(n_disks):
    moves = hanoi_moves(n_disks)
    total = len(moves)

    print(f'''description = """
AGENT EXECUTION PROTOCOL - Towers of Hanoi ({n_disks} disks, {total} moves)

PURPOSE: This is a durability proof, not computation. Steps are pre-computed.
Your job is to execute them mechanically, proving crash-recovery at scale.

EXECUTION LOOP:
1. Find current state: bd mol current <mol-id>
2. Find the next ready (unblocked) step
3. Close it: bd close <step-id>
4. Repeat until no ready steps remain

ON RESUME (after crash/restart/handoff):
- Same as fresh start. The molecule IS the state.
- Query `bd mol current <mol-id>`, continue from there.
- No memory of previous session needed.

DO:
- Close steps as fast as possible (they're trivial mechanical moves)
- Use `gt handoff` when context fills (proactive cycling)
- Trust the pre-computed solution - every move is already correct

DO NOT:
- Try to solve Hanoi yourself - moves are already computed
- Mark steps in_progress - just close them directly
- Ask for permission - this is GUPP territory, just execute
- Stop for human input - run autonomously until complete

MONITORING:
- Progress: Count closed children of the molecule
- For mega-molecules: Use convoy dashboard when available
- Completion: All steps closed = molecule complete

This proves Gas Town can execute arbitrarily long workflows with
nondeterministic idempotence - different sessions, same outcome.
"""
formula = "towers-of-hanoi-{n_disks}"
version = 1

[[steps]]
id = "setup"
title = "Verify initial state"
description = "All {n_disks} disks stacked on peg A. Largest on bottom."
''')

    for i, (disk, src, tgt) in enumerate(moves, 1):
        prev = "setup" if i == 1 else f"move-{i-1}"
        print(f'''
[[steps]]
id = "move-{i}"
title = "Move disk {disk}: {src} → {tgt}"
description = "Move disk {disk} from peg {src} to peg {tgt}. (Move {i}/{total})"
needs = ["{prev}"]''')

    print(f'''
[[steps]]
id = "verify"
title = "Verify final state"
description = "All {n_disks} disks now on peg C. Tower intact, all moves were legal."
needs = ["move-{total}"]''')

if __name__ == "__main__":
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 10
    generate_formula(n)
