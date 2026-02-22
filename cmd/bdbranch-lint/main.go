// Command bdbranch-lint runs the bdbranch analyzer as a standalone vet tool.
//
// Usage:
//
//	go install ./cmd/bdbranch-lint
//	go vet -vettool=$(which bdbranch-lint) ./internal/cmd/...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/steveyegge/gastown/internal/analysis/bdbranch"
)

func main() {
	singlechecker.Main(bdbranch.Analyzer)
}
