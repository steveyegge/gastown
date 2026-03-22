// Package vision provides screenshot capture, OCR, and visual diff on macOS.
package vision

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CaptureScreenshot takes a screenshot using macOS screencapture and saves it to dst.
// If dst is empty, a temp file is created.
func CaptureScreenshot(dst string) (string, error) {
	if dst == "" {
		f, err := os.CreateTemp("", "screenshot-*.png")
		if err != nil {
			return "", fmt.Errorf("create temp file: %w", err)
		}
		dst = f.Name()
		f.Close()
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create dir %s: %w", dir, err)
	}

	// -x suppresses the shutter sound
	cmd := exec.Command("screencapture", "-x", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("screencapture failed: %w: %s", err, string(out))
	}

	info, err := os.Stat(dst)
	if err != nil {
		return "", fmt.Errorf("stat screenshot: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("screenshot file is empty")
	}
	return dst, nil
}

// OCR runs basic text extraction from a screenshot using macOS shortcuts command
// and the Vision framework via a small inline Swift script. Falls back to
// describing pixel dimensions if the system doesn't support it.
func OCR(imagePath string) (string, error) {
	// Use macOS Vision framework via swift inline script
	script := fmt.Sprintf(`
import Foundation
import Vision
import AppKit

let url = URL(fileURLWithPath: "%s")
guard let image = NSImage(contentsOf: url),
      let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    fputs("error: cannot load image\n", stderr)
    exit(1)
}

let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate

let handler = VNImageRequestHandler(cgImage: cgImage)
try handler.perform([request])

let results = request.results ?? []
for observation in results {
    if let candidate = observation.topCandidates(1).first {
        print(candidate.string)
    }
}
`, imagePath)

	cmd := exec.Command("swift", "-")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("OCR swift script failed: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// DiffResult holds the outcome of a visual comparison between two images.
type DiffResult struct {
	// MeanDelta is the average per-pixel difference (0.0 = identical, 1.0 = max diff).
	MeanDelta float64
	// DiffPixels is the number of pixels that differ beyond the threshold.
	DiffPixels int
	// TotalPixels is the total number of pixels compared.
	TotalPixels int
	// Identical is true when MeanDelta is zero.
	Identical bool
}

// VisualDiff compares two PNG images pixel-by-pixel and returns a DiffResult.
func VisualDiff(pathA, pathB string) (*DiffResult, error) {
	imgA, err := loadPNG(pathA)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", pathA, err)
	}
	imgB, err := loadPNG(pathB)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", pathB, err)
	}

	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()
	width := boundsA.Dx()
	height := boundsA.Dy()
	if boundsB.Dx() != width || boundsB.Dy() != height {
		return nil, fmt.Errorf("image dimensions differ: %dx%d vs %dx%d",
			width, height, boundsB.Dx(), boundsB.Dy())
	}

	totalPixels := width * height
	var totalDelta float64
	diffPixels := 0
	const threshold = 0.01

	for y := boundsA.Min.Y; y < boundsA.Max.Y; y++ {
		for x := boundsA.Min.X; x < boundsA.Max.X; x++ {
			rA, gA, bA, _ := imgA.At(x, y).RGBA()
			rB, gB, bB, _ := imgB.At(x, y).RGBA()

			dr := math.Abs(float64(rA)-float64(rB)) / 65535.0
			dg := math.Abs(float64(gA)-float64(gB)) / 65535.0
			db := math.Abs(float64(bA)-float64(bB)) / 65535.0
			pixelDelta := (dr + dg + db) / 3.0

			totalDelta += pixelDelta
			if pixelDelta > threshold {
				diffPixels++
			}
		}
	}

	meanDelta := totalDelta / float64(totalPixels)

	return &DiffResult{
		MeanDelta:   meanDelta,
		DiffPixels:  diffPixels,
		TotalPixels: totalPixels,
		Identical:   meanDelta == 0,
	}, nil
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
