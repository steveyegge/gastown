// Stub package for bdbranch analyzer tests.
package beads

type Beads struct{}

func New(dir string) *Beads                       { return nil }
func NewWithBeadsDir(dir, beadsDir string) *Beads  { return nil }
func NewIsolated(dir string) *Beads                { return nil }
func StripBdBranch(environ []string) []string      { return nil }

func (b *Beads) OnMain() *Beads { return b }
