//go:build integration

package vision_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/ollama"
	"github.com/steveyegge/gastown/internal/vision"
)

// TestScreenshotCapture verifies that screencapture produces a non-empty PNG.
func TestScreenshotCapture(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "capture.png")
	path, err := vision.CaptureScreenshot(dst)
	if err != nil {
		t.Fatalf("CaptureScreenshot: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat screenshot: %v", err)
	}
	if info.Size() < 1000 {
		t.Fatalf("screenshot too small: %d bytes", info.Size())
	}
	t.Logf("screenshot captured: %s (%d bytes)", path, info.Size())
}

// TestVisionAnalysis captures a screenshot and sends it to gemma3:4b for description.
func TestVisionAnalysis(t *testing.T) {
	client := ollama.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		t.Skipf("Ollama not available: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "vision.png")
	path, err := vision.CaptureScreenshot(dst)
	if err != nil {
		t.Fatalf("CaptureScreenshot: %v", err)
	}

	desc, err := client.DescribeImage(ctx, "gemma3:4b", path, "Describe what you see in this screenshot. Be concise.")
	if err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if len(desc) < 10 {
		t.Fatalf("description too short: %q", desc)
	}
	t.Logf("vision description (%d chars): %s", len(desc), desc)
}

// TestOCR captures a screenshot and runs OCR to extract visible text.
func TestOCR(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "ocr.png")
	path, err := vision.CaptureScreenshot(dst)
	if err != nil {
		t.Fatalf("CaptureScreenshot: %v", err)
	}

	text, err := vision.OCR(path)
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	// The screen almost always has some text (menu bar, window titles, etc.)
	t.Logf("OCR extracted %d chars: %s", len(text), truncate(text, 500))
}

// TestVisualDiff takes two screenshots 1 second apart and compares them.
func TestVisualDiff(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "diff_a.png")
	pathB := filepath.Join(dir, "diff_b.png")

	_, err := vision.CaptureScreenshot(pathA)
	if err != nil {
		t.Fatalf("capture A: %v", err)
	}

	time.Sleep(1 * time.Second)

	_, err = vision.CaptureScreenshot(pathB)
	if err != nil {
		t.Fatalf("capture B: %v", err)
	}

	result, err := vision.VisualDiff(pathA, pathB)
	if err != nil {
		t.Fatalf("VisualDiff: %v", err)
	}

	t.Logf("visual diff: mean_delta=%.6f diff_pixels=%d/%d identical=%v",
		result.MeanDelta, result.DiffPixels, result.TotalPixels, result.Identical)

	// Two screenshots 1s apart on a mostly static desktop should have low delta
	if result.MeanDelta > 0.5 {
		t.Errorf("unexpectedly high mean delta: %.4f", result.MeanDelta)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
