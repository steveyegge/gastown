package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var (
	docgenOutDir       string
	docgenOutputFormat string
	docgenFrontmatter  bool
)

var docgenCmd = &cobra.Command{
	Use:   "docgen",
	Short: "Generate Markdown documentation for gt CLI commands",
	Long: `docgen generates Markdown documentation for all gt CLI commands.
It uses the cobra/doc package to produce structured documentation.`,
	RunE: runDocGen,
}

func init() {
	docgenCmd.Flags().StringVarP(&docgenOutDir, "out", "o", "./docs/cli", "Output directory for generated docs")
	docgenCmd.Flags().StringVarP(&docgenOutputFormat, "format", "f", "markdown", "Output format (markdown, man, rest)")
	docgenCmd.Flags().BoolVarP(&docgenFrontmatter, "frontmatter", "", true, "Include frontmatter in markdown output")
	rootCmd.AddCommand(docgenCmd)
}

func runDocGen(cmd *cobra.Command, args []string) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(docgenOutDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Printf("Generating %s documentation to %s...\n", docgenOutputFormat, docgenOutDir)

	// Get the actual gt root command from the cmd package
	gtRoot := Root()

	// Disable the auto-generated tag for reproducible output
	gtRoot.DisableAutoGenTag = true

	switch docgenOutputFormat {
	case "markdown", "md":
		if docgenFrontmatter {
			// Generate with frontmatter
			prepender := func(filename string) string {
				name := strings.TrimSuffix(filename, ".md")
				name = filepath.Base(name)
				name = strings.ReplaceAll(name, "_", " ")
				title := strings.ToTitle(name)
				return fmt.Sprintf(`---
title: "%s"
---

`, title)
			}
			linkHandler := func(name string) string {
				base := strings.TrimSuffix(name, ".md")
				base = filepath.Base(base)
				return fmt.Sprintf("../cli/%s/", base)
			}
			if err := doc.GenMarkdownTreeCustom(gtRoot, docgenOutDir, prepender, linkHandler); err != nil {
				return fmt.Errorf("failed to generate markdown docs: %w", err)
			}
		} else {
			// Generate without frontmatter
			if err := doc.GenMarkdownTree(gtRoot, docgenOutDir); err != nil {
				return fmt.Errorf("failed to generate markdown docs: %w", err)
			}
		}
	case "man":
		// For man pages, generate one file per command in the output directory
		header := &doc.GenManHeader{
			Title:   "GT",
			Section: "1",
		}
		if err := doc.GenManTree(gtRoot, header, docgenOutDir); err != nil {
			return fmt.Errorf("failed to generate man pages: %w", err)
		}
	case "rest", "rst":
		if err := doc.GenReSTTree(gtRoot, docgenOutDir); err != nil {
			return fmt.Errorf("failed to generate reStructuredText docs: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s (supported: markdown, man, rest)", docgenOutputFormat)
	}

	// List generated files
	files, err := filepath.Glob(filepath.Join(docgenOutDir, "*"))
	if err != nil {
		return fmt.Errorf("failed to list generated files: %w", err)
	}

	fmt.Printf("Generated %d file(s):\n", len(files))
	for _, file := range files {
		fmt.Printf("  - %s\n", filepath.Base(file))
	}

	return nil
}
