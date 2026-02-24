---
title: "DOCS/CLI/GT MOL STEP DONE"
---

## gt mol step done

Complete step and auto-continue to next

### Synopsis

Complete a molecule step and automatically continue to the next ready step.

This command handles the step-to-step transition for polecats:

1. Closes the completed step (bd close <step-id>)
2. Extracts the molecule ID from the step
3. Finds the next ready step (dependency-aware)
4. If next step exists:
   - Updates the hook to point to the next step
   - Respawns the pane for a fresh session
5. If molecule complete:
   - Clears the hook
   - Sends POLECAT_DONE to witness
   - Exits the session

IMPORTANT: This is the canonical way to complete molecule steps. Do NOT manually
close steps with 'bd close' - it skips the auto-continuation logic.

Example:
  gt mol step done gt-abc.1    # Complete step 1 of molecule gt-abc

```
gt mol step done <step-id> [flags]
```

### Options

```
  -n, --dry-run   Show what would be done without executing
  -h, --help      help for done
      --json      Output as JSON
```

### SEE ALSO

* [gt mol step](../cli/gt_mol_step/)	 - Molecule step operations

