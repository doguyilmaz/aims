// Package desktop connects the optional macOS companion to the terminal CLI.
// It never reads or changes provider profiles, logins or usage data.
package desktop

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const BundleID = "com.doguyilmaz.aims.usage"
const Cask = "doguyilmaz/tap/aims-usage"

func Supported() bool { return runtime.GOOS == "darwin" }

type Status struct {
	SchemaVersion string `json:"schemaVersion"`
	Installed     bool   `json:"installed"`
	Path          string `json:"path,omitempty"`
	Version       string `json:"version,omitempty"`
	CLIPath       string `json:"cliPath,omitempty"`
	LinkPath      string `json:"linkPath"`
}

func Inspect() Status {
	home, _ := os.UserHomeDir()
	s := Status{SchemaVersion: "1.0.0", LinkPath: filepath.Join(home, ".local", "bin", "aims")}
	if path, err := exec.LookPath("aims"); err == nil {
		s.CLIPath = path
	}
	if !Supported() {
		return s
	}
	paths := []string{filepath.Join(home, "Applications", "Aims Usage.app"), "/Applications/Aims Usage.app"}
	// A bundled CLI also recognizes its own app, including a source preview.
	if self, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(self); err == nil {
			paths = append(paths, filepath.Dir(filepath.Dir(filepath.Dir(real))))
		}
	}
	for _, path := range paths {
		if version, err := readBundle(path); err == nil {
			s.Installed, s.Path, s.Version = true, path, version
			break
		}
	}
	return s
}

func readBundle(path string) (string, error) {
	f, err := os.Open(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return "", err
	}
	if len(data) > 65536 {
		return "", fmt.Errorf("app metadata is too large")
	}
	if bytes.HasPrefix(data, []byte("bplist")) {
		data, err = binaryPlistXML(data)
		if err != nil {
			return "", err
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	values := map[string]string{}
	var key string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if start, ok := token.(xml.StartElement); ok {
			switch start.Name.Local {
			case "key":
				if err := decoder.DecodeElement(&key, &start); err != nil {
					return "", err
				}
			case "string":
				var value string
				if err := decoder.DecodeElement(&value, &start); err != nil {
					return "", err
				}
				values[key] = value
				key = ""
			}
		}
	}
	if values["CFBundleIdentifier"] != BundleID || values["CFBundleExecutable"] != "AimsUsage" {
		return "", fmt.Errorf("not an Aims Usage release/development app: %s", path)
	}
	for _, rel := range []string{"MacOS/AimsUsage", "Resources/aims"} {
		info, err := os.Stat(filepath.Join(path, "Contents", rel))
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", fmt.Errorf("app executable is missing or not executable: %s", rel)
		}
	}
	return values["CFBundleShortVersionString"], nil
}

// linkCLI exclusively creates a per-user command. Existing files, unrelated
// links and package-manager installations are never replaced.
func linkCLI(bundle, directory string) (string, error) {
	if _, err := readBundle(bundle); err != nil {
		return "", err
	}
	target := filepath.Join(bundle, "Contents", "Resources", "aims")
	path := filepath.Join(directory, "aims")
	if old, err := os.Readlink(path); err == nil && old == target {
		return path, nil
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	if err := os.Symlink(target, path); err != nil {
		return "", fmt.Errorf("kept existing terminal command at %s: %w", path, err)
	}
	return path, nil
}
