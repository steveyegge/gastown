package a

import (
	"beads"
	"os/exec"
)

func protectedWithOnMain() {
	// beads.New().OnMain() is protected — no diagnostic expected.
	b := beads.New("dir").OnMain()
	_ = b
}

func protectedNewWithBeadsDirOnMain() {
	// beads.NewWithBeadsDir().OnMain() is protected — no diagnostic expected.
	b := beads.NewWithBeadsDir("dir", "bd").OnMain()
	_ = b
}

func nonBdExecCommand() {
	// exec.Command with non-bd first arg — no diagnostic.
	a := exec.Command("git", "status")
	b := exec.Command("dolt", "sql")
	_ = a
	_ = b
}

func nonBdLookPath() {
	// exec.LookPath with non-bd arg — no diagnostic.
	a, _ := exec.LookPath("git")
	b, _ := exec.LookPath("tmux")
	_ = a
	_ = b
}

func beadsNewIsolated() {
	// NewIsolated is a test helper — no diagnostic.
	b := beads.NewIsolated("dir")
	_ = b
}

func stripBdBranchAlone() {
	// StripBdBranch by itself is not a callsite — no diagnostic.
	env := beads.StripBdBranch(nil)
	_ = env
}

func multipleProtected() {
	a := beads.New("d1").OnMain()
	b := beads.New("d2").OnMain()
	c := beads.New("d3").OnMain()
	_ = a
	_ = b
	_ = c
}
