package upgrade

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    *Version
		wantErr bool
	}{
		{"v0.2.6", &Version{Major: 0, Minor: 2, Patch: 6, Raw: "v0.2.6"}, false},
		{"0.2.6", &Version{Major: 0, Minor: 2, Patch: 6, Raw: "0.2.6"}, false},
		{"v1.0.0", &Version{Major: 1, Minor: 0, Patch: 0, Raw: "v1.0.0"}, false},
		{"v1.0.0-alpha", &Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "alpha", Raw: "v1.0.0-alpha"}, false},
		{"v1.0.0-beta.1", &Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta.1", Raw: "v1.0.0-beta.1"}, false},
		{"v10.20.30", &Version{Major: 10, Minor: 20, Patch: 30, Raw: "v10.20.30"}, false},
		{"invalid", nil, true},
		{"v1.2", nil, true},
		{"v1", nil, true},
		{"", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch {
				t.Errorf("ParseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if got.Prerelease != tt.want.Prerelease {
				t.Errorf("ParseVersion(%q).Prerelease = %q, want %q", tt.input, got.Prerelease, tt.want.Prerelease)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.2.6", "v0.2.6", 0},
		{"v0.2.6", "v0.2.7", -1},
		{"v0.2.7", "v0.2.6", 1},
		{"v0.3.0", "v0.2.9", 1},
		{"v1.0.0", "v0.9.9", 1},
		{"v1.0.0", "v1.0.0-alpha", 1},  // release > prerelease
		{"v1.0.0-alpha", "v1.0.0", -1}, // prerelease < release
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-beta", "v1.0.0-alpha", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			got := a.Compare(b)
			if got != tt.want {
				t.Errorf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionLessThan(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v0.2.5", "v0.2.6", true},
		{"v0.2.6", "v0.2.6", false},
		{"v0.2.7", "v0.2.6", false},
		{"v0.2.6", "v0.3.0", true},
		{"v0.2.6", "v1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_lt_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			got := a.LessThan(b)
			if got != tt.want {
				t.Errorf("%s.LessThan(%s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionIsMajorUpgrade(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{"v0.2.6", "v0.2.7", false},
		{"v0.2.6", "v0.3.0", false},
		{"v0.2.6", "v1.0.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"v1.9.9", "v2.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			from, _ := ParseVersion(tt.from)
			to, _ := ParseVersion(tt.to)
			got := from.IsMajorUpgrade(to)
			if got != tt.want {
				t.Errorf("%s.IsMajorUpgrade(%s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestVersionMatchesPattern(t *testing.T) {
	tests := []struct {
		version string
		pattern string
		want    bool
	}{
		{"v0.2.6", "0.2.x", true},
		{"v0.2.6", "0.2.*", true},
		{"v0.2.6", "0.x", true},
		{"v0.2.6", "0.3.x", false},
		{"v0.2.6", "1.x", false},
		{"v0.2.6", "0.2.6", true},
		{"v0.2.6", "0.2.7", false},
		{"v1.0.0", "1.x", true},
		{"v1.0.0", "1.0.x", true},
	}

	for _, tt := range tests {
		t.Run(tt.version+"_matches_"+tt.pattern, func(t *testing.T) {
			v, _ := ParseVersion(tt.version)
			got := v.MatchesPattern(tt.pattern)
			if got != tt.want {
				t.Errorf("%s.MatchesPattern(%q) = %v, want %v", tt.version, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		input     string
		wantStr   string
		wantStrV  string
	}{
		{"v0.2.6", "0.2.6", "v0.2.6"},
		{"0.2.6", "0.2.6", "v0.2.6"},
		{"v1.0.0-alpha", "1.0.0-alpha", "v1.0.0-alpha"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, _ := ParseVersion(tt.input)
			if got := v.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
			if got := v.StringWithV(); got != tt.wantStrV {
				t.Errorf("StringWithV() = %q, want %q", got, tt.wantStrV)
			}
		})
	}
}
