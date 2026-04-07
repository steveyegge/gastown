package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Schema output types

// SchemaIndex is the top-level catalog returned by `gt schema`.
type SchemaIndex struct {
	Schema   string             `json:"$schema"`
	Name     string             `json:"name"`
	Version  string             `json:"version"`
	Commands []SchemaIndexEntry `json:"commands"`
}

// SchemaIndexEntry is a single command in the index.
type SchemaIndexEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Group       string   `json:"group,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Subcommands []string `json:"subcommands,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
}

const jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// buildSchemaIndex walks the Cobra command tree and returns an index of all commands.
func buildSchemaIndex(root *cobra.Command) SchemaIndex {
	index := SchemaIndex{
		Schema:   jsonSchemaDraft,
		Name:     root.Name(),
		Version:  Version,
		Commands: []SchemaIndexEntry{},
	}

	schemaWalkCommands(root, root.Name(), &index.Commands)
	return index
}

// schemaWalkCommands recursively collects command entries.
func schemaWalkCommands(cmd *cobra.Command, prefix string, entries *[]SchemaIndexEntry) {
	for _, c := range cmd.Commands() {
		fullName := prefix + " " + c.Name()

		var subs []string
		for _, sub := range c.Commands() {
			if !sub.Hidden {
				subs = append(subs, sub.Name())
			}
		}

		entry := SchemaIndexEntry{
			Name:        fullName,
			Description: c.Short,
			Group:       c.GroupID,
			Hidden:      c.Hidden,
		}
		if len(c.Aliases) > 0 {
			entry.Aliases = c.Aliases
		}
		if len(subs) > 0 {
			entry.Subcommands = subs
		}

		*entries = append(*entries, entry)

		// Recurse into subcommands
		if c.HasSubCommands() {
			schemaWalkCommands(c, fullName, entries)
		}
	}
}

// Schema command definitions

var schemaCmd = &cobra.Command{
	Use:     "schema [command...]",
	GroupID: GroupDiag,
	Short:   "Show machine-readable command schemas",
	Long: `Output JSON Schema descriptions of gt commands.

Without arguments, prints an index of all commands.
With a command name, prints the full schema for that command.

Examples:
  gt schema              # List all commands
  gt schema hook         # Schema for 'gt hook'
  gt schema mail send    # Schema for 'gt mail send'`,
	Annotations:  map[string]string{AnnotationPolecatSafe: "true"},
	SilenceUsage: true,
	RunE:         runSchema,
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}

func runSchema(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return schemaOutputJSON(buildSchemaIndex(rootCmd))
	}
	return runSchemaDetail(args)
}

func runSchemaDetail(args []string) error {
	// Placeholder — implemented in Task 2
	return fmt.Errorf("detail mode not yet implemented")
}

func schemaOutputJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// findCommand resolves a space-separated command path (e.g., ["mail", "send"])
// to the corresponding Cobra command.
func findCommand(root *cobra.Command, path []string) (*cobra.Command, error) {
	current := root
	for _, name := range path {
		found := false
		for _, c := range current.Commands() {
			if c.Name() == name || schemaContains(c.Aliases, name) {
				current = c
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown command: %s", strings.Join(path, " "))
		}
	}
	return current, nil
}

func schemaContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
