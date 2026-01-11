package connection

import (
	"testing"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Address
		wantErr bool
	}{
		{
			name:  "rig/polecat",
			input: "gastown/rictus",
			want:  &Address{Rig: "gastown", Polecat: "rictus"},
		},
		{
			name:  "rig/ broadcast",
			input: "gastown/",
			want:  &Address{Rig: "gastown"},
		},
		{
			name:  "machine:rig/polecat",
			input: "vm:gastown/rictus",
			want:  &Address{Machine: "vm", Rig: "gastown", Polecat: "rictus"},
		},
		{
			name:  "machine:rig/ broadcast",
			input: "vm:gastown/",
			want:  &Address{Machine: "vm", Rig: "gastown"},
		},
		{
			name:  "rig only (no slash)",
			input: "gastown",
			want:  &Address{Rig: "gastown"},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty machine",
			input:   ":gastown/rictus",
			wantErr: true,
		},
		{
			name:    "empty rig",
			input:   "vm:/rictus",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAddress(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseAddress(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Machine != tt.want.Machine {
				t.Errorf("Machine = %q, want %q", got.Machine, tt.want.Machine)
			}
			if got.Rig != tt.want.Rig {
				t.Errorf("Rig = %q, want %q", got.Rig, tt.want.Rig)
			}
			if got.Polecat != tt.want.Polecat {
				t.Errorf("Polecat = %q, want %q", got.Polecat, tt.want.Polecat)
			}
		})
	}
}

func TestAddressString(t *testing.T) {
	tests := []struct {
		addr *Address
		want string
	}{
		{
			addr: &Address{Rig: "gastown", Polecat: "rictus"},
			want: "gastown/rictus",
		},
		{
			addr: &Address{Rig: "gastown"},
			want: "gastown/",
		},
		{
			addr: &Address{Machine: "vm", Rig: "gastown", Polecat: "rictus"},
			want: "vm:gastown/rictus",
		},
		{
			addr: &Address{Machine: "vm", Rig: "gastown"},
			want: "vm:gastown/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.addr.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddressIsLocal(t *testing.T) {
	tests := []struct {
		addr *Address
		want bool
	}{
		{&Address{Rig: "gastown"}, true},
		{&Address{Machine: "", Rig: "gastown"}, true},
		{&Address{Machine: "local", Rig: "gastown"}, true},
		{&Address{Machine: "vm", Rig: "gastown"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := tt.addr.IsLocal(); got != tt.want {
				t.Errorf("IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddressIsBroadcast(t *testing.T) {
	tests := []struct {
		addr *Address
		want bool
	}{
		{&Address{Rig: "gastown"}, true},
		{&Address{Rig: "gastown", Polecat: ""}, true},
		{&Address{Rig: "gastown", Polecat: "rictus"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := tt.addr.IsBroadcast(); got != tt.want {
				t.Errorf("IsBroadcast() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddressEqual(t *testing.T) {
	tests := []struct {
		a, b *Address
		want bool
	}{
		{
			&Address{Rig: "gastown", Polecat: "rictus"},
			&Address{Rig: "gastown", Polecat: "rictus"},
			true,
		},
		{
			&Address{Machine: "", Rig: "gastown"},
			&Address{Machine: "local", Rig: "gastown"},
			true,
		},
		{
			&Address{Rig: "gastown", Polecat: "rictus"},
			&Address{Rig: "gastown", Polecat: "nux"},
			false,
		},
		{
			&Address{Rig: "gastown"},
			nil,
			false,
		},
	}

	for _, tt := range tests {
		name := "equal"
		if !tt.want {
			name = "not equal"
		}
		t.Run(name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
