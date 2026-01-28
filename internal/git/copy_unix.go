//go:build !windows

package git

import (
	"os/exec"
)

// copyDirPreserving copies a directory using cp -a, which preserves symlinks,
// permissions, timestamps, and all file attributes.
func copyDirPreserving(src, dest string) error {
	cmd := exec.Command("cp", "-a", src, dest)
	return cmd.Run()
}
