package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// logUsagePath is the JSONL file where command usage is recorded.
var logUsagePath = filepath.Join(os.Getenv("HOME"), ".gt", "cmd-usage.jsonl")

// noLogCommands are top-level commands excluded from telemetry.
// These fire per-tool-use and would dominate the log.
var noLogCommands = map[string]bool{
	"tap":    true,
	"signal": true,
}

// logCommandUsage appends one JSONL line to ~/.gt/cmd-usage.jsonl.
// Fire-and-forget: all errors are silently ignored.
func logCommandUsage(cmd *cobra.Command, args []string) {
	// Walk up to the first subcommand under root to check exclusions.
	root := cmd
	for root.Parent() != nil && root.Parent().Parent() != nil {
		root = root.Parent()
	}
	if noLogCommands[root.Name()] {
		return
	}

	actor := os.Getenv("GT_ROLE")
	if actor == "" {
		actor = "unknown"
	}

	f, err := os.OpenFile(logUsagePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	ts := time.Now().Format(time.RFC3339)
	cmdPath := buildCommandPath(cmd)
	fmt.Fprintf(f, `{"ts":"%s","cmd":"%s","actor":"%s","argc":%d}`+"\n",
		ts, cmdPath, actor, len(args))
}
