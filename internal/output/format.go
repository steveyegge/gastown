// Package output provides output formatting for gt commands.
// Supports JSON (default) and TOON (Token-Optimized Object Notation)
// for reduced LLM token consumption when agents read command output.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	toon "github.com/toon-format/toon-go"
)

// Format represents an output format.
type Format string

const (
	// FormatJSON is standard JSON output.
	FormatJSON Format = "json"
	// FormatTOON is Token-Optimized Object Notation.
	FormatTOON Format = "toon"
)

// ResolveFormat determines the output format from flag value and environment.
// Priority: explicit flag > GT_OUTPUT_FORMAT env var > default (json).
func ResolveFormat(flagValue string) Format {
	if flagValue != "" {
		switch strings.ToLower(flagValue) {
		case "toon":
			return FormatTOON
		case "json":
			return FormatJSON
		}
	}

	if env := os.Getenv("GT_OUTPUT_FORMAT"); env != "" {
		switch strings.ToLower(env) {
		case "toon":
			return FormatTOON
		case "json":
			return FormatJSON
		}
	}

	return FormatJSON
}

// IsTOON returns true if the resolved format is TOON.
func IsTOON() bool {
	return ResolveFormat("") == FormatTOON
}

// PrintJSON writes the value as pretty-printed JSON to stdout.
func PrintJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", data)
	return err
}

// PrintTOON writes the value as TOON to stdout.
func PrintTOON(v any) error {
	data, err := toon.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", string(data))
	return err
}

// PrintFormatted writes the value in the specified format.
// Falls back to JSON if TOON encoding fails.
func PrintFormatted(v any, format Format) error {
	if format == FormatTOON {
		if err := PrintTOON(v); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ toon encoding failed, falling back to JSON: %v\n", err)
			return PrintJSON(v)
		}
		return nil
	}
	return PrintJSON(v)
}

// Print writes the value to stdout in the auto-resolved format.
// Checks GT_OUTPUT_FORMAT env var (set by --format flag or agent config).
// Falls back to JSON if TOON encoding fails.
func Print(v any) error {
	return PrintFormatted(v, ResolveFormat(""))
}

// printJSONBytes pretty-prints already-marshaled JSON bytes.
func printJSONBytes(data []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		_, err = os.Stdout.Write(data)
		return err
	}
	buf.WriteByte('\n')
	_, err := io.Copy(os.Stdout, &buf)
	return err
}
