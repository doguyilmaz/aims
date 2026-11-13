package launch

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestSweepPreservesLiveAndUnverifiableSessionRoots(t *testing.T) {
	runs := t.TempDir()
	old := time.Now().Add(-31 * 24 * time.Hour)
	for _, kind := range []string{"live", "legacy", "malformed"} {
		root := filepath.Join(runs, "run-"+kind)
		testutil.Write(t, filepath.Join(root, "1", "coordination"), "keep this session")
		switch kind {
		case "live":
			testutil.Write(t, filepath.Join(root, "owner.lock"), strconv.Itoa(os.Getpid()))
		case "malformed":
			testutil.Write(t, filepath.Join(root, "owner.lock"), "unknown owner")
		}
		if fi := fsx.Stat(filepath.Join(root, "owner.lock")); fi != nil {
			if err := os.Chtimes(filepath.Join(root, "owner.lock"), old, old); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(root, old, old); err != nil {
			t.Fatal(err)
		}
	}
	sweep(runs)
	for _, kind := range []string{"live", "legacy", "malformed"} {
		if _, err := os.Stat(filepath.Join(runs, "run-"+kind, "1", "coordination")); err != nil {
			t.Fatalf("%s session coordination was deleted: %v", kind, err)
		}
	}
}
