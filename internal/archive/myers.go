package archive

// EditType represents the type of edit in a Myers diff.
type EditType int

const (
	// EditEqual indicates lines that match between sequences.
	EditEqual EditType = iota
	// EditInsert indicates lines added in the new sequence.
	EditInsert
	// EditDelete indicates lines removed from the old sequence.
	EditDelete
)

// String returns a human-readable name for the edit type.
func (et EditType) String() string {
	switch et {
	case EditEqual:
		return "equal"
	case EditInsert:
		return "insert"
	case EditDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// DiffEdit represents a single edit operation from Myers diff.
type DiffEdit struct {
	Type  EditType // Type of edit
	PrevI int      // Index in prev sequence (-1 for inserts)
	NextI int      // Index in next sequence (-1 for deletes)
	Count int      // Number of consecutive lines
}

// MaxDiffThreshold is the maximum edit distance before treating as full redraw.
// For typical terminal heights (~100 lines), this provides a reasonable cutoff.
const MaxDiffThreshold = 200

// traceEntry stores the V array at each step for Myers backtracking.
type traceEntry struct {
	v []int
	d int
}

// MyersDiff computes the shortest edit script between two sequences of hashes
// using Myers' O(ND) algorithm. Returns nil if edit distance exceeds threshold.
//
// Time complexity: O((N+M)·D) where D is the edit distance
// Space complexity: O(N+M)
//
// For small D (typical case with similar screens), this is near-linear.
func MyersDiff(prevHashes, nextHashes []uint64) []DiffEdit {
	n := len(prevHashes)
	m := len(nextHashes)

	// Handle empty sequences
	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 {
		return []DiffEdit{{Type: EditInsert, PrevI: -1, NextI: 0, Count: m}}
	}
	if m == 0 {
		return []DiffEdit{{Type: EditDelete, PrevI: 0, NextI: -1, Count: n}}
	}

	// Myers algorithm uses a V array indexed by diagonal k = x - y
	// We offset by max to handle negative indices
	maxDiag := n + m
	vSize := 2*maxDiag + 1

	// V[k] = furthest x position reached on diagonal k
	v := make([]int, vSize)
	// Initialize with -1 to indicate unvisited
	for i := range v {
		v[i] = -1
	}

	var trace []traceEntry

	// offset converts diagonal k to array index
	offset := func(k int) int { return k + maxDiag }

	// Main Myers loop: find shortest edit script
	v[offset(1)] = 0
	var found bool
	var finalD int

	for d := 0; d <= min(maxDiag, MaxDiffThreshold); d++ {
		// Save current V for backtracking
		vCopy := make([]int, vSize)
		copy(vCopy, v)
		trace = append(trace, traceEntry{v: vCopy, d: d})

		for k := -d; k <= d; k += 2 {
			// Decide whether to go down or right
			var x int
			if k == -d || (k != d && v[offset(k-1)] < v[offset(k+1)]) {
				// Go down (insert)
				x = v[offset(k+1)]
			} else {
				// Go right (delete)
				x = v[offset(k-1)] + 1
			}

			y := x - k

			// Follow diagonal (equal elements)
			for x < n && y < m && hashesMatch(prevHashes[x], nextHashes[y]) {
				x++
				y++
			}

			v[offset(k)] = x

			// Check if we've reached the end
			if x >= n && y >= m {
				found = true
				finalD = d
				break
			}
		}

		if found {
			break
		}
	}

	// If we exceeded threshold, return nil to signal full redraw
	if !found {
		return nil
	}

	// Backtrack to construct the edit script
	return backtrack(trace, prevHashes, nextHashes, finalD, n, m, maxDiag)
}

// hashesMatch returns true if two hashes match.
// Zero hashes (empty lines) don't match to avoid false positives.
func hashesMatch(a, b uint64) bool {
	return a != 0 && a == b
}

// backtrack reconstructs the edit script from the Myers trace.
func backtrack(trace []traceEntry, prevHashes, nextHashes []uint64, d, n, m, maxDiag int) []DiffEdit {
	offset := func(k int) int { return k + maxDiag }

	// Work backwards from (n, m)
	x, y := n, m
	var edits []DiffEdit

	for d > 0 {
		entry := trace[d]
		v := entry.v
		k := x - y

		// Determine previous k
		var prevK int
		if k == -d || (k != d && v[offset(k-1)] < v[offset(k+1)]) {
			prevK = k + 1 // Came from above (insert)
		} else {
			prevK = k - 1 // Came from left (delete)
		}

		prevX := v[offset(prevK)]
		prevY := prevX - prevK

		// Add diagonal moves (equal)
		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, DiffEdit{
				Type:  EditEqual,
				PrevI: x,
				NextI: y,
				Count: 1,
			})
		}

		// Add the insert or delete
		if d > 0 {
			if prevK == k+1 {
				// Insert
				y--
				edits = append(edits, DiffEdit{
					Type:  EditInsert,
					PrevI: -1,
					NextI: y,
					Count: 1,
				})
			} else {
				// Delete
				x--
				edits = append(edits, DiffEdit{
					Type:  EditDelete,
					PrevI: x,
					NextI: -1,
					Count: 1,
				})
			}
		}

		d--
	}

	// Handle initial diagonal
	for x > 0 && y > 0 && hashesMatch(prevHashes[x-1], nextHashes[y-1]) {
		x--
		y--
		edits = append(edits, DiffEdit{
			Type:  EditEqual,
			PrevI: x,
			NextI: y,
			Count: 1,
		})
	}

	// Reverse to get forward order
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}

	// Merge consecutive edits of the same type
	return mergeEdits(edits)
}

// mergeEdits combines consecutive edits of the same type.
func mergeEdits(edits []DiffEdit) []DiffEdit {
	if len(edits) == 0 {
		return nil
	}

	var merged []DiffEdit
	current := edits[0]

	for i := 1; i < len(edits); i++ {
		next := edits[i]

		// Check if we can merge
		canMerge := current.Type == next.Type

		if canMerge {
			switch current.Type {
			case EditEqual:
				// Equal edits must be consecutive in both sequences
				canMerge = current.PrevI+current.Count == next.PrevI &&
					current.NextI+current.Count == next.NextI
			case EditInsert:
				// Inserts must be consecutive in next sequence
				canMerge = current.NextI+current.Count == next.NextI
			case EditDelete:
				// Deletes must be consecutive in prev sequence
				canMerge = current.PrevI+current.Count == next.PrevI
			}
		}

		if canMerge {
			current.Count += next.Count
		} else {
			merged = append(merged, current)
			current = next
		}
	}

	merged = append(merged, current)
	return merged
}

// MyersLCS extracts LCS indices from Myers diff result.
// Returns indices into the prev array that are part of the longest common subsequence.
// This provides compatibility with the LCS-based approach.
func MyersLCS(prevHashes, nextHashes []uint64) []int {
	edits := MyersDiff(prevHashes, nextHashes)
	if edits == nil {
		return nil
	}

	var lcsIndices []int
	for _, edit := range edits {
		if edit.Type == EditEqual {
			for i := 0; i < edit.Count; i++ {
				lcsIndices = append(lcsIndices, edit.PrevI+i)
			}
		}
	}

	return lcsIndices
}
