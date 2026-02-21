// Package bdbranch implements a go/analysis analyzer that detects BD_BRANCH-relevant
// callsites and reports unprotected ones.
//
// The analyzer detects four patterns:
//   - beads.New() without .OnMain() chaining
//   - beads.NewWithBeadsDir() without .OnMain() chaining
//   - exec.Command("bd",...) — always reported for manual review
//   - exec.LookPath("bd") — always reported (proxy for syscall.Exec bd invocations)
//
// Protected callsites (beads.New().OnMain()) are silently accepted.
//
// Limitations:
//   - Only detects default import name "beads", not aliased imports
//   - Only detects "bd" as double-quoted string literal, not backtick or variable
//   - Does not detect indirect bd invocations via helper functions
//
// Usage:
//
//	go vet -vettool=$(which bdbranch-lint) ./internal/cmd/...
//
// See also: internal/cmd/bd_branch_arch_test.go for count-based registry enforcement.
package bdbranch

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "bdbranch",
	Doc:  "reports BD_BRANCH-relevant callsites needing safety review (#1796)",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		checkFile(pass, file)
	}
	return nil, nil
}

func checkFile(pass *analysis.Pass, file *ast.File) {
	// Phase 1: Find all .OnMain() receivers — these beads constructor calls are protected.
	protectedCalls := make(map[*ast.CallExpr]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// .OnMain() with zero args means the inner call (if it's beads.New) is protected.
		if sel.Sel.Name == "OnMain" && len(call.Args) == 0 {
			if innerCall, ok := sel.X.(*ast.CallExpr); ok {
				protectedCalls[innerCall] = true
			}
		}
		return true
	})

	// Phase 2: Report unprotected beads constructors and all bd exec patterns.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		switch {
		case ident.Name == "beads" && sel.Sel.Name == "New":
			if !protectedCalls[call] {
				pass.Reportf(call.Pos(), "beads.New() without .OnMain() — review for BD_BRANCH safety in polecat context (#1796)")
			}

		case ident.Name == "beads" && sel.Sel.Name == "NewWithBeadsDir":
			if !protectedCalls[call] {
				pass.Reportf(call.Pos(), "beads.NewWithBeadsDir() without .OnMain() — review for BD_BRANCH safety (#1796)")
			}

		case ident.Name == "exec" && sel.Sel.Name == "Command":
			if len(call.Args) > 0 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok &&
					lit.Kind == token.STRING && lit.Value == `"bd"` {
					pass.Reportf(call.Pos(), `exec.Command("bd",...) — ensure cmd.Env uses beads.StripBdBranch() for polecat reads (#1796)`)
				}
			}

		case ident.Name == "exec" && sel.Sel.Name == "LookPath":
			if len(call.Args) > 0 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok &&
					lit.Kind == token.STRING && lit.Value == `"bd"` {
					pass.Reportf(call.Pos(), `exec.LookPath("bd") — ensure syscall.Exec env uses beads.StripBdBranch() (#1796)`)
				}
			}
		}

		return true
	})
}
