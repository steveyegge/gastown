package cmd

import (
	"strings"
	"testing"
)

func TestBootSpawnAgentFlag(t *testing.T) {
	flag := bootSpawnCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected boot spawn to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}
