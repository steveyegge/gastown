package a

import (
	"beads"
	"os/exec"
)

func bdVariableArg() {
	// Variable arg to exec.Command — not a string literal, no diagnostic.
	prog := "bd"
	cmd := exec.Command(prog, "show")
	_ = cmd
}

func bareBeadsNewWithoutChain() {
	// Standalone beads.New() without any chained call — unprotected.
	b := beads.New("dir") // want `beads\.New\(\) without \.OnMain\(\)`
	_ = b
}

func mixedProtectedAndNot() {
	// First call protected, second not.
	a := beads.New("d1").OnMain()
	b := beads.New("d2") // want `beads\.New\(\) without \.OnMain\(\)`
	_ = a
	_ = b
}

func execCommandEmptyString() {
	// exec.Command with empty string — not "bd", no diagnostic.
	cmd := exec.Command("")
	_ = cmd
}

func bdBacktickString() {
	// Backtick-quoted "bd" — scanner only matches double-quoted, no diagnostic.
	// This is a documented limitation shared with W-009.
	cmd := exec.Command(`bd`, "show")
	_ = cmd
}

func multiLineOnMainChain() {
	// Multi-line .OnMain() chaining — AST still chains correctly, no diagnostic.
	b := beads.New("dir").
		OnMain()
	_ = b
}
