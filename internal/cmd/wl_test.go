package cmd

import (
	"testing"
)

func TestWlCommandRegistered(t *testing.T) {
	// Verify the wl command is registered on the root command
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "wl" {
			found = true
			break
		}
	}
	if !found {
		t.Error("wl command not found on rootCmd")
	}
}

func TestWlJoinSubcommand(t *testing.T) {
	// Verify join is a subcommand of wl
	found := false
	for _, c := range wlCmd.Commands() {
		if c.Name() == "join" {
			found = true
			// Verify it requires exactly 1 arg
			if err := c.Args(c, []string{}); err == nil {
				t.Error("join should require exactly 1 argument")
			}
			if err := c.Args(c, []string{"org/db"}); err != nil {
				t.Errorf("join should accept 1 argument: %v", err)
			}
			break
		}
	}
	if !found {
		t.Error("join subcommand not found on wl command")
	}
}

func TestWlCommandGroup(t *testing.T) {
	if wlCmd.GroupID != GroupWork {
		t.Errorf("wl command GroupID = %q, want %q", wlCmd.GroupID, GroupWork)
	}
}
