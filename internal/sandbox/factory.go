package sandbox

import (
	"fmt"
	"sync"
)

// backendRegistry holds singleton backend instances.
var (
	localBackend   *LocalBackend
	daytonaBackend *DaytonaBackend
	backendMu      sync.RWMutex
)

// GetBackend returns the appropriate backend for a given configuration.
// Backends are singletons - only one instance of each type is created.
func GetBackend(config *Config) (Backend, error) {
	if config == nil {
		config = DefaultConfig()
	}

	switch config.Backend {
	case BackendLocal, "":
		return GetLocalBackend(config.Local), nil
	case BackendDaytona:
		return GetDaytonaBackend(config.Daytona)
	default:
		return nil, fmt.Errorf("unknown backend type: %s", config.Backend)
	}
}

// GetBackendForRole returns the backend to use for a specific agent role.
// This allows different roles to use different backends.
func GetBackendForRole(config *Config, role string) (Backend, error) {
	if config == nil {
		config = DefaultConfig()
	}

	backendType := config.GetBackendForRole(role)

	switch backendType {
	case BackendLocal, "":
		return GetLocalBackend(config.Local), nil
	case BackendDaytona:
		return GetDaytonaBackend(config.Daytona)
	default:
		return nil, fmt.Errorf("unknown backend type: %s", backendType)
	}
}

// GetLocalBackend returns the singleton local backend.
func GetLocalBackend(config *LocalConfig) *LocalBackend {
	backendMu.Lock()
	defer backendMu.Unlock()

	if localBackend == nil {
		localBackend = NewLocalBackend(config)
	}
	return localBackend
}

// GetDaytonaBackend returns the singleton Daytona backend.
func GetDaytonaBackend(config *DaytonaConfig) (*DaytonaBackend, error) {
	backendMu.Lock()
	defer backendMu.Unlock()

	if daytonaBackend == nil {
		if config == nil {
			config = DefaultDaytonaConfig()
		}
		daytonaBackend = NewDaytonaBackend(config)
	}

	// Verify Daytona is available
	if !daytonaBackend.IsAvailable() {
		return nil, fmt.Errorf("Daytona is not available - ensure daytona CLI is installed and %s is set",
			config.APIKeyEnv)
	}

	return daytonaBackend, nil
}

// ResetBackends clears the singleton backend instances.
// This is primarily useful for testing.
func ResetBackends() {
	backendMu.Lock()
	defer backendMu.Unlock()
	localBackend = nil
	daytonaBackend = nil
}

// MustGetBackend is like GetBackend but panics on error.
// Use only when backend availability has already been validated.
func MustGetBackend(config *Config) Backend {
	backend, err := GetBackend(config)
	if err != nil {
		panic(fmt.Sprintf("failed to get backend: %v", err))
	}
	return backend
}

// IsLocalBackend checks if a backend is the local backend.
func IsLocalBackend(b Backend) bool {
	_, ok := b.(*LocalBackend)
	return ok
}

// IsDaytonaBackend checks if a backend is the Daytona backend.
func IsDaytonaBackend(b Backend) bool {
	_, ok := b.(*DaytonaBackend)
	return ok
}

// AsLocalBackend safely casts a backend to LocalBackend.
// Returns nil if the backend is not a local backend.
func AsLocalBackend(b Backend) *LocalBackend {
	if local, ok := b.(*LocalBackend); ok {
		return local
	}
	return nil
}

// AsDaytonaBackend safely casts a backend to DaytonaBackend.
// Returns nil if the backend is not a Daytona backend.
func AsDaytonaBackend(b Backend) *DaytonaBackend {
	if daytona, ok := b.(*DaytonaBackend); ok {
		return daytona
	}
	return nil
}
