---
title: "DOCS/CLI/GT CONFIG AGENT-EMAIL-DOMAIN"
---

## gt config agent-email-domain

Get or set agent email domain

### Synopsis

Get or set the domain used for agent git commit emails.

When agents commit code via 'gt commit', their identity is converted
to a git email address. For example, "gastown/crew/jack" becomes
"gastown.crew.jack@{domain}".

With no arguments, shows the current domain.
With an argument, sets the domain.

Default: gastown.local

Examples:
  gt config agent-email-domain                 # Show current domain
  gt config agent-email-domain gastown.local   # Set to gastown.local
  gt config agent-email-domain example.com     # Set custom domain

```
gt config agent-email-domain [domain] [flags]
```

### Options

```
  -h, --help   help for agent-email-domain
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

