package sourcemap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeVLQ(t *testing.T) {
	tests := []struct {
		input string
		want  int
		chars int
	}{
		{"A", 0, 1},
		{"C", 1, 1},
		{"D", -1, 1},
		{"E", 2, 1},
		{"F", -2, 1},
		{"K", 5, 1},
		{"L", -5, 1},
		// Multi-char: 173 encoded as "1M" (continuation)
		// Let's test known values from real source maps.
		{"gB", 16, 2},
	}

	for _, tt := range tests {
		got, n, err := decodeVLQ(tt.input)
		if err != nil {
			t.Errorf("decodeVLQ(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("decodeVLQ(%q) = %d, want %d", tt.input, got, tt.want)
		}
		if n != tt.chars {
			t.Errorf("decodeVLQ(%q) consumed %d chars, want %d", tt.input, n, tt.chars)
		}
	}
}

func TestDecodeVLQSegment(t *testing.T) {
	// "AAAA" should decode to [0, 0, 0, 0]
	fields, err := decodeVLQSegment("AAAA")
	if err != nil {
		t.Fatalf("decodeVLQSegment(AAAA): %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}
	for i, f := range fields {
		if f != 0 {
			t.Errorf("field[%d] = %d, want 0", i, f)
		}
	}

	// "KACM" = [5, 0, 1, 6]
	fields, err = decodeVLQSegment("KACM")
	if err != nil {
		t.Fatalf("decodeVLQSegment(KACM): %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d: %v", len(fields), fields)
	}
	want := []int{5, 0, 1, 6}
	for i, w := range want {
		if fields[i] != w {
			t.Errorf("field[%d] = %d, want %d", i, fields[i], w)
		}
	}
}

// TestResolverWithRealSourceMap tests resolution with a realistic source map.
func TestResolverWithRealSourceMap(t *testing.T) {
	// A minimal but valid source map: maps line 1, column 0 of generated file
	// to line 3, column 4 of "src/app.js", name "handleClick".
	//
	// Mappings: "AAEGA" means:
	//   A=0 (genCol), A=0 (srcIdx), E=2 (srcLine -> line 3 in 1-based),
	//   G=3 (srcCol), A=0 (nameIdx)
	sm := `{
		"version": 3,
		"sources": ["src/app.js"],
		"names": ["handleClick"],
		"mappings": "AAEGA"
	}`

	resolver := &Resolver{}
	origFile, origLine, origCol, origName, err := resolver.Resolve([]byte(sm), 1, 0)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if origFile != "src/app.js" {
		t.Errorf("origFile = %q, want %q", origFile, "src/app.js")
	}
	if origLine != 3 {
		t.Errorf("origLine = %d, want 3", origLine)
	}
	if origCol != 3 {
		t.Errorf("origCol = %d, want 3", origCol)
	}
	if origName != "handleClick" {
		t.Errorf("origName = %q, want %q", origName, "handleClick")
	}
}

// TestResolverMultiLine tests resolution across multiple generated lines.
func TestResolverMultiLine(t *testing.T) {
	// Line 1: col 0 -> src/a.js line 1 col 0
	// Line 2: col 0 -> src/b.js line 5 col 2
	// "AAAA" = genCol=0, src=0, srcLine=0, srcCol=0
	// ";ACIAC" = (line 2) genCol=0, src=+1, srcLine=+4, srcCol=+0, name=+1  -- Wait, let me be precise.
	// We want: line 2, col 0, src=1(b.js), srcLine=4(line5 0-indexed), srcCol=2
	// Relative deltas from previous segment (0,0,0,0):
	//   genCol=0, srcIdx=+1, srcLine=+4, srcCol=+2
	// VLQ: 0=A, 1=C, 4=I, 2=E
	sm := `{
		"version": 3,
		"sources": ["src/a.js", "src/b.js"],
		"names": [],
		"mappings": "AAAA;ACIE"
	}`

	resolver := &Resolver{}

	// Line 1 col 0 should map to src/a.js line 1 col 0
	origFile, origLine, origCol, _, err := resolver.Resolve([]byte(sm), 1, 0)
	if err != nil {
		t.Fatalf("Resolve line 1: %v", err)
	}
	if origFile != "src/a.js" {
		t.Errorf("line1 origFile = %q, want %q", origFile, "src/a.js")
	}
	if origLine != 1 {
		t.Errorf("line1 origLine = %d, want 1", origLine)
	}
	if origCol != 0 {
		t.Errorf("line1 origCol = %d, want 0", origCol)
	}

	// Line 2 col 0 should map to src/b.js line 5 col 2
	origFile, origLine, origCol, _, err = resolver.Resolve([]byte(sm), 2, 0)
	if err != nil {
		t.Fatalf("Resolve line 2: %v", err)
	}
	if origFile != "src/b.js" {
		t.Errorf("line2 origFile = %q, want %q", origFile, "src/b.js")
	}
	if origLine != 5 {
		t.Errorf("line2 origLine = %d, want 5", origLine)
	}
	if origCol != 2 {
		t.Errorf("line2 origCol = %d, want 2", origCol)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	data := []byte(`{"version":3,"mappings":"AAAA"}`)

	// Save
	if err := store.Save(1, "v1.0.0", "app.js.map", data); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load
	got, err := store.Load(1, "v1.0.0", "app.js.map")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Load returned %q, want %q", got, data)
	}

	// List
	files, err := store.List(1, "v1.0.0")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0] != "app.js.map" {
		t.Errorf("List = %v, want [app.js.map]", files)
	}

	// Delete
	if err := store.Delete(1, "v1.0.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// List after delete should be empty.
	files, err = store.List(1, "v1.0.0")
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("List after delete = %v, want []", files)
	}
}

func TestStorePermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	data := []byte(`test data`)
	if err := store.Save(42, "rel", "test.map", data); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Check directory permissions.
	info, err := os.Stat(filepath.Join(dir, "42", "rel"))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o750 {
		t.Errorf("dir perm = %o, want 750", perm)
	}

	// Check file permissions.
	info, err = os.Stat(filepath.Join(dir, "42", "rel", "test.map"))
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	perm = info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	_, err := store.Load(1, "v1", "nonexistent.map")
	if err == nil {
		t.Error("expected error loading nonexistent file")
	}
}

func TestStoreListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	files, err := store.List(1, "v1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if files != nil {
		t.Errorf("List = %v, want nil", files)
	}
}

