---
title: "GT THEME CLI"
---

## gt theme cli

View or set CLI color scheme (dark/light/auto)

### Synopsis

Manage CLI output color scheme for Gas Town commands.

Without arguments, shows the current CLI theme mode and detection.
With a mode argument, sets the CLI theme preference.

Modes:
  auto   - Automatically detect terminal background (default)
  dark   - Force dark mode colors (light text for dark backgrounds)
  light  - Force light mode colors (dark text for light backgrounds)

The setting is stored in town settings (settings/config.json) and can
be overridden per-session via the GT_THEME environment variable.

Examples:
  gt theme cli              # Show current CLI theme
  gt theme cli dark         # Set CLI theme to dark mode
  gt theme cli auto         # Reset to auto-detection
  GT_THEME=light gt status  # Override for a single command

```
gt theme cli [mode] [flags]
```

### Options

```
  -h, --help   help for cli
```

### SEE ALSO

* [gt theme](../cli/gt_theme/)	 - View or set tmux theme for the current rig

