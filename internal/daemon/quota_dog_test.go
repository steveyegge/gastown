package daemon

import (
	"encoding/json"
	"testing"
	"time"
)

func TestQuotaDogInterval(t *testing.T) {
	// Default interval
	if got := quotaDogInterval(nil); got != defaultQuotaDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultQuotaDogInterval, got)
	}

	// Custom interval
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			QuotaDog: &QuotaDogConfig{
				Enabled:     true,
				IntervalStr: "2m",
			},
		},
	}
	if got := quotaDogInterval(config); got != 2*time.Minute {
		t.Errorf("expected 2m interval, got %v", got)
	}

	// Invalid interval falls back to default
	config.Patrols.QuotaDog.IntervalStr = "invalid"
	if got := quotaDogInterval(config); got != defaultQuotaDogInterval {
		t.Errorf("expected default interval for invalid config, got %v", got)
	}
}

func TestIsPatrolEnabled_QuotaDog(t *testing.T) {
	// Nil config: disabled (opt-in patrol)
	if IsPatrolEnabled(nil, "quota_dog") {
		t.Error("expected quota_dog to be disabled with nil config")
	}

	// Empty patrols: disabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	if IsPatrolEnabled(config, "quota_dog") {
		t.Error("expected quota_dog to be disabled by default")
	}

	// Explicitly enabled
	config.Patrols.QuotaDog = &QuotaDogConfig{Enabled: true}
	if !IsPatrolEnabled(config, "quota_dog") {
		t.Error("expected quota_dog to be enabled when configured")
	}

	// Explicitly disabled
	config.Patrols.QuotaDog = &QuotaDogConfig{Enabled: false}
	if IsPatrolEnabled(config, "quota_dog") {
		t.Error("expected quota_dog to be disabled when explicitly disabled")
	}
}

func TestQuotaDogConfigJSON(t *testing.T) {
	jsonData := `{"enabled": true, "interval": "3m"}`

	var config QuotaDogConfig
	if err := json.Unmarshal([]byte(jsonData), &config); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !config.Enabled {
		t.Error("expected enabled=true")
	}
	if config.IntervalStr != "3m" {
		t.Errorf("expected interval=3m, got %s", config.IntervalStr)
	}
}

func TestQuotaDogDefaultConstants(t *testing.T) {
	if defaultQuotaDogInterval != 5*time.Minute {
		t.Errorf("expected default interval 5m, got %v", defaultQuotaDogInterval)
	}
	if quotaDogTimeout != 2*time.Minute {
		t.Errorf("expected timeout 2m, got %v", quotaDogTimeout)
	}
}
