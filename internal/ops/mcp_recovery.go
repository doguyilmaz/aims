package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// backupMCP retains a private, content-addressed recovery copy of settings.
// A failed remove/add must not erase the previous MCP entry or unknown keys.
// Identical settings reuse one copy; credential files are never selected.
func backupMCP(a tool.Adapter, h tool.Home) (string, error) {
	l, ok := a.(tool.MCPConfigLocator)
	if !ok {
		return "", fmt.Errorf("%s cannot identify its MCP settings for recovery", a.ID())
	}
	path := l.MCPConfigPath(h)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	const maxSettings = int64(64 << 20)
	if !info.Mode().IsRegular() || info.Size() > maxSettings {
		return "", fmt.Errorf("MCP settings must be a regular file no larger than 64 MiB; kept them untouched")
	}
	in, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return "", fmt.Errorf("MCP settings are not a readable regular file")
	}
	root := filepath.Join(config.Home(), "recovery", "mcp")
	if fsx.Real(root) != filepath.Join(fsx.Real(config.Home()), "recovery", "mcp") {
		return "", fmt.Errorf("MCP recovery directory is externally managed; kept settings untouched")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	out, err := os.CreateTemp(root, ".settings-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(out.Name())
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(in, maxSettings+1))
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		return "", errors.Join(err, closeErr)
	}
	after, err := in.Stat()
	if err != nil || n > maxSettings || n != before.Size() || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		return "", fmt.Errorf("MCP settings changed while preserving them; retry after closing the tool")
	}
	location := sha256.Sum256([]byte(fsx.Abs(path)))
	dest := filepath.Join(root, string(a.ID())+"-"+hex.EncodeToString(location[:8])+"-"+hex.EncodeToString(hash.Sum(nil))+".bak")
	if err := os.Link(out.Name(), dest); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		// A matching filename alone does not prove this is our old copy.
		info, e := os.Lstat(dest)
		if e != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("MCP recovery destination is not a private regular file")
		}
		b, e := os.Open(dest)
		if e != nil {
			return "", e
		}
		existing := sha256.New()
		_, e = io.Copy(existing, b)
		b.Close()
		if e != nil || hex.EncodeToString(existing.Sum(nil)) != hex.EncodeToString(hash.Sum(nil)) {
			return "", fmt.Errorf("MCP recovery destination has been changed; kept settings untouched")
		}
	}
	return dest, nil
}

func updateMCP(ctx context.Context, a tool.Adapter, h tool.Home, server tool.MCPServer, replace bool) error {
	backup, err := backupMCP(a, h)
	if err != nil {
		return err
	}
	if replace {
		_, err = a.UnregisterMCP(ctx, h, server.Name)
	}
	if err == nil {
		err = a.RegisterMCP(ctx, h, server)
	}
	if err != nil && backup != "" {
		return fmt.Errorf("previous MCP settings kept at %s: %w", fsx.Tildify(backup), err)
	}
	return err
}
