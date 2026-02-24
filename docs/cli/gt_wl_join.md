---
title: "GT WL JOIN"
---

## gt wl join

Join a wasteland by forking its commons

### Synopsis

Join a wasteland community by forking its shared commons database.

This command:
  1. Forks the upstream commons to your DoltHub org
  2. Clones the fork locally
  3. Registers your rig in the rigs table
  4. Pushes the registration to your fork
  5. Saves wasteland configuration locally

The upstream argument is a DoltHub path like 'steveyegge/wl-commons'.

Required environment variables:
  DOLTHUB_TOKEN  - Your DoltHub API token
  DOLTHUB_ORG    - Your DoltHub organization name

Examples:
  gt wl join steveyegge/wl-commons
  gt wl join steveyegge/wl-commons --handle my-rig
  gt wl join steveyegge/wl-commons --display-name "Alice's Workshop"

```
gt wl join <upstream> [flags]
```

### Options

```
      --display-name string   Display name for the rig registry
      --handle string         Rig handle for registration (default: DoltHub org)
  -h, --help                  help for join
```

### SEE ALSO

* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands

