// Package formula provides TOML formula parsing and formatting.
package formula

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// FormatTOML converts single-line strings with \n escapes to multi-line strings.
// Preserves comments, blank lines, and key ordering.
// Returns the formatted content, whether any changes were made, and any error.
func FormatTOML(content []byte) ([]byte, bool, error) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))
	changed := false
	inMultiline := false

	for scanner.Scan() {
		line := scanner.Text()

		// If we're inside a multi-line string, just pass through
		if inMultiline {
			out.WriteString(line)
			out.WriteByte('\n')
			// Check if this line ends the multi-line string
			if strings.HasSuffix(strings.TrimSpace(line), `"""`) {
				inMultiline = false
			}
			continue
		}

		// Check if this line starts a multi-line string
		if strings.Contains(line, `= """`) {
			out.WriteString(line)
			out.WriteByte('\n')
			// If it doesn't end on the same line, we're in a multi-line
			trimmed := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmed, `"""`) || strings.Count(trimmed, `"""`) == 1 {
				inMultiline = true
			}
			continue
		}

		// Try to convert this line to multi-line format
		if converted, ok := tryConvertToMultiline(line); ok {
			out.WriteString(converted)
			out.WriteByte('\n')
			changed = true
		} else {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, false, err
	}

	return out.Bytes(), changed, nil
}

// stringAssignmentRegex matches TOML string assignments like: key = "value"
// Captures: 1=key, 2=value (without quotes)
var stringAssignmentRegex = regexp.MustCompile(`^(\s*)(\w+)\s*=\s*"((?:[^"\\]|\\.)*)"\s*$`)

// tryConvertToMultiline checks if a line is a string assignment with \n escapes
// and converts it to multi-line format if so.
func tryConvertToMultiline(line string) (string, bool) {
	matches := stringAssignmentRegex.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}

	indent := matches[1]
	key := matches[2]
	value := matches[3]

	// Only convert if the value contains \n (literal backslash-n)
	if !strings.Contains(value, `\n`) {
		return "", false
	}

	// Unescape the string
	unescaped := unescapeString(value)

	// Escape any triple quotes in the content
	escaped := escapeMultilineContent(unescaped)

	// Format as multi-line string
	// Put opening """ on same line as key, content on next lines
	var result strings.Builder
	result.WriteString(indent)
	result.WriteString(key)
	result.WriteString(` = """`)

	// TOML trims a newline immediately after """, so:
	// - Always put content on a new line after """
	// - If content starts with a newline, escape it as \n to preserve it
	result.WriteByte('\n')
	if strings.HasPrefix(escaped, "\n") {
		// Escape the leading newline so it's not trimmed
		result.WriteString(`\n`)
		result.WriteString(escaped[1:])
	} else {
		result.WriteString(escaped)
	}

	result.WriteString(`"""`)

	return result.String(), true
}

// unescapeString converts TOML escape sequences to actual characters.
// Only expands \n and \t to actual newlines/tabs (for multi-line formatting).
// All other escapes are preserved as-is since they need to remain valid TOML.
func unescapeString(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] != '\\' || i+1 >= len(s) {
			result.WriteByte(s[i])
			i++
			continue
		}

		// Handle escape sequence
		next := s[i+1]
		switch next {
		case 'n':
			// Convert \n to actual newline for multi-line string
			result.WriteByte('\n')
			i += 2
		case 't':
			// Convert \t to actual tab
			result.WriteByte('\t')
			i += 2
		default:
			// Keep all other escapes as-is (\\, \", \r, \b, \f, \uXXXX, \UXXXXXXXX, \xNN)
			// This preserves the original escape sequences
			result.WriteByte(s[i])
			i++
		}
	}

	return result.String()
}

// escapeMultilineContent escapes content for use in a TOML multi-line basic string.
// Since we preserve escape sequences from unescapeString (only \n and \t expand),
// we only need to handle sequences of 3+ double quotes (which would confuse the parser).
func escapeMultilineContent(s string) string {
	var result strings.Builder
	result.Grow(len(s) + 10)

	quoteCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			quoteCount++
			// If we've accumulated 2 quotes and the next is also a quote (making 3),
			// we need to escape this quote to prevent """ being seen as string terminator
			if quoteCount >= 2 && i+1 < len(s) && s[i+1] == '"' {
				result.WriteString(`\"`)
				quoteCount = 0
			} else {
				result.WriteByte(c)
			}
		} else {
			quoteCount = 0
			result.WriteByte(c)
		}
	}

	return result.String()
}
