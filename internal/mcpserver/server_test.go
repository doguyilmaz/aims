package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New("test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

// setup creates claude profiles personal (the existing login) and work.
func setup(t *testing.T) string {
	t.Helper()
	home := testutil.Sandbox(t)
	testutil.Fakes(t)
	testutil.Write(t, filepath.Join(home, ".claude", ".credentials.json"), `{"claudeAiOauth":{"subscriptionType":"pro"}}`)
	testutil.Write(t, filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"me@gmail.com"}}`)
	a := tools.Get("claude")
	if _, err := ops.AddProfile(a, "personal", ops.AddOptions{Existing: true}); err != nil {
		t.Fatal(err)
	}
	res, err := ops.AddProfile(a, "work", ops.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(res.Dir, ".credentials.json"), `{"claudeAiOauth":{"subscriptionType":"max"}}`)
	testutil.Write(t, filepath.Join(res.Dir, ".claude.json"), `{"oauthAccount":{"emailAddress":"work@example.com"}}`)
	return home
}

func TestTools(t *testing.T) {
	setup(t)
	cs := connect(t)
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
		if tl.InputSchema == nil {
			t.Errorf("%s has no input schema", tl.Name)
		}
	}
	want := "aims_status aims_switch aims_failover aims_clear aims_run aims_login_help"
	for _, n := range strings.Fields(want) {
		if !strings.Contains(strings.Join(names, " "), n) {
			t.Errorf("missing tool %s in %v", n, names)
		}
	}
}

func TestStatusAndSwitch(t *testing.T) {
	setup(t)
	t.Setenv("AIMS_SESSION_PROFILE", "claude:work")
	cs := connect(t)

	out, isErr := call(t, cs, "aims_status", map[string]any{"tool": "claude"})
	if isErr || !strings.Contains(out, "This session runs as claude:work") || !strings.Contains(out, "* personal: me@gmail.com (pro), ready") {
		t.Fatalf("status:\n%s", out)
	}

	out, isErr = call(t, cs, "aims_switch", map[string]any{"profile": "work"})
	if isErr || !strings.Contains(out, `Claude Code now use "work"`) {
		t.Fatalf("switch: %s", out)
	}
	out, _ = call(t, cs, "aims_status", nil)
	if !strings.Contains(out, "* work") {
		t.Fatalf("switch did not stick:\n%s", out)
	}

	out, isErr = call(t, cs, "aims_switch", map[string]any{"tool": "claude", "profile": "nobody"})
	if !isErr || !strings.Contains(out, `no claude profile "nobody"`) {
		t.Fatalf("unknown profile: %s", out)
	}
	if out, isErr = call(t, cs, "aims_status", map[string]any{"tool": "gemini"}); !isErr {
		t.Fatalf("unknown tool accepted: %s", out)
	}
}

func TestFailoverUsesSessionProfile(t *testing.T) {
	setup(t)
	t.Setenv("AIMS_SESSION_PROFILE", "claude:personal")
	cs := connect(t)
	out, isErr := call(t, cs, "aims_failover", map[string]any{"tool": "claude", "minutes": 45})
	if isErr || !strings.Contains(out, `"personal" is cooling down`) || !strings.Contains(out, `new sessions use "work" meanwhile`) || !strings.Contains(out, "aims claude --continue") {
		t.Fatalf("failover: %s", out)
	}
	out, _ = call(t, cs, "aims_clear", map[string]any{"tool": "claude"})
	if !strings.Contains(out, "personal") {
		t.Fatalf("clear: %s", out)
	}
}

func TestRunFailsOver(t *testing.T) {
	home := setup(t)
	testutil.Write(t, filepath.Join(home, ".claude", "LIMITED"), "")
	cs := connect(t)

	out, isErr := call(t, cs, "aims_run", map[string]any{"tool": "claude", "prompt": "-p looks like a flag"})
	if isErr || !strings.HasPrefix(out, "[claude/work exit 0]") || !strings.Contains(out, "answer from work: -p looks like a flag") {
		t.Fatalf("run:\n%s", out)
	}

	// Pinned to the limited profile: no failover, the error comes back.
	out, isErr = call(t, cs, "aims_run", map[string]any{"tool": "claude", "profile": "personal", "prompt": "hi"})
	if !isErr || !strings.Contains(out, "usage limit]") || !strings.Contains(out, "hit your session limit") {
		t.Fatalf("pinned run:\n%s", out)
	}

	if out, isErr = call(t, cs, "aims_run", map[string]any{"tool": "claude", "prompt": "  "}); !isErr {
		t.Fatalf("empty prompt accepted: %s", out)
	}
}

func TestLoginHelp(t *testing.T) {
	setup(t)
	cs := connect(t)
	out, _ := call(t, cs, "aims_login_help", map[string]any{"tool": "codex", "profile": "work"})
	if !strings.Contains(out, "aims login codex work") || !strings.Contains(out, "--device-auth") {
		t.Fatalf("login help: %s", out)
	}
}

func TestRunIsReadOnlyUnlessAsked(t *testing.T) {
	setup(t)
	log := filepath.Join(t.TempDir(), "log")
	t.Setenv("FAKE_SIDE_EFFECTS", log)
	t.Setenv("FAKE_RECORD_ARGS", "1")
	cs := connect(t)
	call(t, cs, "aims_run", map[string]any{"tool": "claude", "prompt": "look"})
	call(t, cs, "aims_run", map[string]any{"tool": "claude", "prompt": "edit", "write": true})
	got := testutil.Read(t, log)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "--permission-mode plan") || strings.Contains(lines[1], "--permission-mode") {
		t.Fatalf("runs:\n%s", got)
	}
}

func TestRunRefusesToNest(t *testing.T) {
	setup(t)
	t.Setenv(profiles.NestedVar, "1")
	cs := connect(t)
	out, isErr := call(t, cs, "aims_run", map[string]any{"tool": "claude", "prompt": "again"})
	if !isErr || !strings.Contains(out, "cannot start another") {
		t.Fatalf("nested run: %s", out)
	}
}
