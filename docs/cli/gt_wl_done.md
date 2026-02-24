---
title: "DOCS/CLI/GT WL DONE"
---

## gt wl done

Submit completion evidence for a wanted item

### Synopsis

Submit completion evidence for a claimed wanted item.

Inserts a completion record and updates the wanted item status to 'in_review'.
The item must be claimed by your rig.

The --evidence flag provides the evidence URL (PR link, commit hash, etc.).

A completion ID is generated as c-<hash> where hash is derived from the
wanted ID, rig handle, and timestamp.

Examples:
  gt wl done w-abc123 --evidence 'https://github.com/org/repo/pull/123'
  gt wl done w-abc123 --evidence 'commit abc123def'

```
gt wl done <wanted-id> [flags]
```

### Options

```
      --evidence string   Evidence URL or description (required)
  -h, --help              help for done
```

### SEE ALSO

* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands

