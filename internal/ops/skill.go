package ops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/doguyilmaz/aims/internal/fsx"
)

// writeSkill preserves any previous text before replacing it. Externally
// managed symlinks belong to their manager (for example lockstep or dotfiles).
func managedSkill(path string) bool {
	// Real cannot resolve a dangling link. Its absence is not permission to
	// replace a path owned by a dotfiles/skill manager.
	if fi := fsx.Lstat(path); fi != nil && fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	expected := fsx.Abs(path)
	// HOME may itself sit below a platform alias such as macOS /var. That
	// does not make the skill externally managed; links below HOME do.
	if fsx.Within(path, fsx.Home()) {
		rel, _ := filepath.Rel(fsx.Abs(fsx.Home()), expected)
		expected = filepath.Join(fsx.Real(fsx.Home()), rel)
	}
	return fsx.Real(path) != expected
}

func writeSkill(path string, content []byte) error {
	if managedSkill(path) {
		return fmt.Errorf("skill is managed through a symlink; update it through its owner: %s", fsx.Tildify(path))
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && bytes.Equal(old, content) {
		return nil
	}
	if err == nil {
		// Keep recovery text outside the installed skill directory so clean
		// can remove that directory without deleting the user's previous text.
		backup, err := os.CreateTemp(filepath.Dir(filepath.Dir(path)), ".aims-skill-*.bak")
		if err != nil {
			return err
		}
		if _, err := backup.Write(old); err != nil {
			backup.Close()
			return err
		}
		if err := backup.Close(); err != nil {
			return err
		}
	}
	return fsx.WriteFile(path, content, 0o644)
}