func TestSymbolicate(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create a source map: line 1, col 10 of app.min.js -> src/app.js line 5 col 3, name "onClick"
	// genCol=10(U), srcIdx=0(A), srcLine=4(I), srcCol=3(G), nameIdx=0(A)
	sm := `{
		"version": 3,
		"sources": ["src/app.js"],
		"names": ["onClick"],
		"mappings": "UAIGA"
	}`
	if err := store.Save(1, "v2.0.0", "app.min.js.map", []byte(sm)); err != nil {
		t.Fatalf("Save source map: %v", err)
	}

	// Build a mock Sentry event with a minified stack trace.
	event := map[string]interface{}{
		"release": "v2.0.0",
		"exception": map[string]interface{}{
			"values": []interface{}{
				map[string]interface{}{
					"type":  "TypeError",
					"value": "Cannot read property 'x'",
					"stacktrace": map[string]interface{}{
						"frames": []interface{}{
							map[string]interface{}{
								"abs_path": "https://example.com/static/app.min.js",
								"filename": "app.min.js",
								"lineno":   1,
								"colno":    10,
								"function": "n",
							},
						},
					},
				},
			},
		},
	}

	raw, _ := json.Marshal(event)

	out, err := Symbolicate(store, 1, "v2.0.0", json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Symbolicate: %v", err)
	}

	// Parse the result and verify the frame was updated.
	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	exc, _ := result["exception"].(map[string]interface{})
	vals, _ := exc["values"].([]interface{})
	val, _ := vals[0].(map[string]interface{})
	st, _ := val["stacktrace"].(map[string]interface{})
	frames, _ := st["frames"].([]interface{})
	frame, _ := frames[0].(map[string]interface{})

	if frame["filename"] != "src/app.js" {
		t.Errorf("filename = %v, want src/app.js", frame["filename"])
	}
	lineno, _ := frame["lineno"].(float64)
	if int(lineno) != 5 {
		t.Errorf("lineno = %v, want 5", frame["lineno"])
	}
	colno, _ := frame["colno"].(float64)
	if int(colno) != 3 {
		t.Errorf("colno = %v, want 3", frame["colno"])
	}
	if frame["function"] != "onClick" {
		t.Errorf("function = %v, want onClick", frame["function"])
	}
	if frame["in_app"] != true {
		t.Errorf("in_app = %v, want true", frame["in_app"])
	}
}

func TestSymbolicateNodeModules(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Source map pointing to a file in node_modules.
	sm := `{
		"version": 3,
		"sources": ["node_modules/react/lib/react.js"],
		"names": [],
		"mappings": "AAEA"
	}`
	if err := store.Save(1, "v1", "vendor.min.js.map", []byte(sm)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	event := map[string]interface{}{
		"release": "v1",
		"exception": map[string]interface{}{
			"values": []interface{}{
				map[string]interface{}{
					"type": "Error",
					"stacktrace": map[string]interface{}{
						"frames": []interface{}{
							map[string]interface{}{
								"filename": "vendor.min.js",
								"lineno":   1,
								"colno":    0,
							},
						},
					},
				},
			},
		},
	}

	raw, _ := json.Marshal(event)
	out, err := Symbolicate(store, 1, "v1", json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Symbolicate: %v", err)
	}

	var result map[string]interface{}
	_ = json.Unmarshal(out, &result)

	exc, _ := result["exception"].(map[string]interface{})
	vals, _ := exc["values"].([]interface{})
	val, _ := vals[0].(map[string]interface{})
	st, _ := val["stacktrace"].(map[string]interface{})
	frames, _ := st["frames"].([]interface{})
	frame, _ := frames[0].(map[string]interface{})

	if frame["in_app"] != false {
		t.Errorf("in_app = %v, want false for node_modules path", frame["in_app"])
	}
}

func TestSymbolicateNoRelease(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	raw := json.RawMessage(`{"exception":{"values":[{"type":"Error"}]}}`)
	out, err := Symbolicate(store, 1, "", raw)
	if err != nil {
		t.Fatalf("Symbolicate: %v", err)
	}
	if string(out) != string(raw) {
		t.Error("expected unchanged output when release is empty")
	}
}

func TestSymbolicateNoMaps(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	raw := json.RawMessage(`{"release":"v1","exception":{"values":[{"type":"Error"}]}}`)
	out, err := Symbolicate(store, 1, "v1", raw)
	if err != nil {
		t.Fatalf("Symbolicate: %v", err)
	}
	// No source maps uploaded — event should be returned unchanged.
	if string(out) != string(raw) {
		t.Error("expected unchanged output when no source maps exist")
	}
}
