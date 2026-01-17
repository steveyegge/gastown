package narrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/util"
)

// Chapter represents a narrative chapter for indexing.
type Chapter struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
}

// OutputWriter handles writing narrative content to disk.
type OutputWriter struct {
	outputDir string
}

// NewOutputWriter creates an OutputWriter for the given directory.
// The directory will be created if it doesn't exist.
func NewOutputWriter(outputDir string) *OutputWriter {
	return &OutputWriter{outputDir: outputDir}
}

// DefaultOutputDir returns the default narrative output directory for a town.
func DefaultOutputDir(townRoot string) string {
	return filepath.Join(townRoot, "narrative")
}

// EnsureDir creates the output directory if it doesn't exist.
func (w *OutputWriter) EnsureDir() error {
	return os.MkdirAll(w.outputDir, 0755)
}

// WriteChapter writes a chapter file as chapter-NNN.md.
// The chapter number is zero-padded to 3 digits (e.g., chapter-001.md).
func (w *OutputWriter) WriteChapter(number int, content string) error {
	if err := w.EnsureDir(); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	filename := fmt.Sprintf("chapter-%03d.md", number)
	path := filepath.Join(w.outputDir, filename)

	if err := util.AtomicWriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing chapter %d: %w", number, err)
	}

	return nil
}

// ChapterPath returns the path for a chapter file.
func (w *OutputWriter) ChapterPath(number int) string {
	filename := fmt.Sprintf("chapter-%03d.md", number)
	return filepath.Join(w.outputDir, filename)
}

// WriteIndex writes an index.md file with a table of contents.
// The TOC links to each chapter file with its title and optional summary.
func (w *OutputWriter) WriteIndex(chapters []Chapter) error {
	if err := w.EnsureDir(); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Narrative Index\n\n")

	if len(chapters) == 0 {
		sb.WriteString("*No chapters yet.*\n")
	} else {
		sb.WriteString("## Chapters\n\n")
		for _, ch := range chapters {
			filename := fmt.Sprintf("chapter-%03d.md", ch.Number)
			title := ch.Title
			if title == "" {
				title = fmt.Sprintf("Chapter %d", ch.Number)
			}
			sb.WriteString(fmt.Sprintf("- [%s](%s)", title, filename))
			if ch.Summary != "" {
				sb.WriteString(fmt.Sprintf(" - %s", ch.Summary))
			}
			sb.WriteString("\n")
		}
	}

	path := filepath.Join(w.outputDir, "index.md")
	if err := util.AtomicWriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	return nil
}

// IndexPath returns the path to the index file.
func (w *OutputWriter) IndexPath() string {
	return filepath.Join(w.outputDir, "index.md")
}

// ListChapters returns chapter numbers found in the output directory.
func (w *OutputWriter) ListChapters() ([]int, error) {
	entries, err := os.ReadDir(w.outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var chapters []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var num int
		if _, err := fmt.Sscanf(e.Name(), "chapter-%03d.md", &num); err == nil {
			chapters = append(chapters, num)
		}
	}
	return chapters, nil
}
