package a

import (
	"beads"
	"os/exec"
)

func unprotectedBeadsNew() {
	b := beads.New("dir") // want `beads\.New\(\) without \.OnMain\(\) — review for BD_BRANCH safety in polecat context \(#1796\)`
	_ = b
}

func unprotectedNewWithBeadsDir() {
	b := beads.NewWithBeadsDir("dir", "bd") // want `beads\.NewWithBeadsDir\(\) without \.OnMain\(\) — review for BD_BRANCH safety \(#1796\)`
	_ = b
}

func unprotectedExecCommand() {
	cmd := exec.Command("bd", "show", "id") // want `exec\.Command\("bd",\.\.\.\) — ensure cmd\.Env uses beads\.StripBdBranch\(\) for polecat reads \(#1796\)`
	_ = cmd
}

func unprotectedLookPath() {
	p, _ := exec.LookPath("bd") // want `exec\.LookPath\("bd"\) — ensure syscall\.Exec env uses beads\.StripBdBranch\(\) \(#1796\)`
	_ = p
}

func multipleUnprotected() {
	a := beads.New("d1")                    // want `beads\.New\(\) without \.OnMain\(\)`
	b := beads.New("d2")                    // want `beads\.New\(\) without \.OnMain\(\)`
	c := beads.NewWithBeadsDir("d3", "bd3") // want `beads\.NewWithBeadsDir\(\) without \.OnMain\(\)`
	cmd := exec.Command("bd", "list")       // want `exec\.Command\("bd",\.\.\.\)`
	_ = a
	_ = b
	_ = c
	_ = cmd
}
