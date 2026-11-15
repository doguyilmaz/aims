package fsx

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreatePrivateTemp is CreateTemp with private creation on every platform.
func CreatePrivateTemp(dir, pattern string) (*os.File, error) {
	if strings.ContainsAny(pattern, `/\`) {
		return nil, fmt.Errorf("temporary file pattern contains a path separator")
	}
	if dir == "" {
		dir = os.TempDir()
	}
	for range 100 {
		random := rand.Text()
		name := pattern + random
		if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
			name = pattern[:i] + random + pattern[i+1:]
		}
		f, err := CreatePrivateFile(filepath.Join(dir, name))
		if os.IsExist(err) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("cannot allocate a private temporary file")
}

// MkdirAllPrivate creates only missing directories; existing access controls
// belong to their owner. New Windows directories also protect future children.
func MkdirAllPrivate(path string) error {
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrExist}
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrNotExist}
	}
	if err := MkdirAllPrivate(parent); err != nil {
		return err
	}
	if err := MkdirPrivate(path); err != nil {
		if fi, e := os.Stat(path); os.IsExist(err) && e == nil && fi.IsDir() {
			return nil
		}
		return err
	}
	return nil
}
