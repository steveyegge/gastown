package beads

import "os"

func init() {
	// Force bd to use the local repo by default; avoids auto-routing to ~/.beads-planning.
	if os.Getenv("BD_ROUTING_MODE") == "" {
		_ = os.Setenv("BD_ROUTING_MODE", "maintainer")
	}
}
