package cmd

import (
	"os"
	"strings"
)

func isProductionGovernanceEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GT_GOVERNANCE_ENV"))) {
	case "prod", "production":
		return true
	default:
		return false
	}
}
