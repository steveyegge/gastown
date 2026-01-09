package cmd

import "testing"

func TestParseBeadsVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    beadsVersion
		wantErr bool
	}{
		{"0.44.0", beadsVersion{0, 44, 0}, false},
		{"1.2.3", beadsVersion{1, 2, 3}, false},
		{"0.44.0-dev", beadsVersion{0, 44, 0}, false},
		{"v0.44.0", beadsVersion{0, 44, 0}, false},
		{"0.44", beadsVersion{0, 44, 0}, false},
		{"10.20.30", beadsVersion{10, 20, 30}, false},
		{"invalid", beadsVersion{}, true},
		{"", beadsVersion{}, true},
		{"a.b.c", beadsVersion{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseBeadsVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBeadsVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseBeadsVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBeadsVersionCompare(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"0.44.0", "0.44.0", 0},
		{"0.44.0", "0.43.0", 1},
		{"0.43.0", "0.44.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.44.1", "0.44.0", 1},
		{"0.44.0", "0.44.1", -1},
		{"1.2.3", "1.2.3", 0},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, err := parseBeadsVersion(tt.v1)
			if err != nil {
				t.Fatalf("failed to parse v1 %q: %v", tt.v1, err)
			}
			v2, err := parseBeadsVersion(tt.v2)
			if err != nil {
				t.Fatalf("failed to parse v2 %q: %v", tt.v2, err)
			}

			got := v1.compare(v2)
			if got != tt.want {
				t.Errorf("(%s).compare(%s) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}
