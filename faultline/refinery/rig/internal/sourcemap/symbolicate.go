package sourcemap

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Symbolicate walks the event JSON, resolves source-mapped stack frames, and returns the modified event.
func Symbolicate(store *Store, projectID int64, release string, raw json.RawMessage) (json.RawMessage, error) {
	if release == "" {
		return raw, nil
	}

	// Check if any source maps exist for this release.
	files, err := store.List(projectID, release)
	if err != nil || len(files) == 0 {
		return raw, nil
	}

	// Build a set of available map filenames for quick lookup.
	mapFiles := make(map[string]bool, len(files))
	for _, f := range files {
		mapFiles[f] = true
	}

	// Parse the event into a generic map so we can modify it.
	var event map[string]interface{}
	if err := json.Unmarshal(raw, &event); err != nil {
		return raw, nil // not valid JSON, return as-is
	}

	resolver := &Resolver{}
	modified := false

	// Walk exception.values[].stacktrace.frames[]
	exception, ok := event["exception"]
	if !ok {
		return raw, nil
	}
	excMap, ok := exception.(map[string]interface{})
	if !ok {
		return raw, nil
	}
	values, ok := excMap["values"]
	if !ok {
		return raw, nil
	}
	valSlice, ok := values.([]interface{})
	if !ok {
		return raw, nil
	}

	for _, val := range valSlice {
		valMap, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		st, ok := valMap["stacktrace"]
		if !ok {
			continue
		}
		stMap, ok := st.(map[string]interface{})
		if !ok {
			continue
		}
		frames, ok := stMap["frames"]
		if !ok {
			continue
		}
		frameSlice, ok := frames.([]interface{})
		if !ok {
			continue
		}

		for i, fr := range frameSlice {
			frMap, ok := fr.(map[string]interface{})
			if !ok {
				continue
			}

			absPath, _ := frMap["abs_path"].(string)
			filename, _ := frMap["filename"].(string)
			lineno := jsonInt(frMap["lineno"])
			colno := jsonInt(frMap["colno"])

			if lineno <= 0 {
				continue
			}

			// Determine the source map filename to look up.
			mapName := guessMapName(absPath, filename, mapFiles)
			if mapName == "" {
				continue
			}

			mapData, err := store.Load(projectID, release, mapName)
			if err != nil {
				continue
			}

			origFile, origLine, origCol, origName, err := resolver.Resolve(mapData, lineno, colno)
			if err != nil {
				continue
			}

			// Update frame fields.
			frMap["filename"] = origFile
			frMap["lineno"] = origLine
			frMap["colno"] = origCol
			if origName != "" {
				frMap["function"] = origName
			}

			// Set in_app based on whether the original file is in node_modules.
			inApp := !strings.Contains(origFile, "node_modules")
			frMap["in_app"] = inApp

			frameSlice[i] = frMap
			modified = true
		}
	}

	if !modified {
		return raw, nil
	}

	out, err := json.Marshal(event)
	if err != nil {
		return raw, fmt.Errorf("symbolicate marshal: %w", err)
	}
	return json.RawMessage(out), nil
}

// guessMapName determines which source map file to use for a given frame.
// It tries: filename + ".map", basename + ".map", and exact match in available files.
func guessMapName(absPath, filename string, available map[string]bool) string {
	candidates := []string{}

	// Try the abs_path-based names first.
	if absPath != "" {
		base := filepath.Base(absPath)
		candidates = append(candidates, base+".map")
		candidates = append(candidates, base)
	}
	if filename != "" {
		base := filepath.Base(filename)
		candidates = append(candidates, base+".map")
		candidates = append(candidates, base)
	}

	// Also try stripping query strings (e.g. app.min.js?v=123 -> app.min.js.map).
	for _, path := range []string{absPath, filename} {
		if path == "" {
			continue
		}
		clean := path
		if idx := strings.IndexByte(clean, '?'); idx >= 0 {
			clean = clean[:idx]
		}
		base := filepath.Base(clean)
		candidates = append(candidates, base+".map")
	}

	for _, c := range candidates {
		if available[c] {
			return c
		}
	}
	return ""
}

// jsonInt extracts an int from a JSON-decoded interface{} (which may be float64).
func jsonInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}
