// Package fake has stand-ins for the claude and codex CLIs. Like the real
// ones, they keep their login in the config directory they are pointed at,
// so tests can check which account a run used without any network.
//
// Files in that directory steer them:
//
//	LIMITED      runs fail with a usage-limit error
//	EXPIRED      runs fail with a login error
//	TOOL_FIRST   runs call a tool (appended to $FAKE_SIDE_EFFECTS) before failing
//	USAGE        "pct" for the rate limits the codex app-server reports
//
// $FAKE_SIDE_EFFECTS names a file that collects what they did, and with
// $FAKE_RECORD_ARGS set, the arguments of each headless claude run.
package fake

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func home(envVar, def string) (dir, label string) {
	if v := os.Getenv(envVar); v != "" {
		return v, filepath.Base(v)
	}
	h, _ := os.UserHomeDir()
	if v := os.Getenv("HOME"); v != "" {
		h = v
	}
	return filepath.Join(h, def), "default"
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func readJSON(p string) map[string]any {
	m := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func writeJSON(p string, v any) {
	b, _ := json.Marshal(v)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, b, 0o600)
}

func emit(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

// prompt is the argument after "--", or after -p.
func prompt(args []string) string {
	if i := slices.Index(args, "--"); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	if i := slices.Index(args, "-p"); i >= 0 && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
		return args[i+1]
	}
	return ""
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func sideEffect(label string) { record("tool ran as " + label) }

// record appends a line to $FAKE_SIDE_EFFECTS, for tests to check.
func record(line string) {
	if f := os.Getenv("FAKE_SIDE_EFFECTS"); f != "" {
		fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(fh, line)
			fh.Close()
		}
	}
}

func email(label string, args []string) string {
	if e := flag(args, "--email"); e != "" {
		return e
	}
	if e := os.Getenv("FAKE_EMAIL"); e != "" {
		return e
	}
	return label + "@example.com"
}

// Claude stands in for `claude`.
func Claude() int {
	args := os.Args[1:]
	dir, label := home("CLAUDE_CONFIG_DIR", ".claude")
	jsonFile := filepath.Join(dir, ".claude.json")
	if os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		jsonFile = filepath.Join(filepath.Dir(dir), ".claude.json")
	}
	creds := filepath.Join(dir, ".credentials.json")
	has := func(n string) bool { return exists(filepath.Join(dir, n)) }

	switch {
	case len(args) > 0 && args[0] == "--version":
		fmt.Println("2.1.0 (Claude Code)")
		return 0
	case len(args) > 1 && args[0] == "auth" && args[1] == "login":
		writeJSON(creds, map[string]any{"claudeAiOauth": map[string]any{"accessToken": "a", "refreshToken": "r", "subscriptionType": "max"}})
		j := readJSON(jsonFile)
		j["oauthAccount"] = map[string]any{"emailAddress": email(label, args)}
		writeJSON(jsonFile, j)
		fmt.Println("Login successful.")
		return 0
	case len(args) > 1 && args[0] == "auth" && args[1] == "logout":
		_ = os.Remove(creds)
		record("logout as " + label)
		fmt.Println("Successfully logged out.")
		return 0
	case len(args) > 1 && args[0] == "mcp":
		j := readJSON(jsonFile)
		servers, _ := j["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		rest := slices.DeleteFunc(slices.Clone(args[2:]), func(a string) bool { return a == "--scope" || a == "user" })
		name := rest[0]
		switch args[1] {
		case "add":
			if servers[name] != nil {
				fmt.Fprintf(os.Stderr, "MCP server %s already exists in user config\n", name)
				return 1
			}
			cmd := args[slices.Index(args, "--")+1:]
			servers[name] = map[string]any{"type": "stdio", "command": cmd[0], "args": cmd[1:]}
		case "remove":
			if servers[name] == nil {
				fmt.Fprintf(os.Stderr, "No MCP server named \"%s\" in user scope\n", name)
				return 1
			}
			delete(servers, name)
		}
		j["mcpServers"] = servers
		writeJSON(jsonFile, j)
		return 0
	}

	headless := slices.Contains(args, "-p") || slices.Contains(args, "--print")
	if !headless {
		fmt.Printf("claude interactive as %s args=%q session=%s api_key=%q\n", label, args, os.Getenv("AIMS_SESSION_PROFILE"), os.Getenv("ANTHROPIC_API_KEY"))
		if s := os.Getenv("FAKE_EXIT"); s != "" {
			var code int
			fmt.Sscan(s, &code)
			return code
		}
		return 0
	}
	if os.Getenv("FAKE_RECORD_ARGS") != "" {
		record("claude " + strings.Join(args, " "))
	}
	format := flag(args, "--output-format")
	if format == "" {
		format = "text"
	}
	if format == "stream-json" {
		emit(map[string]any{"type": "system", "subtype": "init"})
	}
	fail := ""
	switch {
	case !exists(creds) || has("EXPIRED"):
		fail = "Invalid API key · Please run /login"
	case has("LIMITED"):
		fail = "You've hit your session limit · resets 5pm"
	}
	if fail != "" && has("TOOL_FIRST") {
		if format == "stream-json" {
			emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "name": "Bash"}}}})
		}
		sideEffect(label)
	}
	text := "answer from " + label + ": " + prompt(args)
	if fail != "" {
		text = fail
	}
	switch format {
	case "text":
		fmt.Println(text)
	default:
		sub := "success"
		if fail != "" {
			sub = "error_during_execution"
		}
		emit(map[string]any{"type": "result", "subtype": sub, "is_error": fail != "", "result": text})
	}
	if fail != "" {
		return 1
	}
	return 0
}

