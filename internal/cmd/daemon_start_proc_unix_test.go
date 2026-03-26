//go:build !windows

package cmd

import (
	"testing"
)

func TestBuildDaemonStartCommand_UsesDetachedSession(t *testing.T) {
	cmd := buildDaemonStartCommand("/tmp/fake-gt", "/tmp/fake-town")

	if got, want := cmd.Dir, "/tmp/fake-town"; got != want {
		t.Fatalf("cmd.Dir = %q, want %q", got, want)
	}
	if got, want := cmd.Path, "/tmp/fake-gt"; got != want {
		t.Fatalf("cmd.Path = %q, want %q", got, want)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "daemon" || cmd.Args[2] != "run" {
		t.Fatalf("cmd.Args = %#v, want daemon run invocation", cmd.Args)
	}
	if cmd.Stdout != nil || cmd.Stderr != nil || cmd.Stdin != nil {
		t.Fatal("buildDaemonStartCommand() should discard stdio")
	}
	if cmd.SysProcAttr == nil {
		t.Fatal("buildDaemonStartCommand() did not configure SysProcAttr")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Fatal("buildDaemonStartCommand() must detach with Setsid on Unix")
	}
}
