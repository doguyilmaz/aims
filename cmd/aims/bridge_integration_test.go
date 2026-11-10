package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/bridge"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Build the real entry point: testscript's own fake claude/codex executables
// cannot test argv[0] dispatch or replacing aims with a native process.
func TestBridgeProcesses(t *testing.T) {
	build := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	aimsBin, native := filepath.Join(build, "aims"+ext), filepath.Join(build, "native-provider"+ext)
	for _, target := range []struct{ out, source string }{{aimsBin, "."}, {native, "./testdata/bridge-native"}} {
		cmd := exec.Command("go", "build", "-o", target.out, target.source)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	home := testutil.Sandbox(t)
	t.Setenv(config.HomeVar, filepath.Join(home, "account config with spaces"))
	t.Setenv("AIMS_CODEX_BIN", native)
	t.Setenv("AIMS_CLAUDE_BIN", native)
	t.Setenv("AIMS_RUN", filepath.Join(home, "parent-run"))
	t.Setenv("CODEX_THREAD_ID", "parent-thread")
	t.Setenv("OPENAI_API_KEY", "shell-key-must-not-override-the-login")
	t.Setenv("CODEX_PATH", "/adapter/pinned/codex")
	t.Setenv("CLAUDE_CODE_EXECUTABLE", "/adapter/pinned/claude")
	for _, name := range []string{"codex-acp", "claude-agent-acp"} {
		if err := os.Link(native, filepath.Join(build, name+ext)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", build+string(os.PathListSeparator)+os.Getenv("PATH"))
	project := t.TempDir()
	cfg, err := config.Update(func(c *config.Config) error {
		for _, id := range []string{"claude", "codex"} {
			key := tool.ID(id)
			c.EnsureHub(key)
			c.Tools[key].Profiles["personal"] = &config.Profile{}
			c.Tools[key].Profiles["work"] = &config.Profile{}
			c.Tools[key].Active = "personal"
			c.Tools[key].Order = []string{"personal", "work"}
		}
		c.Tools["codex"].Projects = map[string][]string{config.ProjectOf(project): {"work"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	output := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(aimsBin, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("aims %v: %v\n%s", args, err, out)
		}
		return out
	}
	output("bridge", "install")
	publicCommands := output("bridge", "commands", "--json")
	if !json.Valid(publicCommands) || !bytes.Contains(publicCommands, []byte(`"schemaVersion": "1.0.0"`)) || bytes.Contains(publicCommands, []byte("shell-key-must-not-override")) {
		t.Fatal("invalid or unsafe integration command catalog")
	}
	var catalog struct {
		Agents []struct {
			ACP struct {
				Env map[string]string `json:"env"`
			} `json:"acp"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(publicCommands, &catalog); err != nil || len(catalog.Agents) != 2 {
		t.Fatal("missing agent catalog")
	}
	for _, agent := range catalog.Agents {
		if agent.ACP.Env[config.HomeVar] != config.Home() {
			t.Fatal("exported ACP command lost its custom configuration root")
		}
	}
	m, err := bridge.ReadLaunchers(bridge.LauncherRoot())
	if err != nil || m == nil {
		t.Fatalf("installation: %v", err)
	}
	codex := filepath.Join(bridge.LauncherBin(), m.Tools["codex"].Name)
	claude := filepath.Join(bridge.LauncherBin(), m.Tools["claude"].Name)

	t.Run("app-server-stream-and-lifecycle", func(t *testing.T) {
		args := []string{"app-server", "--config", `model="a model"`, "--client-option=literal"}
		p := startBridgeProbe(t, codex, args, "", nil)
		first := p.request(t)
		if first.Profile != "codex:personal" || first.CodexHome != cfg.Dir("codex", "personal") || first.HasOpenAIKey || first.Run != "" || first.ParentThread != "" {
			t.Fatalf("incorrect routing: %+v", first)
		}
		if !reflect.DeepEqual(first.Args, args) {
			t.Fatalf("arguments changed: %q", first.Args)
		}
		if runtime.GOOS != "windows" && first.PID != p.cmd.Process.Pid {
			t.Fatal("aims stayed as an unnecessary protocol proxy")
		}
		_, err := config.Update(func(c *config.Config) error { c.Tools["codex"].Active = "work"; return nil })
		if err != nil {
			t.Fatal(err)
		}
		if next := p.request(t); next.Profile != first.Profile || next.PID != first.PID {
			t.Fatal("active protocol process was moved to another account")
		}
		p.finish(t)
		fresh := startBridgeProbe(t, codex, nil, "", nil)
		if got := fresh.request(t); got.Profile != "codex:work" {
			t.Fatalf("new process ignored changed preference: %+v", got)
		}
		fresh.finish(t)
	})

	t.Run("project-order-and-explicit-account", func(t *testing.T) {
		if _, err := config.Update(func(c *config.Config) error { c.Tools["codex"].Active = "personal"; return nil }); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { config.Update(func(c *config.Config) error { c.Tools["codex"].Active = "work"; return nil }) })
		p := startBridgeProbe(t, codex, nil, project, nil)
		if got := p.request(t); got.Profile != "codex:work" {
			t.Fatalf("worker's project order lost: %+v", got)
		}
		p.finish(t)
		p = startBridgeProbe(t, aimsBin, []string{"bridge", "run", "codex@personal", "--", "app-server"}, project, nil)
		if got := p.request(t); got.Profile != "codex:personal" {
			t.Fatal("explicit account ignored")
		}
		p.finish(t)
	})

	t.Run("acp-keeps-bundled-runtime-selection", func(t *testing.T) {
		p := startBridgeProbe(t, aimsBin, []string{"bridge", "acp", "claude@work", "--", "--adapter-flag"}, "", nil)
		got := p.request(t)
		if got.Profile != "claude:work" || got.ClaudeExecutable != "/adapter/pinned/claude" || got.CodexPath != "/adapter/pinned/codex" || !reflect.DeepEqual(got.Args, []string{"--adapter-flag"}) {
			t.Fatalf("ACP runtime or arguments were replaced: %+v", got)
		}
		p.finish(t)
	})

	t.Run("gui-has-no-aims-home-or-native-cli-on-path", func(t *testing.T) {
		// The launcher remembers its config root. It can find the recorded
		// native executable when an IDE does not load the user's shell rc.
		env := replaceEnv(os.Environ(), map[string]string{"AIMS_HOME": "", "AIMS_CODEX_BIN": "", "AIMS_CLAUDE_BIN": "", "PATH": bridge.LauncherBin()})
		p := startBridgeProbe(t, codex, nil, "", env)
		if got := p.request(t); got.Home != config.Home() || got.Profile != "codex:work" {
			t.Fatalf("GUI environment lost routing: %+v", got)
		}
		p.finish(t)
		// Upstream ACP adapter paths are recorded too, without replacing
		// their runtime selection with the system's CLI version.
		env = replaceEnv(env, map[string]string{"AIMS_HOME": config.Home()})
		p = startBridgeProbe(t, aimsBin, []string{"bridge", "acp", "codex"}, "", env)
		if got := p.request(t); got.Profile != "codex:work" || got.CodexPath != "/adapter/pinned/codex" {
			t.Fatal("ACP failed with the GUI's minimal PATH")
		}
		p.finish(t)
	})

	t.Run("exec-launchers-change-only-the-child-path", func(t *testing.T) {
		parentPath := os.Getenv("PATH")
		env := replaceEnv(os.Environ(), map[string]string{"BRIDGE_NATIVE_MODE": "spawn"})
		p := startBridgeProbe(t, aimsBin, []string{"exec", "--launchers", "--", native, "app-server"}, "", env)
		if got := p.request(t); got.Profile != "codex:work" || os.Getenv("PATH") != parentPath {
			t.Fatal("child CLI did not use the launcher, or parent PATH was changed")
		}
		p.finish(t)
	})

	t.Run("stdio-is-byte-for-byte-and-stderr-is-separate", func(t *testing.T) {
		wire := []byte("{\"id\":1,\"text\":\"" + strings.Repeat("ö", 150000) + "\"}\n{\"id\":2}\n")
		cmd := exec.Command(claude, "-p", "--input-format", "stream-json", "--output-format", "stream-json")
		cmd.Env = replaceEnv(os.Environ(), map[string]string{"BRIDGE_NATIVE_MODE": "echo"})
		cmd.Stdin = bytes.NewReader(wire)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		got, err := cmd.Output()
		if err != nil || !bytes.Equal(got, wire) || stderr.String() != "native-stderr\n" {
			t.Fatalf("protocol modified: %v, stdout bytes %d/%d, stderr %q", err, len(got), len(wire), stderr.String())
		}
	})

	t.Run("version-goes-to-native-cli", func(t *testing.T) {
		cmd := exec.Command(codex, "--version")
		if out, err := cmd.Output(); err != nil || string(out) != "native-provider 1.2.3\n" {
			t.Fatalf("native flag consumed by aims: %q %v", out, err)
		}
	})

	t.Run("private-history-and-login-are-not-migrated", func(t *testing.T) {
		if _, err := config.Update(func(c *config.Config) error {
			c.Tools["codex"].Profiles["private"] = &config.Profile{Share: config.ShareSettings}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		cfg, _ := config.Load()
		dir := cfg.Dir("codex", "private")
		testutil.Write(t, filepath.Join(dir, "goals_1.sqlite"), "independent database")
		testutil.Write(t, filepath.Join(dir, "auth.json"), "private login sentinel")
		env := replaceEnv(os.Environ(), map[string]string{"CODEX_SQLITE_HOME": "/another-app/shared-state"})
		p := startBridgeProbe(t, aimsBin, []string{"bridge", "run", "codex@private", "--", "app-server"}, "", env)
		if got := p.request(t); got.Profile != "codex:private" || got.SQLiteHome != "" || got.CodexHome != dir {
			t.Fatalf("private profile was routed to shared state: %+v", got)
		}
		p.finish(t)
		if testutil.Read(t, filepath.Join(dir, "goals_1.sqlite")) != "independent database" || testutil.Read(t, filepath.Join(dir, "auth.json")) != "private login sentinel" {
			t.Fatal("protocol launch modified private data")
		}
	})

	t.Run("signal-and-exit-status", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("os.Process.Signal(os.Interrupt) is not supported on Windows")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, codex)
		cmd.Env = replaceEnv(os.Environ(), map[string]string{"BRIDGE_NATIVE_MODE": "signal"})
		stdout, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Process.Kill()
		if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
			t.Fatal("native process did not start")
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
		var ee *exec.ExitError
		if err := cmd.Wait(); !errors.As(err, &ee) || ee.ExitCode() != 23 {
			t.Fatalf("native signal/exit status was lost: %v", err)
		}
	})

	if _, err := os.Stat(filepath.Join(config.Home(), "state.json")); !os.IsNotExist(err) {
		t.Fatal("protocol launch probed accounts or wrote usage state")
	}
	output("bridge", "uninstall")
	if _, err := os.Stat(bridge.LauncherRoot()); !os.IsNotExist(err) {
		t.Fatal("owned launcher installation was not cleaned up")
	}
}

type probeResult struct {
	PID              int      `json:"pid"`
	Args             []string `json:"args"`
	Profile          string   `json:"profile"`
	Home             string   `json:"home"`
	CodexHome        string   `json:"codexHome"`
	SQLiteHome       string   `json:"sqliteHome"`
	Run              string   `json:"run"`
	ParentThread     string   `json:"parentThread"`
	HasOpenAIKey     bool     `json:"hasOpenAIKey"`
	CodexPath        string   `json:"codexPath"`
	ClaudeExecutable string   `json:"claudeExecutable"`
}

type bridgeProbe struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *json.Decoder
}

func startBridgeProbe(t *testing.T, bin string, args []string, dir string, env []string) *bridgeProbe {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env = dir, env
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { in.Close(); cancel(); cmd.Process.Kill() })
	return &bridgeProbe{cmd, in, json.NewDecoder(out)}
}

func (p *bridgeProbe) request(t *testing.T) probeResult {
	t.Helper()
	if _, err := io.WriteString(p.in, "{\"id\":1,\"method\":\"initialize\"}\n"); err != nil {
		t.Fatal(err)
	}
	var response struct {
		ID     int         `json:"id"`
		Result probeResult `json:"result"`
	}
	if err := p.out.Decode(&response); err != nil || response.ID != 1 {
		t.Fatalf("stream did not return a clean JSON-RPC response: %v", err)
	}
	return response.Result
}

func (p *bridgeProbe) finish(t *testing.T) {
	t.Helper()
	p.in.Close()
	if err := p.cmd.Wait(); err != nil {
		t.Fatalf("native process failed: %v", err)
	}
}

func replaceEnv(base []string, values map[string]string) []string {
	out := []string{}
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := values[k]; !ok {
			out = append(out, kv)
		}
	}
	for k, v := range values {
		if v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}
