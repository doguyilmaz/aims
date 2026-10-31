package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/doguyilmaz/aims/internal/tool"
)

var _ tool.Diagnoser = (*Adapter)(nil)

var (
	tableRe  = regexp.MustCompile(`^\s*\[\s*mcp_servers\.("[^"]+"|[A-Za-z0-9_-]+)(?:\.(env|http_headers))?\s*\]`)
	otherRe  = regexp.MustCompile(`^\s*\[`)
	pairRe   = regexp.MustCompile(`("[^"]+"|[A-Za-z0-9_-]+)\s*=\s*"((?:[^"\\]|\\.)*)"`)
	inlineRe = regexp.MustCompile(`^\s*(env|http_headers)\s*=\s*\{(.*)\}`)
	bearerRe = regexp.MustCompile(`^\s*bearer_token\s*=\s*"((?:[^"\\]|\\.)*)"`)
)

// Diagnose names the MCP servers in config.toml, which every account
// shares, that hold a credential written out.
func (*Adapter) Diagnose(h tool.Home) []string {
	b, err := os.ReadFile(filepath.Join(h.Dir, "config.toml"))
	if err != nil {
		return nil
	}
	return plainSecrets(string(b))
}

func plainSecrets(toml string) []string {
	var out []string
	add := func(server, what string) {
		out = append(out, fmt.Sprintf("MCP server %s has %s written out in config.toml, which every account shares; name it in env_vars (or bearer_token_env_var) so the value stays in your environment", server, what))
	}
	server, field := "", ""
	for _, line := range strings.Split(toml, "\n") {
		if m := tableRe.FindStringSubmatch(line); m != nil {
			server, field = strings.Trim(m[1], `"`), m[2]
			continue
		}
		if otherRe.MatchString(line) {
			server, field = "", ""
			continue
		}
		if server == "" {
			continue
		}
		switch {
		case field != "":
			if m := pairRe.FindStringSubmatch(line); m != nil && strings.HasPrefix(strings.TrimSpace(line), m[1]) && tool.PlainSecret(m[1], m[2]) {
				add(server, field+" "+strings.Trim(m[1], `"`))
			}
		case bearerRe.MatchString(line):
			if m := bearerRe.FindStringSubmatch(line); strings.TrimSpace(m[1]) != "" {
				add(server, "bearer_token")
			}
		default:
			if m := inlineRe.FindStringSubmatch(line); m != nil {
				for _, p := range pairRe.FindAllStringSubmatch(m[2], -1) {
					if tool.PlainSecret(p[1], p[2]) {
						add(server, m[1]+" "+strings.Trim(p[1], `"`))
					}
				}
			}
		}
	}
	return out
}
