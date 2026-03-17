package daemon

import (
	"os"
	"strings"
	"testing"
)

// TestCompactorDog_FloatScanForMinMax verifies that compactor_dog.go scans
// MIN/MAX(rebase_order) as float64, not int. Dolt returns decimal strings
// like "1.00" which cannot be scanned directly into int.
func TestCompactorDog_FloatScanForMinMax(t *testing.T) {
	content, err := os.ReadFile("compactor_dog.go")
	if err != nil {
		t.Fatalf("reading compactor_dog.go: %v", err)
	}

	src := string(content)

	// Must scan as float64, not int
	if strings.Contains(src, "var minOrder, maxOrder int") {
		t.Error("compactor_dog.go must scan MIN/MAX(rebase_order) as float64, not int — Dolt returns decimal strings")
	}
	if !strings.Contains(src, "var minOrderF, maxOrderF float64") {
		t.Error("compactor_dog.go must use float64 variables for MIN/MAX scan")
	}
	if !strings.Contains(src, "int(minOrderF)") {
		t.Error("compactor_dog.go must cast float64 to int after scanning")
	}
}
