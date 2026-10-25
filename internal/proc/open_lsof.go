//go:build darwin || freebsd || openbsd || netbsd

package proc

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Writing lists the files pids have open for writing, by the pid holding
// them. ok is false when this system cannot tell.
func Writing(pids ...int) (map[int][]string, bool) {
	if len(pids) == 0 {
		return nil, false
	}
	bin, err := exec.LookPath("lsof")
	if err != nil {
		bin = "/usr/sbin/lsof"
	}
	list := make([]string, len(pids))
	for i, p := range pids {
		list[i] = strconv.Itoa(p)
	}
	// lsof exits 1 when a pid has gone; what it printed still counts.
	out, _, err := Output(bin, []string{"-w", "-n", "-P", "-p", strings.Join(list, ","), "-Fpan"}, nil, 2*time.Second)
	if err != nil {
		return nil, false
	}
	files := map[int][]string{}
	var pid int
	var mode string
	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		switch v := line[1:]; line[0] {
		case 'p':
			pid, _ = strconv.Atoi(v)
		case 'f':
			mode = ""
		case 'a':
			mode = v
		case 'n':
			if mode == "w" || mode == "u" {
				files[pid] = append(files[pid], v)
			}
		}
	}
	return files, true
}

// Children lists the processes pid started.
func Children(pid int) []int {
	out, _, err := Output("pgrep", []string{"-P", strconv.Itoa(pid)}, nil, 2*time.Second)
	if err != nil {
		return nil
	}
	var kids []int
	for f := range strings.FieldsSeq(out) {
		if k, err := strconv.Atoi(f); err == nil {
			kids = append(kids, k)
		}
	}
	return kids
}
