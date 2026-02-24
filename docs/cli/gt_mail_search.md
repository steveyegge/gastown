---
title: "DOCS/CLI/GT MAIL SEARCH"
---

## gt mail search

Search messages by content

### Synopsis

Search inbox for messages matching a pattern.

SYNTAX:
  gt mail search <query> [flags]

The query is a regular expression pattern. Search is case-insensitive by default.

FLAGS:
  --from <sender>   Filter by sender address (substring match)
  --subject         Only search subject lines
  --body            Only search message body
  --archive         Include archived (closed) messages
  --json            Output as JSON

By default, searches both subject and body text.

Examples:
  gt mail search "urgent"                    # Find messages with "urgent"
  gt mail search "status.*check" --subject   # Regex in subjects only
  gt mail search "error" --from witness      # From witness, containing "error"
  gt mail search "handoff" --archive         # Include archived messages
  gt mail search "" --from mayor/            # All messages from mayor

```
gt mail search <query> [flags]
```

### Options

```
      --archive       Include archived messages
      --body          Only search message body
      --from string   Filter by sender address
  -h, --help          help for search
      --json          Output as JSON
      --subject       Only search subject lines
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

