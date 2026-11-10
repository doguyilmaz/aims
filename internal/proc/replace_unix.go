//go:build !windows

package proc

import "syscall"

// Replace gives a protocol client the native process itself: the same PID,
// standard streams, exit status and signals, with no resident aims supervisor.
func Replace(bin string, args, env []string) (int, error) {
	return 127, syscall.Exec(bin, append([]string{bin}, args...), env)
}
