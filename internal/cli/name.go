// Package cli provides CLI configuration utilities.
package cli

import (
	"os"
	"sync"
)

var (
	name     string
	nameOnce sync.Once
)

// Name returns the Gas Town CLI command name.
// Defaults to "gt", but can be overridden with GT_COMMAND env var.
// This allows coexistence with other tools that use "gt" (e.g., Graphite).
func Name() string {
	nameOnce.Do(func() {
		name = os.Getenv("GT_COMMAND")
		if name == "" {
			name = "gt"
		}
	})
	return name
}
