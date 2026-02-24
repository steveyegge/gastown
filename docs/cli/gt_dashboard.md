---
title: "DOCS/CLI/GT DASHBOARD"
---

## gt dashboard

Start the convoy tracking web dashboard

### Synopsis

Start a web server that displays the convoy tracking dashboard.

The dashboard shows real-time convoy status with:
- Convoy list with status indicators
- Progress tracking for each convoy
- Last activity indicator (green/yellow/red)
- Auto-refresh every 30 seconds via htmx

Example:
  gt dashboard              # Start on default port 8080
  gt dashboard --port 3000  # Start on port 3000
  gt dashboard --open       # Start and open browser

```
gt dashboard [flags]
```

### Options

```
  -h, --help       help for dashboard
      --open       Open browser automatically
      --port int   HTTP port to listen on (default 8080)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

