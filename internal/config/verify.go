package config

import (
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/verify"
)

// ToVerifyGates converts merge_queue gate configuration into executable gates.
func (c *MergeQueueConfig) ToVerifyGates() []verify.Gate {
	if c == nil || len(c.Gates) == 0 {
		return nil
	}
	gates := make([]verify.Gate, 0, len(c.Gates))
	for name, gate := range c.Gates {
		if gate == nil || strings.TrimSpace(gate.Cmd) == "" {
			continue
		}
		var timeout time.Duration
		if gate.Timeout != "" {
			if parsed, err := time.ParseDuration(gate.Timeout); err == nil {
				timeout = parsed
			}
		}
		phase := verify.Phase(strings.TrimSpace(gate.Phase))
		if phase == "" {
			phase = verify.PhasePreMerge
		}
		gates = append(gates, verify.Gate{
			Name:    name,
			Cmd:     strings.TrimSpace(gate.Cmd),
			Timeout: timeout,
			Phase:   phase,
		})
	}
	return gates
}
