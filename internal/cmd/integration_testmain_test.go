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
	os.Exit(m.Run())
}
