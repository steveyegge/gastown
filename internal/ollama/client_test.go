package ollama

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"bytes"
	"testing"
	"time"
)

// testCtx returns a context with a generous timeout for model inference.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// makeTestPNG creates a small solid-colour PNG in memory for vision tests.
func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	// Fill with red so vision models have something to describe.
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

func TestPing(t *testing.T) {
	c := NewClient()
	if err := c.Ping(testCtx(t)); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	t.Log("Ollama is reachable at", c.BaseURL)
}

func TestListModels(t *testing.T) {
	c := NewClient()
	models, err := c.ListModels(testCtx(t))
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}

	required := map[string]bool{
		"gemma3:4b":          false,
		"nemotron-3-nano:4b": false,
		"moondream:latest":   false,
	}
	for _, m := range models {
		if _, ok := required[m.Name]; ok {
			required[m.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("required model %q not found", name)
		}
	}
	t.Logf("found %d models, all required models present", len(models))
}

func TestGenerateText(t *testing.T) {
	c := NewClient()
	resp, err := c.Generate(testCtx(t), "nemotron-3-nano:4b", "Say hello in exactly one sentence.")
	if err != nil {
		t.Fatalf("Generate (text) failed: %v", err)
	}
	if resp.Response == "" {
		t.Fatal("got empty response from nemotron-3-nano:4b")
	}
	if !resp.Done {
		t.Fatal("response not marked done")
	}
	t.Logf("nemotron-3-nano:4b response: %s", resp.Response)
}

func TestGenerateVisionGemma(t *testing.T) {
	c := NewClient()
	img := makeTestPNG(t)
	resp, err := c.Generate(testCtx(t), "gemma3:4b", "Describe this image briefly.", img)
	if err != nil {
		t.Fatalf("Generate (vision/gemma3) failed: %v", err)
	}
	if resp.Response == "" {
		t.Fatal("got empty response from gemma3:4b vision")
	}
	t.Logf("gemma3:4b vision response: %s", resp.Response)
}

func TestGenerateVisionMoondream(t *testing.T) {
	c := NewClient()
	img := makeTestPNG(t)
	resp, err := c.Generate(testCtx(t), "moondream:latest", "What do you see in this image?", img)
	if err != nil {
		t.Fatalf("Generate (vision/moondream) failed: %v", err)
	}
	if resp.Response == "" {
		t.Fatal("got empty response from moondream")
	}
	t.Logf("moondream response: %s", resp.Response)
}
