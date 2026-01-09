package cmd

import (
	"os"
	"testing"
)

func TestDeriveSessionName(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "polecat",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-gastown-toast",
		},
		{
			name: "crew session",
			envVars: map[string]string{
				"GT_ROLE": "crew",
				"GT_RIG":  "gastown",
				"GT_CREW": "max",
			},
			expected: "gt-gastown-crew-max",
		},
		{
			name: "witness session",
			envVars: map[string]string{
				"GT_ROLE": "witness",
				"GT_RIG":  "gastown",
			},
			expected: "gt-gastown-witness",
		},
		{
			name: "refinery session",
			envVars: map[string]string{
				"GT_ROLE": "refinery",
				"GT_RIG":  "gastown",
			},
			expected: "gt-gastown-refinery",
		},
		{
			name: "mayor session",
			envVars: map[string]string{
				"GT_ROLE": "mayor",
				"GT_TOWN": "ai",
			},
			expected: "gt-ai-mayor",
		},
		{
			name: "deacon session",
			envVars: map[string]string{
				"GT_ROLE": "deacon",
				"GT_TOWN": "ai",
			},
			expected: "gt-ai-deacon",
		},
		{
			name: "mayor session without GT_TOWN",
			envVars: map[string]string{
				"GT_ROLE": "mayor",
			},
			expected: "gt-mayor",
		},
		{
			name: "deacon session without GT_TOWN",
			envVars: map[string]string{
				"GT_ROLE": "deacon",
			},
			expected: "gt-deacon",
		},
		{
			name:     "no env vars",
			envVars:  map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear relevant env vars
			saved := make(map[string]string)
			envKeys := []string{"GT_ROLE", "GT_RIG", "GT_POLECAT", "GT_CREW", "GT_TOWN"}
			for _, key := range envKeys {
				saved[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			defer func() {
				// Restore env vars
				for key, val := range saved {
					if val != "" {
						os.Setenv(key, val)
					}
				}
			}()

			// Set test env vars
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			result := deriveSessionName()
			if result != tt.expected {
				t.Errorf("deriveSessionName() = %q, want %q", result, tt.expected)
			}
		})
	}
}
