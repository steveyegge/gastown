---
title: "GT OVERRIDE"
---

## gt override

Manage role prompt overrides

### Synopsis

Manage local overrides for role prompts.

Overrides allow you to customize role prompts (mayor, witness, refinery, etc.)
without modifying the gastown source code. Overrides are stored in your town
root and tracked in git.

Override path: $GT_TOWN_ROOT/.gt/overrides/{role}.md.tmpl

Examples:
  gt override list                    # List all active overrides
  gt override show mayor              # Show mayor override content
  gt override edit mayor              # Edit mayor override in $EDITOR
  gt override create mayor            # Create new mayor override from template
  gt override delete mayor            # Remove mayor override (use embedded)
  gt override diff mayor              # Compare override with embedded template

```
gt override [flags]
```

### Options

```
  -h, --help   help for override
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt override create](../cli/gt_override_create/)	 - Create new override from embedded template
* [gt override delete](../cli/gt_override_delete/)	 - Remove override (revert to embedded)
* [gt override diff](../cli/gt_override_diff/)	 - Compare override with embedded template
* [gt override edit](../cli/gt_override_edit/)	 - Edit override in $EDITOR
* [gt override list](../cli/gt_override_list/)	 - List active overrides
* [gt override show](../cli/gt_override_show/)	 - Show override content

