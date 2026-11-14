package meter

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Grok owns token refresh and persistence. A snapshot only binds an optional
// web quota read to the same single first-party login used by the native call.
type grokLogin struct {
	path, token string
	digest      [32]byte
}

func grokLoginSnapshot(now time.Time) grokLogin {
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" {
		return grokLogin{}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return grokLogin{}
	}
	dir := os.Getenv("GROK_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".grok")
	}
	path := os.Getenv("GROK_AUTH_PATH")
	if path == "" {
		path = filepath.Join(dir, "auth.json")
	}
	if !filepath.IsAbs(path) {
		return grokLogin{}
	}
	raw, err := readGrokLogin(path)
	if err != nil {
		return grokLogin{}
	}
	var entries map[string]struct {
		Key           string    `json:"key"`
		Issuer        string    `json:"oidc_issuer"`
		Mode          string    `json:"auth_mode"`
		Expires       time.Time `json:"expires_at"`
		PrincipalType string    `json:"principal_type"`
	}
	if json.Unmarshal(raw, &entries) != nil {
		return grokLogin{}
	}
	var token string
	for scope, entry := range entries {
		if !strings.HasPrefix(scope, "https://auth.x.ai::") || entry.Issuer != "https://auth.x.ai" {
			continue
		}
		if token != "" || entry.Key == "" || entry.PrincipalType == "team" ||
			(entry.Mode != "oidc" && entry.Mode != "external") || !entry.Expires.After(now) {
			return grokLogin{}
		}
		token = entry.Key
	}
	return grokLogin{path: path, token: token, digest: sha256.Sum256(raw)}
}

func readGrokLogin(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("Grok login unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if len(raw) > 1<<20 {
		return nil, errors.New("Grok login unavailable")
	}
	return raw, err
}

func (login grokLogin) unchanged() bool {
	if login.token == "" {
		return false
	}
	raw, err := readGrokLogin(login.path)
	return err == nil && sha256.Sum256(raw) == login.digest
}
