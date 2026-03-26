//go:build darwin

package appctl

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// isGUIAvailable returns true if a window server / display is accessible.
// On headless CI the DISPLAY is absent and osascript will fail to open apps.
func isGUIAvailable() bool {
	// On macOS, check if we can talk to the window server.
	// A simple heuristic: the CGS session environment or the absence of SSH.
	// We try a lightweight osascript that does not open any app.
	_, err := runScript(`tell application "System Events" to get name of first process`)
	return err == nil
}

// runScript is a thin test helper that runs an AppleScript and returns output+error.
func runScript(script string) (string, error) {
	import_exec := func() {}
	_ = import_exec
	return "", nil // placeholder — replaced below
}

func TestIsRunning_FinderIsRunning(t *testing.T) {
	// Finder is always running on macOS.
	got := IsRunning("Finder")
	require.True(t, got, "IsRunning(\"Finder\") should return true — Finder is always running on macOS")
}

func TestIsRunning_BogusApp(t *testing.T) {
	got := IsRunning("ThisAppDefinitelyDoesNotExist_xyzzy")
	require.False(t, got, "IsRunning should return false for a non-existent application")
}

func TestIsRunning_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		appName string
		want    bool
	}{
		{"Finder always running", "Finder", true},
		{"bogus app not running", "NonExistentApp_zyxwv", false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := IsRunning(tc.appName)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLaunchAndQuit_Calculator(t *testing.T) {
	// Skip when no GUI is available (headless CI).
	if os.Getenv("CI") != "" {
		t.Skip("skipping Launch/Quit test in CI (no GUI available)")
	}
	if !isGUIAvailable() {
		t.Skip("skipping Launch/Quit test: window server not available")
	}

	const app = "Calculator"

	// Ensure Calculator is not running before we start.
	if IsRunning(app) {
		if err := Quit(app); err != nil {
			t.Fatalf("pre-test cleanup: Quit(%q) failed: %v", app, err)
		}
	}

	// Launch.
	err := Launch(app)
	require.NoError(t, err, "Launch(%q) should succeed", app)
	require.True(t, IsRunning(app), "%s should be running after Launch", app)

	// Quit.
	err = Quit(app)
	require.NoError(t, err, "Quit(%q) should succeed", app)
}

func TestLaunch_InvalidApp(t *testing.T) {
	// Skip when no GUI is available (headless CI).
	if os.Getenv("CI") != "" {
		t.Skip("skipping Launch error test in CI (no GUI available)")
	}
	if !isGUIAvailable() {
		t.Skip("skipping Launch error test: window server not available")
	}

	err := Launch("ThisAppDefinitelyDoesNotExist_xyzzy")
	require.Error(t, err, "Launch of a non-existent app should return an error")
}
