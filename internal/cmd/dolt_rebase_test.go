package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestDoltRebase_DecimalScanForMinMax verifies that dolt_rebase.go scans
// MIN/MAX(rebase_order) as string then parses, not as int directly.
// Dolt returns decimal strings like "1.00" as []uint8 byte slices which
// cannot be scanned directly into int or float64.
func TestDoltRebase_DecimalScanForMinMax(t *testing.T) {
	content, err := os.ReadFile("dolt_rebase.go")
	if err != nil {
		t.Fatalf("reading dolt_rebase.go: %v", err)
	}

	src := string(content)

	// Must NOT scan directly as int
	if strings.Contains(src, "var minOrder, maxOrder int") {
		t.Error("dolt_rebase.go must not scan MIN/MAX(rebase_order) as int — Dolt returns decimal strings as []uint8")
	}
	// Must scan as string then parse
	if !strings.Contains(src, "var minOrderStr, maxOrderStr string") {
		t.Error("dolt_rebase.go must use string variables for MIN/MAX scan (Dolt returns []uint8 byte slices)")
	}
	if !strings.Contains(src, "strconv.ParseFloat(minOrderStr") {
		t.Error("dolt_rebase.go must parse string to float after scanning")
	}
	if !strings.Contains(src, "int(minOrderF)") {
		t.Error("dolt_rebase.go must cast float64 to int after parsing")
	}
}
