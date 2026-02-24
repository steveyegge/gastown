---
title: "GT INIT"
---

## gt init

Initialize current directory as a Gas Town rig

### Synopsis

Initialize the current directory for use as a Gas Town rig.

This creates the standard agent directories (polecats/, witness/, refinery/,
mayor/) and updates .git/info/exclude to ignore them.

The current directory must be a git repository. Use --force to reinitialize
an existing rig structure.

```
gt init [flags]
```

### Options

```
  -f, --force   Reinitialize existing structure
  -h, --help    help for init
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

