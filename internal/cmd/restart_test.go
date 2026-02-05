package cmd

import (
	"testing"
)

func TestRestartCmd_Registered(t *testing.T) {
	// Verify restart command is registered
	cmd, _, err := rootCmd.Find([]string{"restart"})
	if err != nil {
		t.Fatalf("restart command not found: %v", err)
	}
	if cmd.Name() != "restart" {
		t.Errorf("expected command name 'restart', got %q", cmd.Name())
	}
}

func TestRestartCmd_Flags(t *testing.T) {
	// Verify flags are registered
	flags := restartCmd.Flags()

	if flags.Lookup("quiet") == nil {
		t.Error("--quiet flag not registered")
	}
	if flags.Lookup("restore") == nil {
		t.Error("--restore flag not registered")
	}
	if flags.Lookup("polecats") == nil {
		t.Error("--polecats flag not registered")
	}
}

func TestRestartCmd_ShortFlags(t *testing.T) {
	// Verify short flags
	flags := restartCmd.Flags()

	if flags.ShorthandLookup("q") == nil {
		t.Error("-q short flag not registered")
	}
	if flags.ShorthandLookup("p") == nil {
		t.Error("-p short flag not registered")
	}
}

func TestDownOptions_FromFlags(t *testing.T) {
	// Save original values
	savedQuiet := downQuiet
	savedForce := downForce
	savedAll := downAll
	savedNuke := downNuke
	savedDryRun := downDryRun
	savedPolecats := downPolecats

	// Restore after test
	defer func() {
		downQuiet = savedQuiet
		downForce = savedForce
		downAll = savedAll
		downNuke = savedNuke
		downDryRun = savedDryRun
		downPolecats = savedPolecats
	}()

	// Set test values
	downQuiet = true
	downForce = true
	downAll = true
	downNuke = false
	downDryRun = true
	downPolecats = true

	opts := downOptionsFromFlags()

	if opts.Quiet != true {
		t.Error("Quiet should be true")
	}
	if opts.Force != true {
		t.Error("Force should be true")
	}
	if opts.All != true {
		t.Error("All should be true")
	}
	if opts.Nuke != false {
		t.Error("Nuke should be false")
	}
	if opts.DryRun != true {
		t.Error("DryRun should be true")
	}
	if opts.Polecats != true {
		t.Error("Polecats should be true")
	}
}

func TestUpOptions_FromFlags(t *testing.T) {
	// Save original values
	savedQuiet := upQuiet
	savedRestore := upRestore

	// Restore after test
	defer func() {
		upQuiet = savedQuiet
		upRestore = savedRestore
	}()

	// Set test values
	upQuiet = true
	upRestore = true

	opts := upOptionsFromFlags()

	if opts.Quiet != true {
		t.Error("Quiet should be true")
	}
	if opts.Restore != true {
		t.Error("Restore should be true")
	}
}

func TestUpCmd_NoRestartFlag(t *testing.T) {
	// Verify --restart flag was removed from gt up
	flags := upCmd.Flags()

	if flags.Lookup("restart") != nil {
		t.Error("--restart flag should not exist on gt up (use gt restart instead)")
	}
}
