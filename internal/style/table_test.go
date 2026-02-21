package style

import (
	"strings"
	"testing"
)

func TestNewTable(t *testing.T) {
	tbl := NewTable(
		Column{Name: "Name", Width: 10},
		Column{Name: "Value", Width: 20},
	)
	if tbl == nil {
		t.Fatal("NewTable() returned nil")
	}
	if len(tbl.columns) != 2 {
		t.Errorf("columns = %d, want 2", len(tbl.columns))
	}
	if !tbl.headerSep {
		t.Error("headerSep should default to true")
	}
	if tbl.indent != "  " {
		t.Errorf("indent = %q, want %q", tbl.indent, "  ")
	}
}

func TestTable_SetIndent(t *testing.T) {
	tbl := NewTable(Column{Name: "A", Width: 5})
	ret := tbl.SetIndent("    ")
	if ret != tbl {
		t.Error("SetIndent should return the table for chaining")
	}
	if tbl.indent != "    " {
		t.Errorf("indent = %q, want %q", tbl.indent, "    ")
	}
}

func TestTable_SetHeaderSeparator(t *testing.T) {
	tbl := NewTable(Column{Name: "A", Width: 5})
	ret := tbl.SetHeaderSeparator(false)
	if ret != tbl {
		t.Error("SetHeaderSeparator should return the table for chaining")
	}
	if tbl.headerSep {
		t.Error("headerSep should be false")
	}
}

func TestTable_AddRow(t *testing.T) {
	tbl := NewTable(
		Column{Name: "A", Width: 5},
		Column{Name: "B", Width: 5},
	)

	t.Run("exact columns", func(t *testing.T) {
		tbl.AddRow("x", "y")
		if len(tbl.rows) != 1 {
			t.Fatalf("rows = %d, want 1", len(tbl.rows))
		}
		if tbl.rows[0][0] != "x" || tbl.rows[0][1] != "y" {
			t.Errorf("row = %v, want [x y]", tbl.rows[0])
		}
	})

	t.Run("fewer columns padded", func(t *testing.T) {
		tbl.AddRow("only-one")
		last := tbl.rows[len(tbl.rows)-1]
		if len(last) != 2 {
			t.Fatalf("row len = %d, want 2 (padded)", len(last))
		}
		if last[1] != "" {
			t.Errorf("padded value = %q, want empty", last[1])
		}
	})

	t.Run("chaining", func(t *testing.T) {
		ret := tbl.AddRow("a", "b")
		if ret != tbl {
			t.Error("AddRow should return the table for chaining")
		}
	})
}

func TestTable_Render_Empty(t *testing.T) {
	tbl := NewTable()
	if result := tbl.Render(); result != "" {
		t.Errorf("Render() with no columns = %q, want empty", result)
	}
}

func TestTable_Render_Basic(t *testing.T) {
	tbl := NewTable(
		Column{Name: "ID", Width: 5},
		Column{Name: "Name", Width: 10},
	)
	tbl.SetHeaderSeparator(false)
	tbl.SetIndent("")
	tbl.AddRow("1", "Alice")
	tbl.AddRow("2", "Bob")

	result := tbl.Render()
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d: %v", len(lines), lines)
	}

	// Row data should be present
	if !strings.Contains(stripAnsi(lines[1]), "1") || !strings.Contains(stripAnsi(lines[1]), "Alice") {
		t.Errorf("row 1 missing data: %q", lines[1])
	}
	if !strings.Contains(stripAnsi(lines[2]), "2") || !strings.Contains(stripAnsi(lines[2]), "Bob") {
		t.Errorf("row 2 missing data: %q", lines[2])
	}
}

func TestTable_Render_WithSeparator(t *testing.T) {
	tbl := NewTable(
		Column{Name: "X", Width: 5},
	)
	tbl.SetIndent("")
	tbl.AddRow("val")

	result := tbl.Render()
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")

	// header + separator + 1 row = 3 lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + sep + row), got %d", len(lines))
	}

	// Separator line should contain dash-like characters
	sepPlain := stripAnsi(lines[1])
	if !strings.Contains(sepPlain, "─") && !strings.Contains(sepPlain, "-") {
		t.Errorf("separator line doesn't look like a separator: %q", sepPlain)
	}
}

