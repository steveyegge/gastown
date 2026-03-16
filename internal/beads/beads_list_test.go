package beads

import (
	"reflect"
	"testing"
)

func TestParseIDsFromBDListText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "no issues found message",
			input: "No issues found.\n",
			want:  nil,
		},
		{
			name:  "single issue",
			input: "○ hq-abc [● P2] Fix the thing\n",
			want:  []string{"hq-abc"},
		},
		{
			name: "two issues, one duplicate",
			input: "○ hq-abc [● P2] Fix the thing\n" +
				"○ hq-xyz [● P1] Another issue\n" +
				"○ hq-abc [● P2] Fix the thing (duplicate)\n",
			want: []string{"hq-abc", "hq-xyz"},
		},
		{
			name: "multiple distinct issues",
			input: "○ hq-wisp-itai [● P2] Some task\n" +
				"? hq-foo1 ● P1 Other task\n",
			want: []string{"hq-wisp-itai", "hq-foo1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIDsFromBDListText([]byte(tt.input))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseIDsFromBDListText() = %v, want %v", got, tt.want)
			}
		})
	}
}
