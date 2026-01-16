package archive

import (
	"hash/fnv"
	"strings"
)

// DiffType represents the type of change in a diff region.
type DiffType int

const (
	// Unchanged indicates lines that are identical in both captures.
	Unchanged DiffType = iota
	// Inserted indicates lines that appear only in the new capture.
	Inserted
	// Deleted indicates lines that appear only in the old capture.
	Deleted
	// Modified indicates lines that changed between captures.
	Modified
)

// String returns a human-readable name for the diff type.
func (dt DiffType) String() string {
	switch dt {
	case Unchanged:
		return "unchanged"
	case Inserted:
		return "inserted"
	case Deleted:
		return "deleted"
	case Modified:
		return "modified"
	default:
		return "unknown"
	}
}

// DiffRegion represents a contiguous region of changes between two captures.
type DiffRegion struct {
	Type      DiffType
	PrevStart int // Start index in previous capture (inclusive)
	PrevEnd   int // End index in previous capture (exclusive)
	NextStart int // Start index in next capture (inclusive)
	NextEnd   int // End index in next capture (exclusive)
}

// AnchorPair represents a matched pair of lines between two captures.
type AnchorPair struct {
	PrevIndex int    // Index in previous capture
	NextIndex int    // Index in next capture
	Hash      uint64 // Hash of the matched line
}

// anchorToken patterns that indicate stable content.
var anchorTokens = []string{
	"> ",      // Claude prompt
	"$ ",      // Shell prompt
	">>> ",    // Python prompt
	"```",     // Code fence
	"---",     // Horizontal rule / YAML separator
	"===",     // Section separator
	"###",     // Markdown heading
	"//",      // Comment
	"/*",      // Block comment
	"func ",   // Go function
	"def ",    // Python function
	"class ",  // Class definition
	"import ", // Import statement
}

// HashLines computes a hash for each line using FNV-1a.
// Empty lines get a hash of 0 to avoid false matches on whitespace.
func HashLines(lines []string) []uint64 {
	hashes := make([]uint64, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			hashes[i] = 0
			continue
		}
		h := fnv.New64a()
		h.Write([]byte(trimmed))
		hashes[i] = h.Sum64()
	}
	return hashes
}

// isAnchorCandidate returns true if a line is a good anchor candidate.
// Anchor candidates are lines that are likely to be stable across redraws.
func isAnchorCandidate(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Long lines are more unique
	if len(trimmed) > 40 {
		return true
	}

	// Lines with anchor tokens
	for _, token := range anchorTokens {
		if strings.Contains(trimmed, token) {
			return true
		}
	}

	// Lines that look like file paths
	if strings.Contains(trimmed, "/") && !strings.HasPrefix(trimmed, "http") {
		return true
	}

	return false
}

// FindAnchors finds matching anchor pairs between two captures.
// It prioritizes lines that are good anchor candidates (long, stable tokens).
func FindAnchors(prev, next []string) []AnchorPair {
	prevHashes := HashLines(prev)
	nextHashes := HashLines(next)

	// Build a map of hash -> indices for next capture
	nextHashMap := make(map[uint64][]int)
	for i, h := range nextHashes {
		if h != 0 { // Skip empty lines
			nextHashMap[h] = append(nextHashMap[h], i)
		}
	}

	var anchors []AnchorPair

	// Find matches, prioritizing anchor candidates
	for i, h := range prevHashes {
		if h == 0 {
			continue
		}
		if indices, ok := nextHashMap[h]; ok {
			// Prefer matches that maintain relative order
			for _, j := range indices {
				// Check if this is a good anchor
				if isAnchorCandidate(prev[i]) || isAnchorCandidate(next[j]) {
					anchors = append(anchors, AnchorPair{
						PrevIndex: i,
						NextIndex: j,
						Hash:      h,
					})
					break // Take first match to maintain order
				}
			}
		}
	}

	return anchors
}

