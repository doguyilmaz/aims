package codex

import (
	"strings"
	"testing"
)

func TestPlainSecretsInConfigToml(t *testing.T) {
	got := plainSecrets(`model = "gpt-5"
[mcp_servers.jira]
command = "jira-mcp"
env = { JIRA_TOKEN = "abc123", JIRA_URL = "https://x" }
bearer_token = "sk-live"

[mcp_servers.jira.env]
API_KEY = "literal"
OTHER = "fine"
SAFE_TOKEN = "${SAFE_TOKEN}"

[mcp_servers."my.srv".http_headers]
Authorization = "Bearer xyz"

[profiles.work]
api_key = "not an mcp server"
`)
	want := []string{"jira has env JIRA_TOKEN", "jira has bearer_token", "jira has env API_KEY", "my.srv has http_headers Authorization"}
	if len(got) != len(want) {
		t.Fatalf("got %d findings:\n%s", len(got), strings.Join(got, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("finding %d: %q, want %q", i, got[i], w)
		}
	}
}
