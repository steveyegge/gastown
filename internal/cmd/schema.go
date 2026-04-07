package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// SchemaDetail is the full JSON Schema for a single command.
type SchemaDetail struct {
	Schema       string                 `json:"$schema"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Group        string                 `json:"group,omitempty"`
	Aliases      []string               `json:"aliases,omitempty"`
	Annotations  map[string]interface{} `json:"annotations,omitempty"`
	Arguments    *SchemaArguments       `json:"arguments,omitempty"`
	Flags        map[string]interface{} `json:"flags,omitempty"`
	OutputSchema interface{}            `json:"outputSchema,omitempty"`
	Subcommands  []string               `json:"subcommands,omitempty"`
}

// SchemaArguments describes positional arguments for a command.
type SchemaArguments struct {
	Type  string          `json:"type"`
	Items []SchemaArgItem `json:"items,omitempty"`
}

// SchemaArgItem describes a single positional argument.
type SchemaArgItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// SchemaFlag describes a single flag's JSON Schema entry.
type SchemaFlag struct {
	Type        string       `json:"type"`
	Description string       `json:"description,omitempty"`
	Shorthand   string       `json:"shorthand,omitempty"`
	Default     interface{}  `json:"default,omitempty"`
	Items       *SchemaItems `json:"items,omitempty"`
}

// SchemaItems is used for array-type flags.
type SchemaItems struct {
	Type string `json:"type"`
}

const jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// AnnotationOutputSchema holds a JSON Schema string describing the command's
// JSON output. Commands add this to their Annotations map to provide typed
// output contracts.
const AnnotationOutputSchema = "outputSchema"

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
	detail, err := buildSchemaDetail(rootCmd, args)
	if err != nil {
		return err
	}
	return schemaOutputJSON(detail)
}

// buildSchemaDetail resolves a command path and returns its full JSON Schema.
func buildSchemaDetail(root *cobra.Command, path []string) (*SchemaDetail, error) {
	cmd, err := findCommand(root, path)
	if err != nil {
		return nil, err
	}

	fullName := root.Name() + " " + strings.Join(path, " ")

	detail := &SchemaDetail{
		Schema:      jsonSchemaDraft,
		Name:        fullName,
		Description: cmd.Short,
		Group:       cmd.GroupID,
	}

	if len(cmd.Aliases) > 0 {
		detail.Aliases = cmd.Aliases
	}

	detail.Flags = buildFlagProperties(cmd)
	detail.Arguments = extractArgSchema(cmd)

	// Subcommands
	for _, sub := range cmd.Commands() {
		if !sub.Hidden {
			detail.Subcommands = append(detail.Subcommands, sub.Name())
		}
	}

	if len(cmd.Annotations) > 0 {
		annotations := make(map[string]interface{}, len(cmd.Annotations))
		for k, v := range cmd.Annotations {
			switch v {
			case "true":
				annotations[k] = true
			case "false":
				annotations[k] = false
			default:
				annotations[k] = v
			}
		}
		detail.Annotations = annotations
	}

	// Output schema from annotation (if present)
	if raw, ok := cmd.Annotations[AnnotationOutputSchema]; ok {
		var parsed interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			detail.OutputSchema = parsed
		}
	}

	return detail, nil
}

// buildFlagProperties converts a command's flags into a JSON Schema properties map.
func buildFlagProperties(cmd *cobra.Command) map[string]interface{} {
	properties := make(map[string]interface{})

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}

		sf := SchemaFlag{
			Type:        flagTypeToJSONSchema(f.Value.Type()),
			Description: f.Usage,
			Shorthand:   f.Shorthand,
		}

		// Set default only if non-zero
		def := f.DefValue
		if def != "" && def != "false" && def != "0" && def != "[]" {
			sf.Default = def
		}

		// Array types get items
		if sf.Type == "array" {
			sf.Items = &SchemaItems{Type: "string"}
		}

		properties[f.Name] = sf
	})

	if len(properties) == 0 {
		return nil
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
}

// flagTypeToJSONSchema maps pflag type strings to JSON Schema types.
func flagTypeToJSONSchema(flagType string) string {
	switch flagType {
	case "bool":
		return "boolean"
	case "int", "int32", "int64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "stringSlice", "stringArray":
		return "array"
	default:
		return "string"
	}
}

// extractArgSchema parses cmd.Use to extract positional argument names and optionality.
func extractArgSchema(cmd *cobra.Command) *SchemaArguments {
	use := cmd.Use
	// Strip the command name (first word)
	idx := strings.Index(use, " ")
	if idx < 0 {
		return nil
	}
	argPart := strings.TrimSpace(use[idx+1:])
	if argPart == "" {
		return nil
	}

	var items []SchemaArgItem
	for _, token := range strings.Fields(argPart) {
		// Skip variadic/repeat indicators
		if token == "..." || token == "[...]" {
			continue
		}
		required := !strings.HasPrefix(token, "[")
		name := strings.Trim(token, "<>[]")
		name = strings.TrimSuffix(name, "...")
		if name == "" {
			continue
		}
		items = append(items, SchemaArgItem{Name: name, Required: required})
	}

	if len(items) == 0 {
		return nil
	}

	return &SchemaArguments{
		Type:  "array",
		Items: items,
	}
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
