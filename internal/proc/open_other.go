//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd

package proc

func HoldingTree(dir string) (bool, bool) { return Holding(dir) }

// Writing lists the files pids have open for writing, by the pid holding
// them. ok is false when this system cannot tell.
func Writing(...int) (map[int][]string, bool) { return nil, false }

// Holding reports whether a process has a file in dir open. ok is false
// when this system cannot tell.
func Holding(string) (held, ok bool) { return false, false }

// Children lists the processes pid started.
func Children(int) []int { return nil }