// ComputeLCS finds the longest common subsequence of hashes.
// Returns indices into the prev array that are part of the LCS.
// This forms the "stable spine" of content that didn't change.
func ComputeLCS(prevHashes, nextHashes []uint64) []int {
	m, n := len(prevHashes), len(nextHashes)
	if m == 0 || n == 0 {
		return nil
	}

	// DP table for LCS length
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if prevHashes[i-1] == nextHashes[j-1] && prevHashes[i-1] != 0 {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	// Backtrack to find LCS indices
	var lcsIndices []int
	i, j := m, n
	for i > 0 && j > 0 {
		if prevHashes[i-1] == nextHashes[j-1] && prevHashes[i-1] != 0 {
			lcsIndices = append([]int{i - 1}, lcsIndices...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return lcsIndices
}

// DiffWithAnchors computes a diff using anchor-based matching.
// This handles full-screen redraws where scroll detection fails,
// by finding stable content and treating everything else as edits.
func DiffWithAnchors(prev, next []string) []DiffRegion {
	if len(prev) == 0 && len(next) == 0 {
		return nil
	}
	if len(prev) == 0 {
		return []DiffRegion{{
			Type:      Inserted,
			PrevStart: 0,
			PrevEnd:   0,
			NextStart: 0,
			NextEnd:   len(next),
		}}
	}
	if len(next) == 0 {
		return []DiffRegion{{
			Type:      Deleted,
			PrevStart: 0,
			PrevEnd:   len(prev),
			NextStart: 0,
			NextEnd:   0,
		}}
	}

	prevHashes := HashLines(prev)
	nextHashes := HashLines(next)

	// Find LCS - the stable spine
	lcsIndices := ComputeLCS(prevHashes, nextHashes)

	// Build mapping from prev index to next index for LCS elements
	lcsMap := make(map[int]int)
	if len(lcsIndices) > 0 {
		// Find corresponding next indices for LCS
		nextIdx := 0
		for _, prevIdx := range lcsIndices {
			// Find matching hash in next starting from nextIdx
			for nextIdx < len(nextHashes) {
				if nextHashes[nextIdx] == prevHashes[prevIdx] && prevHashes[prevIdx] != 0 {
					lcsMap[prevIdx] = nextIdx
					nextIdx++
					break
				}
				nextIdx++
			}
		}
	}

	// Generate diff regions
	var regions []DiffRegion
	prevPos, nextPos := 0, 0

	for _, prevIdx := range lcsIndices {
		nextIdx, ok := lcsMap[prevIdx]
		if !ok {
			continue
		}

		// Handle gap before this anchor
		if prevPos < prevIdx || nextPos < nextIdx {
			region := DiffRegion{
				PrevStart: prevPos,
				PrevEnd:   prevIdx,
				NextStart: nextPos,
				NextEnd:   nextIdx,
			}

			// Classify the gap
			if prevPos == prevIdx {
				region.Type = Inserted
			} else if nextPos == nextIdx {
				region.Type = Deleted
			} else {
				region.Type = Modified
			}

			if region.PrevStart != region.PrevEnd || region.NextStart != region.NextEnd {
				regions = append(regions, region)
			}
		}

		// Add unchanged region for the anchor
		regions = append(regions, DiffRegion{
			Type:      Unchanged,
			PrevStart: prevIdx,
			PrevEnd:   prevIdx + 1,
			NextStart: nextIdx,
			NextEnd:   nextIdx + 1,
		})

		prevPos = prevIdx + 1
		nextPos = nextIdx + 1
	}

	// Handle trailing content after last anchor
	if prevPos < len(prev) || nextPos < len(next) {
		region := DiffRegion{
			PrevStart: prevPos,
			PrevEnd:   len(prev),
			NextStart: nextPos,
			NextEnd:   len(next),
		}

		if prevPos == len(prev) {
			region.Type = Inserted
		} else if nextPos == len(next) {
			region.Type = Deleted
		} else {
			region.Type = Modified
		}

		regions = append(regions, region)
	}

	return mergeAdjacentRegions(regions)
}

// mergeAdjacentRegions combines adjacent regions of the same type.
func mergeAdjacentRegions(regions []DiffRegion) []DiffRegion {
	if len(regions) <= 1 {
		return regions
	}

	var merged []DiffRegion
	current := regions[0]

	for i := 1; i < len(regions); i++ {
		next := regions[i]

		// Merge if same type and adjacent
		if current.Type == next.Type &&
			current.PrevEnd == next.PrevStart &&
			current.NextEnd == next.NextStart {
			current.PrevEnd = next.PrevEnd
			current.NextEnd = next.NextEnd
		} else {
			merged = append(merged, current)
			current = next
		}
	}

	merged = append(merged, current)
	return merged
}
