//go:build windows

package cmd

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/tmux"
)

func freezeSessionGroup(_ *tmux.Tmux, _ string) error {
	return fmt.Errorf("freezing tmux session groups is not supported on Windows")
}

func thawSessionGroup(_ *tmux.Tmux, _ string) error {
	return fmt.Errorf("resuming tmux session groups is not supported on Windows")
}
