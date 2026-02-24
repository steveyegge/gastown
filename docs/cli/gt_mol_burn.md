---
title: "DOCS/CLI/GT MOL BURN"
---

## gt mol burn

Burn current molecule without creating a digest

### Synopsis

Burn (destroy) the current molecule attachment.

This discards the molecule without creating a permanent record. Use this
when abandoning work or when a molecule doesn't need an audit trail.

If no target is specified, burns the current agent's attached molecule.

For wisps, burning is the default completion action. For regular molecules,
consider using 'squash' instead to preserve an audit trail.

```
gt mol burn [target] [flags]
```

### Options

```
  -h, --help   help for burn
      --json   Output as JSON
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

