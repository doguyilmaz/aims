package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	return holdingDirectory(dir, false)
}

func HoldingTree(dir string) (held, ok bool) {
	return holdingDirectory(dir, true)
}

func holdingDirectory(dir string, recursive bool) (held, ok bool) {
	return retryInspection(func() (bool, bool) { return inspectDirectory(dir, recursive) })
}

// A process can exit while /proc is being inspected. Retry an incomplete
// snapshot briefly; a persistently opaque process still leaves it unknown.
func retryInspection(inspect func() (held, known bool)) (bool, bool) {
	for attempt := range 3 {
		held, known := inspect()
		if held || known {
			return held, known
		}
		if attempt < 2 {
			time.Sleep(25 * time.Millisecond)
		}
	}
	return false, false
}

func inspectDirectory(dir string, recursive bool) (held, ok bool) {
	// With hidepid, other processes are out of sight.
	if mounts, err := os.ReadFile("/proc/self/mounts"); err != nil || strings.Contains(string(mounts), "hidepid=") {
		return false, false
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false, false
	}
	for _, process := range processes {
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		root := filepath.Join("/proc", process.Name())
		info, err := os.Stat(root)
		if os.IsNotExist(err) { // the process exited
			continue
		}
		if err != nil {
			return false, false
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return false, false
		}
		if stat.Uid != uint32(os.Geteuid()) {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(root, "fd"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil { // e.g. a non-dumpable process owned by this user
			return false, false
		}
		for _, fd := range fds {
			path, err := os.Readlink(filepath.Join(root, "fd", fd.Name()))
			if err != nil && !os.IsNotExist(err) {
				return false, false
			}
			if err == nil && (filepath.Dir(path) == dir || recursive && strings.HasPrefix(path, dir+string(filepath.Separator))) {
				return true, true
			}
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
