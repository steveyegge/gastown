package sourcemap

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sourceMap represents a Source Map v3 JSON structure.
type sourceMap struct {
	Version  int      `json:"version"`
	Sources  []string `json:"sources"`
	Names    []string `json:"names"`
	Mappings string   `json:"mappings"`
}

// mapping represents a single decoded source map segment.
type mapping struct {
	GenCol  int
	SrcIdx  int
	SrcLine int
	SrcCol  int
	NameIdx int
	HasName bool
	HasSrc  bool
}

// Resolver applies source maps to resolve minified locations back to originals.
type Resolver struct{}

// Resolve takes raw source map JSON data and a generated line/column (1-based line, 0-based column)
// and returns the original file, line (1-based), column (0-based), and function name.
func (r *Resolver) Resolve(mapData []byte, line, column int) (origFile string, origLine, origCol int, origName string, err error) {
	var sm sourceMap
	if err := json.Unmarshal(mapData, &sm); err != nil {
		return "", 0, 0, "", fmt.Errorf("parse source map: %w", err)
	}
	if sm.Version != 3 {
		return "", 0, 0, "", fmt.Errorf("unsupported source map version: %d", sm.Version)
	}

	// Find the best matching mapping for the given line/column.
	// Lines in mappings are 0-indexed; the caller passes 1-based line numbers.
	targetLine := line - 1
	if targetLine < 0 {
		targetLine = 0
	}

	// Walk through line groups to find the right line.
	var best *mapping
	lines := strings.Split(sm.Mappings, ";")
	genCol := 0
	srcIdx := 0
	srcLine := 0
	srcCol := 0
	nameIdx := 0

	for lineIdx, lineStr := range lines {
		genCol = 0 // reset generated column per line
		if lineStr == "" {
			continue
		}

		segments := strings.Split(lineStr, ",")
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			fields, decErr := decodeVLQSegment(seg)
			if decErr != nil {
				continue
			}
			if len(fields) < 1 {
				continue
			}

			genCol += fields[0]

			var m mapping
			m.GenCol = genCol

			if len(fields) >= 4 {
				srcIdx += fields[1]
				srcLine += fields[2]
				srcCol += fields[3]
				m.SrcIdx = srcIdx
				m.SrcLine = srcLine
				m.SrcCol = srcCol
				m.HasSrc = true
			}
			if len(fields) >= 5 {
				nameIdx += fields[4]
				m.NameIdx = nameIdx
				m.HasName = true
			}

			if lineIdx == targetLine && m.HasSrc {
				if m.GenCol <= column {
					best = &mapping{
						GenCol:  m.GenCol,
						SrcIdx:  m.SrcIdx,
						SrcLine: m.SrcLine,
						SrcCol:  m.SrcCol,
						NameIdx: m.NameIdx,
						HasName: m.HasName,
						HasSrc:  m.HasSrc,
					}
				}
			}
		}
	}

	if best == nil {
		return "", 0, 0, "", fmt.Errorf("no mapping found for line %d, column %d", line, column)
	}

	if best.SrcIdx < 0 || best.SrcIdx >= len(sm.Sources) {
		return "", 0, 0, "", fmt.Errorf("source index %d out of range", best.SrcIdx)
	}

	origFile = sm.Sources[best.SrcIdx]
	origLine = best.SrcLine + 1 // convert back to 1-based
	origCol = best.SrcCol

	if best.HasName && best.NameIdx >= 0 && best.NameIdx < len(sm.Names) {
		origName = sm.Names[best.NameIdx]
	}

	return origFile, origLine, origCol, origName, nil
}

// decodeVLQSegment decodes a single VLQ-encoded segment string into a slice of integers.
func decodeVLQSegment(seg string) ([]int, error) {
	var result []int
	i := 0
	for i < len(seg) {
		val, consumed, err := decodeVLQ(seg[i:])
		if err != nil {
			return nil, err
		}
		result = append(result, val)
		i += consumed
	}
	return result, nil
}

const vlqBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// decodeVLQ decodes one VLQ value from the string, returning the decoded int and number of chars consumed.
func decodeVLQ(s string) (int, int, error) {
	shift := 0
	result := 0

	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(vlqBase64, s[i])
		if idx < 0 {
			return 0, 0, fmt.Errorf("invalid VLQ character: %c", s[i])
		}

		// Each base64 digit encodes 6 bits. Bit 5 (0x20) is the continuation bit.
		// Bits 0-4 are data bits.
		result |= (idx & 0x1F) << shift
		shift += 5

		if idx&0x20 == 0 {
			// No continuation bit — we're done.
			// Bit 0 of result is the sign bit.
			if result&1 == 1 {
				return -(result >> 1), i + 1, nil
			}
			return result >> 1, i + 1, nil
		}
	}

	return 0, 0, fmt.Errorf("unterminated VLQ sequence")
}
