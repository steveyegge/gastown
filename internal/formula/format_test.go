package formula

import (
	"fmt"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"pgregory.net/rapid"
)

func TestFormatTOML_SimpleConversion(t *testing.T) {
	input := `description = "line1\n\nline2"`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes but got none")
	}

	expected := `description = """
line1

line2"""`
	if strings.TrimSpace(string(output)) != expected {
		t.Errorf("unexpected output:\ngot:\n%s\n\nwant:\n%s", output, expected)
	}
}

func TestFormatTOML_NoConversionNeeded(t *testing.T) {
	input := `id = "simple-value"`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected no changes")
	}
	if strings.TrimSpace(string(output)) != input {
		t.Errorf("output should match input:\ngot: %s\nwant: %s", output, input)
	}
}

func TestFormatTOML_AlreadyMultiline(t *testing.T) {
	input := `description = """
already
multiline"""`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected no changes for already multiline string")
	}
	if strings.TrimSpace(string(output)) != strings.TrimSpace(input) {
		t.Errorf("output should match input:\ngot:\n%s\nwant:\n%s", output, input)
	}
}

func TestFormatTOML_PreservesComments(t *testing.T) {
	input := `# This is a comment
description = "line1\nline2"
# Another comment`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes")
	}

	if !strings.Contains(string(output), "# This is a comment") {
		t.Error("first comment should be preserved")
	}
	if !strings.Contains(string(output), "# Another comment") {
		t.Error("second comment should be preserved")
	}
}

func TestFormatTOML_PreservesBlankLines(t *testing.T) {
	input := `key1 = "value1"

key2 = "line1\nline2"

key3 = "value3"`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes")
	}

	// Count blank lines in output
	lines := strings.Split(string(output), "\n")
	blankCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
		}
	}
	// Should have at least 2 blank lines (between keys)
	if blankCount < 2 {
		t.Errorf("blank lines not preserved, got %d blank lines", blankCount)
	}
}

func TestFormatTOML_HandleTripleQuotes(t *testing.T) {
	// Input contains """ which needs escaping
	input := `description = "code:\n\"\"\"\nprint('hello')\n\"\"\""`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes")
	}

	// Verify it still parses
	var result struct{ Description string }
	if _, err := toml.Decode(string(output), &result); err != nil {
		t.Fatalf("formatted output should be valid TOML: %v\noutput:\n%s", err, output)
	}
}

func TestFormatTOML_PreservesTableHeaders(t *testing.T) {
	input := `[section]
description = "line1\nline2"

[[steps]]
id = "step1"
description = "step\ndescription"`

	output, changed, err := FormatTOML([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes")
	}

	if !strings.Contains(string(output), "[section]") {
		t.Error("section header should be preserved")
	}
	if !strings.Contains(string(output), "[[steps]]") {
		t.Error("array of tables header should be preserved")
	}
}

func TestUnescapeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello\nworld`, "hello\nworld"},       // \n expands
		{`tab\there`, "tab\there"},             // \t expands
		{`quote\"here`, `quote\"here`},         // \" preserved
		{`backslash\\here`, `backslash\\here`}, // \\ preserved
		{`\r\n`, `\r` + "\n"},                  // \r preserved, \n expands
		{`no escapes`, `no escapes`},
		{`mixed\nescapes\there`, "mixed\nescapes\there"},
		{`\x00`, `\x00`},     // hex escape preserved
		{`\b\f`, `\b\f`},     // control escapes preserved
		{`\uXXXX`, `\uXXXX`}, // unicode preserved
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := unescapeString(tc.input)
			if result != tc.expected {
				t.Errorf("unescapeString(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestEscapeMultilineContent(t *testing.T) {
	// Test that escaping produces valid TOML via round-trip
	tests := []string{
		`no quotes`,
		`one " quote`,
		`two "" quotes`,
		`three """ quotes`,
		`four """" quotes`,
		`five """"" quotes`,
		`"""`,
		`end with """`,
		`""" start`,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			escaped := escapeMultilineContent(input)
			// Build a multi-line TOML string
			tomlStr := fmt.Sprintf("value = \"\"\"\n%s\"\"\"", escaped)

			// Parse it
			var result struct{ Value string }
			if _, err := toml.Decode(tomlStr, &result); err != nil {
				t.Fatalf("escaped content not valid TOML: %v\nescaped: %q\ntoml:\n%s", err, escaped, tomlStr)
			}

			// Verify round-trip
			if result.Value != input {
				t.Errorf("round-trip mismatch:\ninput: %q\nresult: %q\nescaped: %q", input, result.Value, escaped)
			}
		})
	}
}

// Property-based tests using rapid

func TestFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary strings (may contain special chars)
		value := rapid.String().Draw(t, "value")

		// Build a valid TOML line using Go's %q which produces valid TOML strings
		input := fmt.Sprintf("description = %q", value)

		// First verify the input is valid TOML
		var orig struct{ Description string }
		if _, err := toml.Decode(input, &orig); err != nil {
			// If Go's %q produced something TOML can't parse, skip this case
			t.Skip("input not valid TOML")
		}

		// Format it
		formatted, _, err := FormatTOML([]byte(input))
		if err != nil {
			t.Fatalf("FormatTOML error: %v", err)
		}

		// Parse the formatted output
		var result struct{ Description string }
		if _, err := toml.Decode(string(formatted), &result); err != nil {
			t.Fatalf("formatted output not valid TOML: %v\ninput: %s\nformatted:\n%s", err, input, formatted)
		}

		// Values must be identical
		if orig.Description != result.Description {
			t.Fatalf("round-trip mismatch:\noriginal: %q\nresult: %q\ninput: %s\nformatted:\n%s",
				orig.Description, result.Description, input, formatted)
		}
	})
}

func TestFormatIdempotency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary strings
		value := rapid.String().Draw(t, "value")

		// Build a valid TOML line
		input := fmt.Sprintf("description = %q", value)

		// First verify the input is valid TOML
		var check struct{ Description string }
		if _, err := toml.Decode(input, &check); err != nil {
			t.Skip("input not valid TOML")
		}

		// Format once
		formatted1, _, err := FormatTOML([]byte(input))
		if err != nil {
			t.Fatalf("first format error: %v", err)
		}

		// Format twice
		formatted2, changed, err := FormatTOML(formatted1)
		if err != nil {
			t.Fatalf("second format error: %v", err)
		}

		// Second format should not change anything
		if changed {
			t.Fatalf("format is not idempotent:\nfirst:\n%s\nsecond:\n%s", formatted1, formatted2)
		}

		// Content should be identical
		if string(formatted1) != string(formatted2) {
			t.Fatalf("format output differs on second pass:\nfirst:\n%s\nsecond:\n%s", formatted1, formatted2)
		}
	})
}

func TestFormatPreservesNonStringLines(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate various non-string TOML lines
		lineType := rapid.IntRange(0, 4).Draw(t, "lineType")

		var line string
		switch lineType {
		case 0: // Comment
			text := rapid.StringMatching(`[a-zA-Z0-9 ]+`).Draw(t, "comment")
			line = "# " + text
		case 1: // Section header
			name := rapid.StringMatching(`[a-z][a-z0-9_]*`).Draw(t, "section")
			line = "[" + name + "]"
		case 2: // Array of tables header
			name := rapid.StringMatching(`[a-z][a-z0-9_]*`).Draw(t, "table")
			line = "[[" + name + "]]"
		case 3: // Integer assignment
			key := rapid.StringMatching(`[a-z][a-z0-9_]*`).Draw(t, "key")
			val := rapid.IntRange(0, 1000).Draw(t, "value")
			line = fmt.Sprintf("%s = %d", key, val)
		case 4: // Boolean assignment
			key := rapid.StringMatching(`[a-z][a-z0-9_]*`).Draw(t, "key")
			val := rapid.Bool().Draw(t, "value")
			line = fmt.Sprintf("%s = %v", key, val)
		}

		formatted, changed, err := FormatTOML([]byte(line))
		if err != nil {
			t.Fatalf("format error: %v", err)
		}

		// Non-string lines should not be changed
		if changed {
			t.Fatalf("non-string line was changed:\ninput: %s\noutput: %s", line, formatted)
		}

		// Content should be preserved
		if strings.TrimSpace(string(formatted)) != strings.TrimSpace(line) {
			t.Fatalf("line was modified:\ninput: %s\noutput: %s", line, formatted)
		}
	})
}

// Test with specific edge case strings
func TestFormatEdgeCases(t *testing.T) {
	cases := []string{
		"",                      // empty
		"simple",                // no escapes
		"with\nnewline",         // newline
		"with\ttab",             // tab
		`with "quotes"`,         // quotes
		`with \\ backslash`,     // backslash
		"line1\n\nline2",        // multiple newlines
		"ends with newline\n",   // trailing newline
		"\nstarts with newline", // leading newline
		`"""triple"""`,          // triple quotes
		"unicode: \u0048\u0065\u006c\u006c\u006f", // unicode
		"mixed\n\"\\\t\r",                         // multiple escapes
	}

	for _, value := range cases {
		t.Run(fmt.Sprintf("value=%q", value), func(t *testing.T) {
			input := fmt.Sprintf("description = %q", value)

			// Verify input is valid TOML
			var orig struct{ Description string }
			if _, err := toml.Decode(input, &orig); err != nil {
				t.Skipf("input not valid TOML: %v", err)
			}

			// Format
			formatted, _, err := FormatTOML([]byte(input))
			if err != nil {
				t.Fatalf("format error: %v", err)
			}

			// Parse formatted
			var result struct{ Description string }
			if _, err := toml.Decode(string(formatted), &result); err != nil {
				t.Fatalf("formatted output not valid TOML: %v\ninput: %s\nformatted:\n%s", err, input, formatted)
			}

			// Compare
			if orig.Description != result.Description {
				t.Fatalf("round-trip mismatch:\noriginal: %q\nresult: %q", orig.Description, result.Description)
			}
		})
	}
}
