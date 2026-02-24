---
title: "GT COMPLETION BASH"
---

## gt completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(gt completion bash)

To load completions for every new session, execute once:

#### Linux:

	gt completion bash > /etc/bash_completion.d/gt

#### macOS:

	gt completion bash > $(brew --prefix)/etc/bash_completion.d/gt

You will need to start a new shell for this setup to take effect.


```
gt completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### SEE ALSO

* [gt completion](../cli/gt_completion/)	 - Generate the autocompletion script for the specified shell

