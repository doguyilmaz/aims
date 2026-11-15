//go:build !windows

package proc

func attachedCommand(bin string, args, env []string) (string, []string, error) { return bin, args, nil }
