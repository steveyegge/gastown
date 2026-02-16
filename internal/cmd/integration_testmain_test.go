//go:build integration

package cmd

import (
	"flag"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Force sequential test execution to avoid bd file locks on Windows.
	_ = flag.Set("test.parallel", "1")
	flag.Parse()
	code := m.Run()
	// Clean up the shared dolt test server if one was started.
	cleanupDoltServer()
	os.Exit(code)
}
