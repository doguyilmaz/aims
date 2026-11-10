//go:build windows

package proc

// Replace uses attached standard streams on Windows, which has no exec(2).
// RunAttached relays termination and waits for the native process to finish.
func Replace(bin string, args, env []string) (int, error) {
	return RunAttached(bin, args, env, "")
}
