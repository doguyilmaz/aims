package proc

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func envKeyEqual(a, b string) bool {
	return a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b)
}

func EnvironmentMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		if key, value, ok := strings.Cut(kv, "="); ok {
			if runtime.GOOS == "windows" {
				key = strings.ToUpper(key)
			}
			out[key] = value
		}
	}
	return out
}

// EnvValue follows the child's platform rules and last-value-wins behavior.
func EnvValue(env []string, key string) (string, bool) {
	var value string
	found := false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && envKeyEqual(k, key) {
			value, found = v, true
		}
	}
	return value, found
}

func PrependPath(env []string, dir string) []string {
	path, _ := EnvValue(env, "PATH")
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !envKeyEqual(key, "PATH") {
			out = append(out, kv)
		}
	}
	if path != "" {
		dir += string(os.PathListSeparator) + path
	}
	return append(out, "PATH="+dir)
}

// LookPathEnv resolves against the environment the child will actually get,
// without temporarily changing aims' process-wide PATH.
func LookPathEnv(bin string, env []string) (string, error) {
	if env == nil {
		env = os.Environ()
	}
	extensions := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(bin) == "" {
		ext, _ := EnvValue(env, "PATHEXT")
		if ext == "" {
			ext = ".COM;.EXE;.BAT;.CMD"
		}
		for _, s := range strings.Split(ext, ";") {
			if s != "" {
				extensions = append(extensions, s)
			}
		}
	}
	find := func(path string) (string, error) {
		path, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		for _, suffix := range extensions {
			candidate := path + suffix
			if info, err := os.Stat(candidate); err != nil || info.IsDir() {
				continue
			}
			if p, err := exec.LookPath(candidate); err == nil {
				return filepath.Abs(p)
			}
		}
		return "", &exec.Error{Name: bin, Err: exec.ErrNotFound}
	}
	if strings.ContainsAny(bin, `/\`) || filepath.IsAbs(bin) || filepath.VolumeName(bin) != "" {
		return find(bin)
	}
	path, _ := EnvValue(env, "PATH")
	for _, dir := range filepath.SplitList(path) {
		p, err := find(filepath.Join(dir, bin))
		if err != nil {
			continue
		}
		if !filepath.IsAbs(dir) {
			return "", &exec.Error{Name: bin, Err: exec.ErrDot}
		}
		return p, nil
	}
	return "", &exec.Error{Name: bin, Err: exec.ErrNotFound}
}
