package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Events in this map must have a corresponding events.LogFeed call in command
// functions that log them. Events omitted here are intentionally exempt.
var townlogEventsRequiringFeed = map[string]struct{}{
	"EventSpawn":          {},
	"EventWake":           {},
	"EventNudge":          {},
	"EventHandoff":        {},
	"EventDone":           {},
	"EventCrash":          {},
	"EventKill":           {},
	"EventPatrolStarted":  {},
	"EventPolecatChecked": {},
	"EventPolecatNudged":  {},
	"EventEscalationSent": {},
	"EventPatrolComplete": {},
}

var townlogHelpersRequiringFeed = map[string]struct{}{
	"LogSpawn":   {},
	"LogWake":    {},
	"LogNudge":   {},
	"LogHandoff": {},
	"LogDone":    {},
	"LogCrash":   {},
	"LogKill":    {},
}

type functionCoverage struct {
	feedCount       int
	transitionCalls []string
}

func eventCoverageTestDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Dir(filename)
}

func requiredTownlogEventName(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "townlog" {
		return ""
	}
	if _, ok := townlogEventsRequiringFeed[sel.Sel.Name]; !ok {
		return ""
	}
	return sel.Sel.Name
}

func analyzeFunctionCoverage(fn *ast.FuncDecl) functionCoverage {
	var cov functionCoverage
	if fn == nil || fn.Body == nil {
		return cov
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok && x.Name == "events" && fun.Sel.Name == "LogFeed" {
				cov.feedCount++
				return true
			}

			// Direct townlog logger.Log(townlog.EventX, ...)
			if fun.Sel.Name == "Log" && len(call.Args) > 0 {
				if ev := requiredTownlogEventName(call.Args[0]); ev != "" {
					cov.transitionCalls = append(cov.transitionCalls, "townlog."+ev)
				}
			}

		case *ast.Ident:
			// Helper wrappers (LogDone, LogHandoff, etc.)
			if _, ok := townlogHelpersRequiringFeed[fun.Name]; ok {
				cov.transitionCalls = append(cov.transitionCalls, fun.Name)
				return true
			}

			// Generic wrappers with explicit event arg.
			if fun.Name == "LogEvent" && len(call.Args) > 0 {
				if ev := requiredTownlogEventName(call.Args[0]); ev != "" {
					cov.transitionCalls = append(cov.transitionCalls, "LogEvent("+ev+")")
				}
				return true
			}
			if fun.Name == "LogEventWithRoot" && len(call.Args) > 1 {
				if ev := requiredTownlogEventName(call.Args[1]); ev != "" {
					cov.transitionCalls = append(cov.transitionCalls, "LogEventWithRoot("+ev+")")
				}
				return true
			}
		}
		return true
	})

	return cov
}

func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 || len(recv.List[0].Names) == 0 {
		return ""
	}
	return recv.List[0].Names[0].Name
}

// TestEventCoverageTownlogRequiresFeed enforces an event-first contract:
// command functions that log state transitions to townlog must also emit at
// least one feed-visible event via events.LogFeed.
func TestEventCoverageTownlogRequiresFeed(t *testing.T) {
	dir := eventCoverageTestDir(t)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse dir %s: %v", dir, err)
	}

	var violations []string
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			base := filepath.Base(filename)
			// log.go defines helper wrappers; the call-sites using these helpers are
			// checked in their own files.
			if base == "log.go" {
				continue
			}

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				cov := analyzeFunctionCoverage(fn)
				if len(cov.transitionCalls) == 0 {
					continue
				}
				if cov.feedCount > 0 {
					continue
				}

				qualified := fn.Name.Name
				if recv := receiverName(fn.Recv); recv != "" {
					qualified = recv + "." + qualified
				}
				sort.Strings(cov.transitionCalls)
				violations = append(violations, fmt.Sprintf(
					"%s:%s logs transitions (%s) but has no events.LogFeed call",
					base,
					qualified,
					strings.Join(cov.transitionCalls, ", "),
				))
			}
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("event coverage violations:\n  %s", strings.Join(violations, "\n  "))
	}
}
