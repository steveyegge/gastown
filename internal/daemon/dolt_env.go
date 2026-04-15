package daemon

import "os"

// environWithout returns a copy of os.Environ() with any entries for the
// given key removed. This prevents duplicate env vars when the caller needs
// to override an inherited value (e.g. DOLT_CLI_PASSWORD) with a specific
// value — in Go's exec.Cmd both the inherited and the override would be
// visible to the child process otherwise, with winner depending on the OS.
func environWithout(key string) []string {
	prefix := key + "="
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) < len(prefix) || e[:len(prefix)] != prefix {
			out = append(out, e)
		}
	}
	return out
}
