package ui

import (
	"os"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

// RenderMarkdown renders markdown text with glamour styling.
// Returns raw markdown on failure for graceful degradation.
func RenderMarkdown(markdown string) string {
	// agent mode outputs plain text for machine parsing
	if IsAgentMode() {
		return markdown
	}

	// no styling when colors are disabled
	if !ShouldUseColor() {
		return markdown
	}

	wrapWidth := getTerminalWidth()

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
