package tool

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ValidSessionID reports whether id looks like a conversation id, safe to
// pass as an argument: not empty, not an option.
func ValidSessionID(id string) bool { return sessionIDRe.MatchString(id) }

// EventsFile is where hooks record the turns of a supervised run, in its
// RunDir.
const EventsFile = "events"

// RecordEvent appends a turn's end to dir's events file.
func RecordEvent(dir string, ev SessionEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, EventsFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// ReadLines returns the complete lines of path written after offset, and
// the new offset. A line still being written is left for the next read.
func ReadLines(path string, offset int64) ([][]byte, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	b, _ := io.ReadAll(f)
	var out [][]byte
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			return out, offset
		}
		out = append(out, b[:i])
		b, offset = b[i+1:], offset+int64(i+1)
	}
}

// TailLines returns the whole lines in the last n bytes of path.
func TailLines(path string, n int64) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	// From one byte before the window, to tell whether it starts a line.
	start := max(fi.Size()-n-1, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	b, _ := io.ReadAll(f)
	if start > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			return nil
		}
		b = b[i+1:]
	}
	var out [][]byte
	for line := range bytes.Lines(b) {
		if l, ok := bytes.CutSuffix(line, []byte("\n")); ok {
			out = append(out, l)
		}
	}
	return out
}

// FollowEvents returns a poll over dir's events file, starting empty.
func FollowEvents(dir string) func() []SessionEvent {
	path := filepath.Join(dir, EventsFile)
	_ = os.Remove(path)
	var offset int64
	return func() []SessionEvent {
		var lines [][]byte
		lines, offset = ReadLines(path, offset)
		var out []SessionEvent
		for _, l := range lines {
			var ev SessionEvent
			if json.Unmarshal(l, &ev) == nil && ev.Session != "" {
				out = append(out, ev)
			}
		}
		return out
	}
}
