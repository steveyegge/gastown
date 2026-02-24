// docgen generates Markdown documentation for the gt CLI commands.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	gtcmd "github.com/steveyegge/gastown/internal/cmd"
)

var (
	outDir       string
	outputFormat string
	frontmatter  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "docgen",
		Short: "Generate Markdown documentation for gt CLI commands",
		Long: `docgen generates Markdown documentation for all gt CLI commands.
It uses the cobra/doc package to produce structured documentation.`,
		RunE: runDocGen,
	}

	rootCmd.Flags().StringVarP(&outDir, "out", "o", "./docs/cli", "Output directory for generated docs")
	rootCmd.Flags().StringVarP(&outputFormat, "format", "f", "markdown", "Output format (markdown, man, rest)")
	rootCmd.Flags().BoolVarP(&frontmatter, "frontmatter", "", true, "Include frontmatter in markdown output")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDocGen(cmd *cobra.Command, args []string) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Printf("Generating %s documentation to %s...\n", outputFormat, outDir)

	// Get the actual gt root command from the cmd package
	gtRoot := gtcmd.GetRootCmd()

	// Disable the auto-generated tag for reproducible output
	gtRoot.DisableAutoGenTag = true

	switch outputFormat {
	case "markdown", "md":
		if frontmatter {
			// Generate with frontmatter
			prepender := func(filename string) string {
				name := strings.TrimSuffix(filename, ".md")
				name = strings.ReplaceAll(name, "_", " ")
				title := strings.ToTitle(name)
				return fmt.Sprintf(`---
title: "%s"
---

`, title)
			}
			linkHandler := func(name string) string {
				base := strings.TrimSuffix(name, ".md")
				return fmt.Sprintf("../cli/%s/", base)
			}
			if err := doc.GenMarkdownTreeCustom(gtRoot, outDir, prepender, linkHandler); err != nil {
				return fmt.Errorf("failed to generate markdown docs: %w", err)
			}
		} else {
			// Generate without frontmatter
			if err := doc.GenMarkdownTree(gtRoot, outDir); err != nil {
				return fmt.Errorf("failed to generate markdown docs: %w", err)
			}
		}
	case "man":
		// For man pages, generate one file per command in the output directory
		header := &doc.GenManHeader{
			Title:   "GT",
			Section: "1",
		}
		if err := doc.GenManTree(gtRoot, header, outDir); err != nil {
			return fmt.Errorf("failed to generate man pages: %w", err)
		}
	case "rest", "rst":
		if err := doc.GenReSTTree(gtRoot, outDir); err != nil {
			return fmt.Errorf("failed to generate reStructuredText docs: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s (supported: markdown, man, rest)", outputFormat)
	}

	// List generated files
	files, err := filepath.Glob(filepath.Join(outDir, "*"))
	if err != nil {
		return fmt.Errorf("failed to list generated files: %w", err)
	}

	fmt.Printf("Generated %d file(s):\n", len(files))
	for _, file := range files {
		fmt.Printf("  - %s\n", filepath.Base(file))
	}

	return nil
}
