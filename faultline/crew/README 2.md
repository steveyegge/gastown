# Crew Directory

This directory contains crew worker workspaces.

## Adding a Crew Member

```bash
gt crew add <name>    # Creates crew/<name>/ with a git clone
```

## Crew vs Polecats

- **Crew**: Persistent, user-managed workspaces (never auto-garbage-collected)
- **Polecats**: Transient, witness-managed workers (cleaned up after work completes)

Use crew for your own workspace. Polecats are for batch work dispatch.