// idToken builds an unsigned JWT carrying what aims reads from Codex logins.
func idToken(email, plan string) string {
	enc := base64.RawURLEncoding.EncodeToString
	payload, _ := json.Marshal(map[string]any{"email": email, "https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": plan}})
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

// Codex stands in for `codex`.
func Codex() int {
	args := os.Args[1:]
	dir, label := home("CODEX_HOME", ".codex")
	auth := filepath.Join(dir, "auth.json")
	toml := filepath.Join(dir, "config.toml")
	has := func(n string) bool { return exists(filepath.Join(dir, n)) }
	loggedIn := exists(auth)

	if len(args) == 0 {
		fmt.Printf("codex interactive as %s session=%s\n", label, os.Getenv("AIMS_SESSION_PROFILE"))
		return 0
	}
	switch args[0] {
	case "--version":
		fmt.Println("codex-cli 0.200.0")
		return 0
	case "login":
		if len(args) > 1 && args[1] == "status" {
			if loggedIn {
				fmt.Println("Logged in using ChatGPT")
				return 0
			}
			fmt.Println("Not logged in")
			return 1
		}
		writeJSON(auth, map[string]any{"OPENAI_API_KEY": nil, "tokens": map[string]any{
			"id_token": idToken(email(label, args), "plus"), "access_token": "a", "refresh_token": "r",
		}})
		fmt.Println("Successfully logged in")
		return 0
	case "logout":
		_ = os.Remove(auth)
		fmt.Println("Successfully logged out")
		return 0
	case "mcp":
		name := args[2]
		b, _ := os.ReadFile(toml)
		section := "[mcp_servers." + name + "]"
		switch args[1] {
		case "add": // like codex: replaces an existing entry
			if i := strings.Index(string(b), section); i >= 0 {
				b = b[:i]
			}
			cmd := args[slices.Index(args, "--")+1:]
			q, _ := json.Marshal(cmd[1:])
			b = append(b, fmt.Sprintf("\n%s\ncommand = %q\nargs = %s\n", section, cmd[0], q)...)
		case "remove":
			s := string(b)
			i := strings.Index(s, section)
			if i < 0 { // codex reports this with status 0
				fmt.Printf("No MCP server named '%s' found.\n", name)
				return 0
			}
			end := strings.Index(s[i+len(section):], "\n[")
			if end < 0 {
				s = s[:i]
			} else {
				s = s[:i] + s[i+len(section)+end+1:]
			}
			b = []byte(strings.TrimRight(s, "\n") + "\n")
		}
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(toml, b, 0o644)
		return 0
	case "app-server":
		return appServer(loggedIn, has("LIMITED"), dir, label)
	case "exec", "e":
		jsonOut := slices.Contains(args, "--json")
		fail := ""
		switch {
		case !loggedIn || has("EXPIRED"):
			fail = "401 Unauthorized: Your access token could not be refreshed. Please log out and sign in again."
		case has("LIMITED"):
			fail = "You've hit your usage limit. Try again later."
		}
		if fail != "" && has("TOOL_FIRST") {
			if jsonOut {
				emit(map[string]any{"type": "item.started", "item": map[string]any{"type": "command_execution", "command": "touch x"}})
			}
			sideEffect(label)
		}
		if fail != "" {
			if jsonOut {
				emit(map[string]any{"type": "turn.failed", "error": map[string]any{"message": fail}})
			} else {
				fmt.Fprintln(os.Stderr, "ERROR: "+fail)
			}
			return 1
		}
		text := "answer from " + label + ": " + prompt(args)
		if jsonOut {
			emit(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": text}})
			emit(map[string]any{"type": "turn.completed"})
		} else {
			fmt.Println(text)
		}
		return 0
	}
	fmt.Printf("codex interactive as %s args=%q\n", label, args)
	return 0
}

// appServer answers the two JSON-RPC calls aims makes.
func appServer(loggedIn, limited bool, dir, label string) int {
	pct := 12.0
	if b, err := os.ReadFile(filepath.Join(dir, "USAGE")); err == nil {
		fmt.Sscan(strings.TrimSpace(string(b)), &pct)
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": *req.ID}
		switch req.Method {
		case "initialize":
			reply["result"] = map[string]any{"userAgent": "fake"}
		case "account/read":
			if !loggedIn {
				reply["result"] = map[string]any{"account": nil, "requiresOpenaiAuth": true}
			} else {
				reply["result"] = map[string]any{"account": map[string]any{"type": "chatgpt", "email": label + "@example.com", "planType": "plus"}}
			}
		case "account/rateLimits/read":
			primary := map[string]any{"usedPercent": pct, "windowDurationMins": 300, "resetsAt": 4102444800}
			limits := map[string]any{"primary": primary}
			if limited {
				primary["usedPercent"] = 100
				limits["rateLimitReachedType"] = "usage_limit_reached"
			}
			reply["result"] = map[string]any{"rateLimits": limits}
		default:
			reply["error"] = map[string]any{"code": -32601, "message": "unknown method"}
		}
		emit(reply)
	}
	return 0
}
