package ops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// A logout completion is independent of config/state replacement. The tiny
// private record is allocated and synced before the native command starts;
// successful logout overwrites one byte on the already open descriptor.
// It contains registration metadata only, never provider credentials.
type purgeJournal struct {
	file      *os.File
	path      string
	offset    int64
	completed bool
}

func journalHeader(id tool.ID, name, generation string) []byte {
	b, _ := json.Marshal(struct {
		Tool             tool.ID
		Name, Generation string
	}{id, name, generation})
	return append(b, '\n')
}

func openPurgeJournal(id tool.ID, name string, p *config.Profile) (*purgeJournal, error) {
	dir := filepath.Join(config.Home(), "recovery", "profile-removals")
	header := journalHeader(id, name, p.Generation)
	var f *os.File
	var err error
	if p.Purge == nil || p.Purge.Journal == "" {
		if err := fsx.MkdirAllPrivate(dir); err != nil {
			return nil, err
		}
		f, err = fsx.CreatePrivateTemp(dir, "removal-*")
		if err != nil {
			return nil, err
		}
		data := append(append([]byte{}, header...), '0')
		if _, err = f.Write(data); err == nil {
			err = f.Sync()
		}
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, err
		}
		return &purgeJournal{file: f, path: f.Name(), offset: int64(len(header))}, nil
	} else {
		leaf := p.Purge.Journal
		if filepath.Base(leaf) != leaf || !strings.HasPrefix(leaf, "removal-") || strings.ContainsAny(leaf, `/\`) {
			return nil, fmt.Errorf("invalid purge recovery record")
		}
		path := filepath.Join(dir, leaf)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() != int64(len(header)+1) {
			return nil, fmt.Errorf("invalid purge recovery record at %s", path)
		}
		f, err = os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		actual, err := f.Stat()
		if err != nil || !os.SameFile(info, actual) {
			f.Close()
			return nil, fmt.Errorf("purge recovery record changed")
		}
	}
	data := make([]byte, len(header)+1)
	if _, err := f.ReadAt(data, 0); err != nil && err != io.EOF {
		f.Close()
		return nil, err
	}
	if !bytes.Equal(data[:len(header)], header) || (data[len(header)] != '0' && data[len(header)] != '1') {
		f.Close()
		return nil, fmt.Errorf("purge recovery record belongs to another registration")
	}
	return &purgeJournal{file: f, path: f.Name(), offset: int64(len(header)), completed: data[len(header)] == '1'}, nil
}

func (j *purgeJournal) complete() error {
	if _, err := j.file.WriteAt([]byte{'1'}, j.offset); err != nil {
		return err
	}
	if err := j.file.Sync(); err != nil {
		return err
	}
	j.completed = true
	return nil
}

// Remove only the record we opened; keep externally replaced files intact.
func (j *purgeJournal) remove() {
	want, err := j.file.Stat()
	if err != nil {
		return
	}
	now, err := os.Lstat(j.path)
	if err == nil && now.Mode().IsRegular() && os.SameFile(want, now) {
		// Windows requires the open file to be closed before removing it.
		j.file.Close()
		_ = os.Remove(j.path)
	}
}
