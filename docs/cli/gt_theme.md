---
title: "DOCS/CLI/GT THEME"
---

## gt theme

View or set tmux theme for the current rig

### Synopsis

Manage tmux status bar themes for Gas Town sessions.

Without arguments, shows the current theme assignment.
With a name argument, sets the theme for this rig.

Examples:
  gt theme              # Show current theme
  gt theme --list       # List available themes
  gt theme forest       # Set theme to 'forest'
  gt theme apply        # Apply theme to all running sessions in this rig

```
gt theme [name] [flags]
```

### Options

```
  -h, --help   help for theme
  -l, --list   List available themes
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt theme apply](../cli/gt_theme_apply/)	 - Apply theme to running sessions
* [gt theme cli](../cli/gt_theme_cli/)	 - View or set CLI color scheme (dark/light/auto)

