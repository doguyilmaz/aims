package tool

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

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
