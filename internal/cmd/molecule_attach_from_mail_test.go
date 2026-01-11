package cmd

import "testing"

func TestExtractMoleculeIDFromMail(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "attached_molecule field",
			body:     "Hello agent,\n\nattached_molecule: gt-abc123\n\nPlease work on this.",
			expected: "gt-abc123",
		},
		{
			name:     "molecule_id field",
			body:     "Work assignment:\nmolecule_id: mol-xyz789",
			expected: "mol-xyz789",
		},
		{
			name:     "molecule field",
			body:     "molecule: gt-task-42",
			expected: "gt-task-42",
		},
		{
			name:     "mol field",
			body:     "Quick task:\nmol: gt-quick\nDo this now.",
			expected: "gt-quick",
		},
		{
			name:     "no molecule field",
			body:     "This is just a regular message without any molecule.",
			expected: "",
		},
		{
			name:     "empty body",
			body:     "",
			expected: "",
		},
		{
			name:     "molecule with extra whitespace",
			body:     "attached_molecule:   gt-whitespace  \n\nmore text",
			expected: "gt-whitespace",
		},
		{
			name:     "multiple fields - first wins",
			body:     "attached_molecule: first\nmolecule: second",
			expected: "first",
		},
		{
			name:     "case insensitive line matching",
			body:     "Attached_Molecule: gt-case",
			expected: "gt-case",
		},
		{
			name:     "molecule in multiline context",
			body: `Subject: Work Assignment

This is your next task.

attached_molecule: gt-multiline

Please complete by EOD.

Thanks,
Mayor`,
			expected: "gt-multiline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMoleculeIDFromMail(tt.body)
			if result != tt.expected {
				t.Errorf("extractMoleculeIDFromMail(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}
