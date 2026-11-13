//go:build !windows

package bridge

func deferLauncherUninstall(string, *Launchers) (bool, error) { return false, nil }