func TestTable_Render_WithIndent(t *testing.T) {
	tbl := NewTable(Column{Name: "A", Width: 5})
	tbl.SetIndent(">>>")
	tbl.AddRow("x")

	result := tbl.Render()
	for _, line := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
		if !strings.HasPrefix(line, ">>>") {
			t.Errorf("line missing indent: %q", line)
		}
	}
}

func TestTable_Render_Truncation(t *testing.T) {
	tbl := NewTable(Column{Name: "N", Width: 8})
	tbl.SetHeaderSeparator(false)
	tbl.SetIndent("")
	tbl.AddRow("this-is-way-too-long-for-the-column")

	result := tbl.Render()
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")

	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines")
	}

	rowPlain := stripAnsi(lines[1])
	if !strings.HasSuffix(strings.TrimSpace(rowPlain), "...") {
		t.Errorf("truncated row should end with '...': %q", rowPlain)
	}
	if len(strings.TrimSpace(rowPlain)) > 8 {
		t.Errorf("truncated row too wide: %d chars", len(strings.TrimSpace(rowPlain)))
	}
}

func TestTable_Pad_AlignLeft(t *testing.T) {
	tbl := &Table{}
	result := tbl.pad("hi", "hi", 10, AlignLeft)
	if result != "hi        " {
		t.Errorf("pad left = %q, want %q", result, "hi        ")
	}
}

func TestTable_Pad_AlignRight(t *testing.T) {
	tbl := &Table{}
	result := tbl.pad("hi", "hi", 10, AlignRight)
	if result != "        hi" {
		t.Errorf("pad right = %q, want %q", result, "        hi")
	}
}

func TestTable_Pad_AlignCenter(t *testing.T) {
	tbl := &Table{}
	result := tbl.pad("hi", "hi", 10, AlignCenter)
	// 8 chars padding: 4 left, 4 right
	if result != "    hi    " {
		t.Errorf("pad center = %q, want %q", result, "    hi    ")
	}
}

func TestTable_Pad_ExactWidth(t *testing.T) {
	tbl := &Table{}
	result := tbl.pad("hello", "hello", 5, AlignLeft)
	if result != "hello" {
		t.Errorf("pad exact = %q, want %q", result, "hello")
	}
}

func TestTable_Pad_Overflow(t *testing.T) {
	tbl := &Table{}
	result := tbl.pad("toolong", "toolong", 3, AlignLeft)
	// When plain text >= width, return styled text as-is
	if result != "toolong" {
		t.Errorf("pad overflow = %q, want %q", result, "toolong")
	}
}

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no ansi", "hello", "hello"},
		{"bold", "\x1b[1mhello\x1b[0m", "hello"},
		{"color", "\x1b[31mred\x1b[0m", "red"},
		{"multiple", "\x1b[1m\x1b[31mbold red\x1b[0m", "bold red"},
		{"empty", "", ""},
		{"mixed", "before\x1b[32mgreen\x1b[0mafter", "beforegreenafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAlignmentConstants(t *testing.T) {
	// Verify the constants are distinct
	if AlignLeft == AlignRight || AlignLeft == AlignCenter || AlignRight == AlignCenter {
		t.Error("alignment constants should be distinct")
	}
}

func TestTable_Render_MultipleAlignments(t *testing.T) {
	tbl := NewTable(
		Column{Name: "L", Width: 10, Align: AlignLeft},
		Column{Name: "R", Width: 10, Align: AlignRight},
		Column{Name: "C", Width: 10, Align: AlignCenter},
	)
	tbl.SetHeaderSeparator(false)
	tbl.SetIndent("")
	tbl.AddRow("left", "right", "center")

	result := tbl.Render()
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines")
	}

	rowPlain := stripAnsi(lines[1])
	// Left-aligned: starts with "left"
	if !strings.HasPrefix(rowPlain, "left") {
		t.Errorf("left column not left-aligned: %q", rowPlain)
	}
}

func TestTable_Render_NoRows(t *testing.T) {
	tbl := NewTable(Column{Name: "Header", Width: 10})
	tbl.SetIndent("")

	result := tbl.Render()
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")

	// Should have header + separator only
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + sep), got %d", len(lines))
	}
}
