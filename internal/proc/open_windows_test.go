package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsHoldingDistinguishesOpenAndClosedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.sqlite")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if held, known := Holding(dir); !held || !known {
		f.Close()
		t.Fatalf("open file not detected: %v %v", held, known)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if held, known := Holding(dir); held || !known {
		t.Fatalf("closed folder cannot reconcile: %v %v", held, known)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err = os.Create(filepath.Join(dir, "nested", "rollout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if held, known := HoldingTree(dir); !held || !known {
		t.Fatalf("nested writer not detected: %v %v", held, known)
	}
}
