package doctor

// Session naming constants.
const (
	// GastownSessionPrefix is the prefix for all Gas Town tmux sessions.
	GastownSessionPrefix = "gt-"

	// CrewMarker is the marker used to identify crew sessions.
	CrewMarker = "crew"
)

// Role names for rig components.
const (
	// RoleWitness is the witness role name.
	RoleWitness = "witness"

	// RoleRefinery is the refinery role name.
	RoleRefinery = "refinery"
)

// RigMarkers are directory names that indicate a directory is a rig.
var RigMarkers = []string{CrewMarker, "polecats", RoleWitness, RoleRefinery}

// ExcludedDirs are directory names that should not be considered as rigs.
var ExcludedDirs = []string{"mayor", ".beads"}
