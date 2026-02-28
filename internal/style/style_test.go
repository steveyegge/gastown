package style

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestStyleVariables(t *testing.T) {
	// Test that all style variables render non-empty output
	tests := []struct {
		name   string
		render func(...string) string
	}{
		{"Success", Success.Render},
		{"Warning", Warning.Render},
		{"Error", Error.Render},
		{"Info", Info.Render},
		{"Dim", Dim.Render},
		{"Bold", Bold.Render},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.render == nil {
				t.Errorf("Style variable %s should not be nil", tt.name)
			}
			// Test that Render works
			result := tt.render("test")
			if result == "" {
				t.Errorf("Style %s.Render() should not return empty string", tt.name)
			}
		})
	}
}

func TestPrefixVariables(t *testing.T) {
	// Test that all prefix variables are non-empty
	tests := []struct {
		name   string
		prefix string
	}{
		{"SuccessPrefix", SuccessPrefix},
		{"WarningPrefix", WarningPrefix},
		{"ErrorPrefix", ErrorPrefix},
		{"ArrowPrefix", ArrowPrefix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prefix == "" {
				t.Errorf("Prefix variable %s should not be empty", tt.name)
			}
		})
	}
}

func TestPrintWarning(t *testing.T) {
	// Capture stderr (PrintWarning writes to stderr, not stdout)
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	PrintWarning("test warning: %s", "value")

	w.Close()
	os.Stderr = oldStderr

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output == "" {
		t.Error("PrintWarning() should produce output on stderr")
	}

	// Check that warning message is present
	if !bytes.Contains(buf.Bytes(), []byte("test warning: value")) {
		t.Error("PrintWarning() output should contain the warning message")
	}
}

func TestPrintWarning_NoFormatArgs(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	PrintWarning("simple warning")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output == "" {
		t.Error("PrintWarning() should produce output on stderr")
	}

	if !bytes.Contains(buf.Bytes(), []byte("simple warning")) {
		t.Error("PrintWarning() output should contain the message")
	}
}

func TestPrintWarning_DoesNotWriteStdout(t *testing.T) {
	// Verify PrintWarning does NOT write to stdout (prevents JSON contamination)
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW

	PrintWarning("should go to stderr only")

	stdoutW.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf, stderrBuf bytes.Buffer
	io.Copy(&stdoutBuf, stdoutR)
	io.Copy(&stderrBuf, stderrR)

	if stdoutBuf.Len() > 0 {
		t.Errorf("PrintWarning() must not write to stdout (got %q), it contaminates JSON output", stdoutBuf.String())
	}
	if stderrBuf.Len() == 0 {
		t.Error("PrintWarning() should write to stderr")
	}
}

func TestStyles_RenderConsistently(t *testing.T) {
	// Test that styles consistently render non-empty output
	testText := "test message"

	styles := map[string]func(...string) string{
		"Success": Success.Render,
		"Warning": Warning.Render,
		"Error":   Error.Render,
		"Info":    Info.Render,
		"Dim":     Dim.Render,
		"Bold":    Bold.Render,
	}

	for name, renderFunc := range styles {
		t.Run(name, func(t *testing.T) {
			result := renderFunc(testText)
			if result == "" {
				t.Errorf("Style %s.Render() should not return empty string", name)
			}
			// Result should be different from input (has styling codes)
			// except possibly for some edge cases
		})
	}
}

func TestMultiplePrintWarning(t *testing.T) {
	// Test that multiple warnings can be printed to stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	for i := 0; i < 3; i++ {
		PrintWarning("warning %d", i)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	_ = buf.String() // ensure buffer is read

	// Should have 3 lines
	lineCount := 0
	for _, b := range buf.Bytes() {
		if b == '\n' {
			lineCount++
		}
	}

	if lineCount != 3 {
		t.Errorf("Expected 3 lines of output, got %d", lineCount)
	}
}

func ExamplePrintWarning() {
	// PrintWarning writes to stderr (not stdout) to avoid contaminating
	// structured output like JSON. This example demonstrates usage;
	// output appears on stderr, not in the example output below.
	PrintWarning("This is a warning message")
	PrintWarning("Warning with value: %d", 42)
}
