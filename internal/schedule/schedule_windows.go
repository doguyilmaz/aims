package schedule

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/proc"
)

func task(j Job) string { return "aims " + j.Name }

func install(j Job) (string, error) {
	quoted := make([]string, len(j.Command))
	for i, a := range j.Command {
		quoted[i] = `"` + a + `"`
	}
	mins := max(1, int(j.Every.Minutes()))
	if testing.Testing() {
		return task(j), nil
	}
	return task(j), schtasks("/Create", "/F", "/SC", "MINUTE", "/MO", fmt.Sprint(mins), "/TN", task(j), "/TR", strings.Join(quoted, " "))
}

func uninstall(j Job) (string, bool, error) {
	if _, ok := installed(j); !ok || testing.Testing() {
		return task(j), false, nil
	}
	return task(j), true, schtasks("/Delete", "/F", "/TN", task(j))
}

func installed(j Job) (string, bool) {
	if testing.Testing() {
		return task(j), false
	}
	_, code, err := proc.Output("schtasks", []string{"/Query", "/TN", task(j)}, nil, defaultTimeout)
	return task(j), err == nil && code == 0
}

func schtasks(args ...string) error {
	out, code, err := proc.CombinedOutput("schtasks", args, nil, defaultTimeout)
	if err == nil && code != 0 {
		err = fmt.Errorf("schtasks: %s", strings.TrimSpace(out))
	}
	return err
}

func heal(Job) (bool, error) { return false, nil }
