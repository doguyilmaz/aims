package links

import "golang.org/x/sys/windows"

func renameNoReplace(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// os.Rename uses MOVEFILE_REPLACE_EXISTING, so use the native operation
	// without that flag to preserve a concurrently created destination.
	return windows.MoveFileEx(source, target, 0)
}
