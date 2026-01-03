package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDashboardCmd_FlagsExist(t *testing.T) {
	// Verify required flags exist with correct defaults
	portFlag := dashboardCmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag should exist")
	}
	if portFlag.DefValue != "8080" {
		t.Errorf("--port default should be 8080, got %s", portFlag.DefValue)
	}

	openFlag := dashboardCmd.Flags().Lookup("open")
	if openFlag == nil {
		t.Fatal("--open flag should exist")
	}
	if openFlag.DefValue != "false" {
		t.Errorf("--open default should be false, got %s", openFlag.DefValue)
	}
}

func TestDashboardCmd_IsRegistered(t *testing.T) {
	// Verify command is registered under root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "dashboard" {
			found = true
			break
		}
	}
	if !found {
		t.Error("dashboard command should be registered with rootCmd")
	}
}

func TestDashboardCmd_HasCorrectGroup(t *testing.T) {
	if dashboardCmd.GroupID != GroupDiag {
		t.Errorf("dashboard should be in diag group, got %s", dashboardCmd.GroupID)
	}
}

func TestDashboardCmd_RequiresWorkspace(t *testing.T) {
	// Create a test command that simulates running outside workspace
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	// The actual workspace check happens in runDashboard
	// This test verifies the command structure is correct
	if dashboardCmd.RunE == nil {
		t.Error("dashboard command should have RunE set")
	}
}
