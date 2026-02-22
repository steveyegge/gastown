package bdbranch_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/steveyegge/gastown/internal/analysis/bdbranch"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, testdataDir(t), bdbranch.Analyzer, "a")
}
