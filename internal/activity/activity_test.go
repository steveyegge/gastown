package activity

import (
	"testing"
	"time"
)

func TestCalculateActivity_Green(t *testing.T) {
	tests := []struct {
		name     string
		age      time.Duration
		wantAge  string
		wantColor string
	}{
		{"just now", 0, "<1m", ColorGreen},
		{"30 seconds", 30 * time.Second, "<1m", ColorGreen},
		{"1 minute", 1 * time.Minute, "1m", ColorGreen},
		{"1m30s", 90 * time.Second, "1m", ColorGreen},
		{"1m59s", 119 * time.Second, "1m", ColorGreen},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastActivity := time.Now().Add(-tt.age)
			info := Calculate(lastActivity)

			if info.FormattedAge != tt.wantAge {
				t.Errorf("FormattedAge = %q, want %q", info.FormattedAge, tt.wantAge)
			}
			if info.ColorClass != tt.wantColor {
				t.Errorf("ColorClass = %q, want %q", info.ColorClass, tt.wantColor)
			}
		})
	}
}

func TestCalculateActivity_Yellow(t *testing.T) {
	tests := []struct {
		name     string
		age      time.Duration
		wantAge  string
		wantColor string
	}{
		{"2 minutes", 2 * time.Minute, "2m", ColorYellow},
		{"3 minutes", 3 * time.Minute, "3m", ColorYellow},
		{"4 minutes", 4 * time.Minute, "4m", ColorYellow},
		{"4m59s", 299 * time.Second, "4m", ColorYellow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastActivity := time.Now().Add(-tt.age)
			info := Calculate(lastActivity)

			if info.FormattedAge != tt.wantAge {
				t.Errorf("FormattedAge = %q, want %q", info.FormattedAge, tt.wantAge)
			}
			if info.ColorClass != tt.wantColor {
				t.Errorf("ColorClass = %q, want %q", info.ColorClass, tt.wantColor)
			}
		})
	}
}

func TestCalculateActivity_Red(t *testing.T) {
	tests := []struct {
		name     string
		age      time.Duration
		wantAge  string
		wantColor string
	}{
		{"5 minutes", 5 * time.Minute, "5m", ColorRed},
		{"10 minutes", 10 * time.Minute, "10m", ColorRed},
		{"30 minutes", 30 * time.Minute, "30m", ColorRed},
		{"1 hour", 1 * time.Hour, "1h", ColorRed},
		{"2 hours", 2 * time.Hour, "2h", ColorRed},
		{"1 day", 24 * time.Hour, "1d", ColorRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastActivity := time.Now().Add(-tt.age)
			info := Calculate(lastActivity)

			if info.FormattedAge != tt.wantAge {
				t.Errorf("FormattedAge = %q, want %q", info.FormattedAge, tt.wantAge)
			}
			if info.ColorClass != tt.wantColor {
				t.Errorf("ColorClass = %q, want %q", info.ColorClass, tt.wantColor)
			}
		})
	}
}

func TestCalculateActivity_ZeroTime(t *testing.T) {
	// Zero time should return unknown state
	info := Calculate(time.Time{})

	if info.ColorClass != ColorUnknown {
		t.Errorf("ColorClass = %q, want %q for zero time", info.ColorClass, ColorUnknown)
	}
	if info.FormattedAge != "unknown" {
		t.Errorf("FormattedAge = %q, want %q for zero time", info.FormattedAge, "unknown")
	}
}

func TestCalculateActivity_FutureTime(t *testing.T) {
	// Future time (clock skew) should be treated as "just now"
	futureTime := time.Now().Add(5 * time.Second)
	info := Calculate(futureTime)

	if info.ColorClass != ColorGreen {
		t.Errorf("ColorClass = %q, want %q for future time", info.ColorClass, ColorGreen)
	}
}

func TestInfo_IsActive(t *testing.T) {
	tests := []struct {
		color    string
		isActive bool
	}{
		{ColorGreen, true},
		{ColorYellow, false},
		{ColorRed, false},
		{ColorUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			info := Info{ColorClass: tt.color}
			if info.IsActive() != tt.isActive {
				t.Errorf("IsActive() = %v, want %v for color %q", info.IsActive(), tt.isActive, tt.color)
			}
		})
	}
}

func TestInfo_IsStale(t *testing.T) {
	tests := []struct {
		color   string
		isStale bool
	}{
		{ColorGreen, false},
		{ColorYellow, true},
		{ColorRed, false},
		{ColorUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			info := Info{ColorClass: tt.color}
			if info.IsStale() != tt.isStale {
				t.Errorf("IsStale() = %v, want %v for color %q", info.IsStale(), tt.isStale, tt.color)
			}
		})
	}
}

func TestInfo_IsStuck(t *testing.T) {
	tests := []struct {
		color   string
		isStuck bool
	}{
		{ColorGreen, false},
		{ColorYellow, false},
		{ColorRed, true},
		{ColorUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			info := Info{ColorClass: tt.color}
			if info.IsStuck() != tt.isStuck {
				t.Errorf("IsStuck() = %v, want %v for color %q", info.IsStuck(), tt.isStuck, tt.color)
			}
		})
	}
}
