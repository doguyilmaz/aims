package claude

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

var (
	_ tool.Supervisor = (*Adapter)(nil)
	_ tool.Hooker     = (*Adapter)(nil)
	_ tool.Diagnoser  = (*Adapter)(nil)
)

// subcommands are Claude Code's commands that are not a conversation.
var subcommands = []string{
	"agents", "attach", "auth", "auto-mode", "config", "doctor", "gateway", "import", "install",
	"logs", "mcp", "migrate-installer", "plugin", "plugins", "purge", "respawn", "rm",
	"setup-token", "stop", "kill", "update", "upgrade",
}

// Supervises leaves out subcommands, runs in the background, and sessions in
// a worktree or tmux, which a resumed run would not find its way back to.
func (a *Adapter) Supervises(args []string) bool {
	if a.IsHeadless(args) || (len(args) > 0 && slices.Contains(subcommands, args[0])) {
		return false
	}
	for _, f := range []string{"-h", "--help", "-v", "--version", "--bg", "--background", "-w", "--worktree", "--tmux"} {
		if tool.HasFlag(args, f) {
			return false
		}
	}
	return true
}

// hookEvents are the Claude Code hooks a supervised session reports: the
// conversation opened, a prompt sent, the end of every turn, and a turn
// ended by an API error such as a usage limit.
var hookEvents = []string{"SessionStart", "UserPromptSubmit", "Stop", "StopFailure"}

// Watch passes Claude Code the hooks with --settings, merged into the
// user's own --settings when there is one, so the user's settings files stay
// untouched and a session started without aims reports nothing. The merged
// settings go in a file of the run, readable by the user only, rather than
// on the command line, where other users can read it. The hooks record the
// turns in the run's directory.
func (*Adapter) Watch(w tool.Watch) ([]string, func() []tool.SessionEvent, error) {
	args, err := withSettings(w.Args, shellQuote(w.Self)+" hook claude", filepath.Join(w.RunDir, "settings.json"))
	if err != nil {
		return nil, nil, err
	}
	return args, tool.FollowEvents(w.RunDir), nil
}

// withSettings writes the user's --settings (or none) with the hooks added
// to file, and points args at it.
func withSettings(args []string, command, file string) ([]string, error) {
	write := func(settings string) (string, error) {
		merged, err := withHooks(settings, command)
		if err != nil {
			return "", err
		}
		return file, fsx.WriteFile(file, []byte(merged), 0o600)
	}
	for i, a := range args {
		if a == "--" {
			break
		}
		value, inline := strings.CutPrefix(a, "--settings=")
		if a == "--settings" && i+1 < len(args) {
			value = args[i+1]
		} else if !inline {
			continue
		}
		path, err := write(value)
		if err != nil {
			return nil, err
		}
		out := slices.Clone(args)
		if inline {
			out[i] = "--settings=" + path
		} else {
			out[i+1] = path
		}
		return out, nil
	}
	path, err := write("{}")
	if err != nil {
		return nil, err
	}
	return append([]string{"--settings", path}, args...), nil
}

// withHooks adds command as a hook for hookEvents to settings, given as JSON
// or as the path of a JSON file.
func withHooks(settings, command string) (string, error) {
	doc := settings
	if !strings.HasPrefix(strings.TrimSpace(settings), "{") {
		b, err := os.ReadFile(settings)
		if err != nil {
			return "", fmt.Errorf("--settings: %w", err)
		}
		doc = string(b)
	}
	if !gjson.Valid(doc) {
		return "", fmt.Errorf("--settings is not valid JSON")
	}
	group := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
	for _, ev := range hookEvents {
		var err error
		if doc, err = sjson.Set(doc, "hooks."+ev+".-1", group); err != nil {
			return "", err
		}
	}
	return doc, nil
}

