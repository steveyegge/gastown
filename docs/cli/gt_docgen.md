---
title: "GT DOCGEN"
---

## gt docgen

Generate Markdown documentation for gt CLI commands

### Synopsis

docgen generates Markdown documentation for all gt CLI commands.
It uses the cobra/doc package to produce structured documentation.

```
gt docgen [flags]
```

### Options

```
  -f, --format string   Output format (markdown, man, rest) (default "markdown")
      --frontmatter     Include frontmatter in markdown output (default true)
  -h, --help            help for docgen
  -o, --out string      Output directory for generated docs (default "./docs/cli")
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

