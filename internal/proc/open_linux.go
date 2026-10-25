package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Writing lists the files pids have open for writing, by the pid holding
// them. ok is false when this system cannot tell.
func Writing(pids ...int) (map[int][]string, bool) {
	if _, err := os.Stat("/proc/self/fdinfo"); err != nil {
		return nil, false
	}
	files := map[int][]string{}
	for _, pid := range pids {
		dir := "/proc/" + strconv.Itoa(pid)
		fds, _ := os.ReadDir(filepath.Join(dir, "fd"))
		for _, fd := range fds {
			info, err := os.ReadFile(filepath.Join(dir, "fdinfo", fd.Name()))
			if err != nil || !writable(string(info)) {
				continue
			}
			if path, err := os.Readlink(filepath.Join(dir, "fd", fd.Name())); err == nil && filepath.IsAbs(path) {
				files[pid] = append(files[pid], path)
			}
		}
	}
	return files, true
}

// writable reads the access mode from an fdinfo's octal flags.
func writable(fdinfo string) bool {
	for line := range strings.Lines(fdinfo) {
		if v, ok := strings.CutPrefix(line, "flags:"); ok {
			flags, err := strconv.ParseUint(strings.TrimSpace(v), 8, 64)
			return err == nil && flags&3 != 0
		}
	}
	return false
}

// Holding reports whether a process has a file in dir open. ok is false
// when this system cannot tell.
func Holding(dir string) (held, ok bool) {
	// With hidepid, other processes are out of sight.
	if mounts, err := os.ReadFile("/proc/self/mounts"); err != nil || strings.Contains(string(mounts), "hidepid=") {
		return false, false
	}
	fds, err := filepath.Glob("/proc/[0-9]*/fd/*")
	if err != nil || len(fds) == 0 {
		return false, false
	}
	for _, fd := range fds {
		if path, err := os.Readlink(fd); err == nil && filepath.Dir(path) == dir {
			return true, true
		}
	}
	return false, true
}

// Children lists the processes pid started.
func Children(pid int) []int {
	tasks, _ := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/task/*/children")
	var kids []int
	for _, t := range tasks {
		b, _ := os.ReadFile(t)
		for f := range strings.FieldsSeq(string(b)) {
			if k, err := strconv.Atoi(f); err == nil {
				kids = append(kids, k)
			}
		}
	}
	return kids
}