// shellQuote makes s one word for the shell that runs hooks: sh, or on
// Windows Git Bash or cmd, which both take a double-quoted path.
func shellQuote(s string) string {
	if runtime.GOOS == "windows" {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// HookEvent reads a Stop or StopFailure hook. Of the errors a turn can end
// on, only a limit and a dead login are ones another account fixes.
func (*Adapter) HookEvent(raw []byte) (tool.SessionEvent, bool) {
	j := gjson.ParseBytes(raw)
	name, id := j.Get("hook_event_name").String(), j.Get("session_id").String()
	if id == "" || !slices.Contains(hookEvents, name) {
		return tool.SessionEvent{}, false
	}
	ev := tool.SessionEvent{Session: id, Transcript: j.Get("transcript_path").String()}
	switch name {
	case "SessionStart":
		ev.Opened = true
		return ev, true
	case "UserPromptSubmit":
		ev.Started = true
		return ev, true
	case "Stop":
		return ev, true
	}
	ev.Text = strings.TrimSpace(j.Get("last_assistant_message").String() + "\n" + j.Get("error_details").String())
	ev.Failure = tool.Classify(ev.Text)
	switch j.Get("error").String() {
	case "authentication_failed", "oauth_org_not_allowed", "account_on_hold":
		ev.Failure = tool.FailLogin
	case "rate_limit":
		if ev.Failure == tool.FailNone {
			ev.Failure = tool.FailBusy
		}
	}
	return ev, ev.Failure != tool.FailNone
}

// sessionFlags pick or name a conversation; a resumed run drops them, as
// they would start or open another one.
var sessionFlags = []string{
	"-c", "--continue", "-r", "--resume", "--session-id", "--fork-session",
	"--from-pr", "--teleport", "-n", "--name",
}

func (*Adapter) ResumeArgs(bin string, args []string, id, prompt string) ([]string, error) {
	arity, err := optionArity(bin)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break // the rest is the prompt
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			continue // the prompt
		}
		name, _, inline := strings.Cut(a, "=")
		end := i + 1
		if !inline {
			switch arity[name] {
			case argRequired:
				end = min(i+2, len(args))
			case argOptional:
				if end < len(args) && !strings.HasPrefix(args[end], "-") {
					end++
				}
			case argVariadic:
				for end < len(args) && !strings.HasPrefix(args[end], "-") {
					end++
				}
			}
		}
		if !slices.Contains(sessionFlags, name) {
			out = append(out, args[i:end]...)
		}
		i = end - 1
	}
	if id == "" {
		return out, nil
	}
	out = append(out, "--resume", id)
	if prompt != "" {
		out = append(out, "--", prompt)
	}
	return out, nil
}

const (
	argNone = iota
	argRequired
	argOptional
	argVariadic
)

// optionLine matches an option in `claude --help`:
// "  -r, --resume [value]", "  --add-dir <directories...>".
var optionLine = regexp.MustCompile(`^\s+((?:-{1,2}[\w-]+)(?:, -{1,2}[\w-]+)*)(?: ([<\[][^>\]]*[>\]]))?`)

// optionArity reads which options take a value from the installed claude's
// help, so a prompt is told apart from an option's value whatever version
// is installed.
func optionArity(bin string) (map[string]int, error) {
	out := map[string]int{}
	help, code, err := proc.Output(bin, []string{"--help"}, nil, 10*time.Second)
	if err == nil && (code != 0 || !strings.Contains(help, "--resume")) {
		err = fmt.Errorf("claude --help did not list its options")
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(strings.NewReader(help))
	for sc.Scan() {
		m := optionLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		kind := argNone
		switch v := m[2]; {
		case v == "":
		case strings.Contains(v, "..."):
			kind = argVariadic
		case strings.HasPrefix(v, "<"):
			kind = argRequired
		default:
			kind = argOptional
		}
		for n := range strings.SplitSeq(m[1], ", ") {
			out[n] = kind
		}
	}
	return out, nil
}

// managedSettings is where an organization's policy for Claude Code lives.
func managedSettings() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		return `C:\Program Files\ClaudeCode\managed-settings.json`
	}
	return "/etc/claude-code/managed-settings.json"
}

// Diagnose reports settings that turn off the hooks a session reports its
// turns with, so it would not move between accounts.
func (*Adapter) Diagnose(h tool.Home) []string {
	var out []string
	if b, err := os.ReadFile(filepath.Join(h.Dir, "settings.json")); err == nil && gjson.GetBytes(b, "disableAllHooks").Bool() {
		out = append(out, "disableAllHooks is set in settings.json, so sessions do not move between accounts by themselves")
	}
	if b, err := os.ReadFile(managedSettings()); err == nil && (gjson.GetBytes(b, "allowManagedHooksOnly").Bool() || gjson.GetBytes(b, "disableAllHooks").Bool()) {
		out = append(out, "your organization's Claude Code policy blocks other hooks, so sessions do not move between accounts by themselves")
	}
	return out
}
