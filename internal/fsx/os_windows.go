//go:build windows

package fsx

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// busy reports a sharing violation or a file pending deletion, which Windows
// reports as "access denied" and which clears within milliseconds.
func busy(err error) bool { return errors.Is(err, fs.ErrPermission) }

// rename retries while another process briefly holds the target open (a
// reader, an indexer, a virus scanner): Windows cannot replace an open file.
func rename(from, to string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(from, to); err == nil || !busy(err) {
			return err
		}
		time.Sleep(10 * time.Millisecond * time.Duration(i+1))
	}
	return err
}
