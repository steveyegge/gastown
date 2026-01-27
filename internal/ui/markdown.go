package ui

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

// WrapText wraps text at word boundaries to fit within maxWidth.
// Preserves existing line breaks.
func WrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(wrapLine(line, maxWidth))
	}

	return result.String()
}

// wrapLine wraps a single line at word boundaries.
func wrapLine(line string, maxWidth int) string {
	if utf8.RuneCountInString(line) <= maxWidth {
		return line
	}

	var result strings.Builder
	words := strings.Fields(line)
	currentLen := 0

	for _, word := range words {
		wordLen := utf8.RuneCountInString(word)

		// If this is first word on line, add it even if too long
		if currentLen == 0 {
			result.WriteString(word)
			currentLen = wordLen
			continue
		}

		// Check if word fits on current line (with space)
		if currentLen+1+wordLen <= maxWidth {
			result.WriteString(" ")
			result.WriteString(word)
			currentLen += 1 + wordLen
		} else {
			// Start new line
			result.WriteString("\n")
			result.WriteString(word)
			currentLen = wordLen
		}
	}

	return result.String()
}

// RenderMarkdown renders markdown text with glamour styling.
// Returns raw markdown on failure for graceful degradation.
// Always wraps text at terminal width, even in agent mode.
func RenderMarkdown(markdown string) string {
	wrapWidth := getTerminalWidth()

	// agent mode: wrap text but skip glamour styling
	if IsAgentMode() {
		return WrapText(markdown, wrapWidth)
	}

	// no styling when colors are disabled, but still wrap
	if !ShouldUseColor() {
		return WrapText(markdown, wrapWidth)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return markdown
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}

	return rendered
}

// getTerminalWidth returns the terminal width for word wrapping.
// Caps at 100 chars for readability (research suggests 50-75 optimal, 80-100 comfortable).
// Falls back to 80 if detection fails.
func getTerminalWidth() int {
	const (
		defaultWidth = 80
		maxWidth     = 100
	)

	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return defaultWidth
	}

	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		return defaultWidth
	}

	if width > maxWidth {
		return maxWidth
	}

	return width
}
