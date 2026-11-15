package desktop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func binaryPlistXML(data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", "-")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Output()
}

// Install uses Homebrew for public releases, or copies an explicitly selected,
// signature-verified local bundle into the user's Applications directory.
func Install(ctx context.Context, from string, in io.Reader, out, stderr io.Writer) error {
	if from == "" {
		brew, err := exec.LookPath("brew")
		if err != nil {
			return fmt.Errorf("install Homebrew first, or build the source preview and run: aims app install --from 'dist/Aims Usage.app'")
		}
		args := []string{"install", "--cask"}
		// This entry point already has a CLI. Retain its original installer.
		if Inspect().CLIPath != "" {
			args = append(args, "--skip-cask-deps")
		}
		args = append(args, Cask)
		cmd := exec.CommandContext(ctx, brew, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("app installation did not complete: %w; until the public cask is available, build the source preview and use aims app install --from 'dist/Aims Usage.app'", err)
		}
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return installBundle(ctx, from, filepath.Join(home, "Applications", "Aims Usage.app"))
}

func installBundle(ctx context.Context, from, destination string) error {
	from, err := filepath.Abs(from)
	if err != nil {
		return err
	}
	from, err = filepath.EvalSymlinks(from)
	if err != nil {
		return err
	}
	if _, err := readBundle(from); err != nil {
		return err
	}
	if filepath.Clean(from) == filepath.Clean(destination) {
		return verifyBundle(ctx, from)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("kept existing app at %s; use its updater or move it explicitly before installing another copy", destination)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".aims-usage-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	bundle := filepath.Join(stage, "Aims Usage.app")
	if err := exec.CommandContext(ctx, "/usr/bin/ditto", from, bundle).Run(); err != nil {
		return err
	}
	if _, err := readBundle(bundle); err != nil {
		return err
	}
	if err := verifyBundle(ctx, bundle); err != nil {
		return err
	}
	// RENAME_EXCL also protects an app created after our preflight.
	return unix.RenamexNp(bundle, destination, unix.RENAME_EXCL)
}

func verifyBundle(ctx context.Context, bundle string) error {
	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(verifyCtx, "/usr/bin/codesign", "--verify", "--deep", "--strict", bundle).Run(); err != nil {
		return fmt.Errorf("app signature verification failed; nothing installed: %w", err)
	}
	return nil
}

func Open(ctx context.Context) error {
	s := Inspect()
	if !s.Installed {
		return fmt.Errorf("Aims Usage is not installed; run aims app install")
	}
	return exec.CommandContext(ctx, "/usr/bin/open", s.Path).Run()
}

func LinkCLI(bundle string) (string, error) {
	s := Inspect()
	if s.CLIPath != "" {
		return s.CLIPath, nil
	}
	if bundle == "" {
		bundle = s.Path
	}
	if bundle == "" {
		return "", fmt.Errorf("install Aims Usage first")
	}
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if filepath.Dir(bundle) != filepath.Join(home, "Applications") && filepath.Dir(bundle) != "/Applications" {
		return "", fmt.Errorf("move Aims Usage.app to Applications before adding its terminal command")
	}
	if err := verifyBundle(context.Background(), bundle); err != nil {
		return "", err
	}
	return linkCLI(bundle, filepath.Dir(s.LinkPath))
}
