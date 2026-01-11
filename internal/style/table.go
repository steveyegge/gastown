// Package style provides consistent terminal styling using Lipgloss.
package style

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Column defines a table column with name and width.
type Column struct {
	Name  string
	Width int
	Align Alignment
	Style lipgloss.Style
}

// Alignment specifies column text alignment.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignRight
	AlignCenter
)

// Table provides styled table rendering.
type Table struct {
	columns    []Column
	rows       [][]string
	headerSep  bool
	indent     string
	headerStyle lipgloss.Style
}

// NewTable creates a new table with the given columns.
func NewTable(columns ...Column) *Table {
	return &Table{
		columns:    columns,
		headerSep:  true,
		indent:     "  ",
		headerStyle: Bold,
	}
}

// SetIndent sets the left indent for the table.
func (t *Table) SetIndent(indent string) *Table {
	t.indent = indent
	return t
}

// SetHeaderSeparator enables/disables the header separator line.
func (t *Table) SetHeaderSeparator(enabled bool) *Table {
	t.headerSep = enabled
	return t
}

// AddRow adds a row of values to the table.
func (t *Table) AddRow(values ...string) *Table {
	// Pad with empty strings if needed
	for len(values) < len(t.columns) {
		values = append(values, "")
	}
	t.rows = append(t.rows, values)
	return t
}

// Render returns the formatted table string.
func (t *Table) Render() string {
	if len(t.columns) == 0 {
		return ""
	}

	var sb strings.Builder

	// Render header
	sb.WriteString(t.indent)
	for i, col := range t.columns {
		text := t.headerStyle.Render(col.Name)
		sb.WriteString(t.pad(text, col.Name, col.Width, col.Align))
		if i < len(t.columns)-1 {
			sb.WriteString(" ")
		}
	}
	sb.WriteString("\n")

	// Render separator
	if t.headerSep {
		sb.WriteString(t.indent)
		totalWidth := 0
		for i, col := range t.columns {
			totalWidth += col.Width
			if i < len(t.columns)-1 {
				totalWidth++ // space between columns
			}
		}
		sb.WriteString(Dim.Render(strings.Repeat("─", totalWidth)))
		sb.WriteString("\n")
	}

	// Render rows
	for _, row := range t.rows {
		sb.WriteString(t.indent)
		for i, col := range t.columns {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			// Truncate if too long
			plainVal := stripAnsi(val)
			if len(plainVal) > col.Width {
				val = plainVal[:col.Width-3] + "..."
			}
			// Apply column style if set
			if col.Style.Value() != "" {
				val = col.Style.Render(val)
			}
			sb.WriteString(t.pad(val, plainVal, col.Width, col.Align))
			if i < len(t.columns)-1 {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// pad pads text to width, accounting for ANSI escape sequences.
// styledText is the text with ANSI codes, plainText is without.
func (t *Table) pad(styledText, plainText string, width int, align Alignment) string {
	plainLen := len(plainText)
	if plainLen >= width {
		return styledText
	}

	padding := width - plainLen

	switch align {
	case AlignRight:
		return strings.Repeat(" ", padding) + styledText
	case AlignCenter:
		left := padding / 2
		right := padding - left
		return strings.Repeat(" ", left) + styledText + strings.Repeat(" ", right)
	default: // AlignLeft
		return styledText + strings.Repeat(" ", padding)
	}
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

// PhaseTable renders the molecule phase transition table.
func PhaseTable() string {
	return `
  Phase Flow:
    discovery ──┬──→ structural ──→ tactical ──→ synthesis
                │    (sequential)   (parallel)   (single)
                └─── (parallel)

  ┌─────────────┬─────────────┬─────────────┬─────────────────────┐
  │ Phase       │ Parallelism │ Blocks      │ Purpose             │
  ├─────────────┼─────────────┼─────────────┼─────────────────────┤
  │ discovery   │ full        │ (nothing)   │ Inventory, gather   │
  │ structural  │ sequential  │ discovery   │ Big-picture review  │
  │ tactical    │ parallel    │ structural  │ Detailed work       │
  │ synthesis   │ single      │ tactical    │ Aggregate results   │
  └─────────────┴─────────────┴─────────────┴─────────────────────┘`
}

// MoleculeLifecycleASCII renders the molecule lifecycle diagram.
func MoleculeLifecycleASCII() string {
	return `
  Proto (template)
       │
       ▼ bond
  ┌─────────────────┐
  │ Mol (durable)   │
  │ Wisp (ephemeral)│
  └────────┬────────┘
           │
    ┌──────┴──────┐
    ▼             ▼
  burn         squash
  (no record)  (→ digest)`
}

// DAGProgress renders a DAG progress visualization.
// steps is a map of step name to status (done, in_progress, ready, blocked).
func DAGProgress(steps map[string]string, phases []string) string {
	var sb strings.Builder

	icons := map[string]string{
		"done":        "✓",
		"in_progress": "⧖",
		"ready":       "○",
		"blocked":     "◌",
	}

	colors := map[string]lipgloss.Style{
		"done":        Success,
		"in_progress": Warning,
		"ready":       Info,
		"blocked":     Dim,
	}

	for _, phase := range phases {
		sb.WriteString(fmt.Sprintf("  %s\n", Bold.Render(phase)))
		for name, status := range steps {
			if strings.HasPrefix(name, phase+"-") || strings.HasPrefix(name, phase+"/") {
				icon := icons[status]
				style := colors[status]
				stepName := strings.TrimPrefix(strings.TrimPrefix(name, phase+"-"), phase+"/")
				sb.WriteString(fmt.Sprintf("    %s %s\n", style.Render(icon), stepName))
			}
		}
	}

	return sb.String()
}

// SuggestionBox renders a "did you mean" suggestion box.
func SuggestionBox(message string, suggestions []string, hint string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n%s %s\n", ErrorPrefix, message))

	if len(suggestions) > 0 {
		sb.WriteString("\n  Did you mean?\n")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("    • %s\n", s))
		}
	}

	if hint != "" {
		sb.WriteString(fmt.Sprintf("\n  %s\n", Dim.Render(hint)))
	}

	return sb.String()
}

// ProgressBar renders a simple progress bar.
func ProgressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := (percent * width) / 100
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("[%s] %d%%", bar, percent)
}
