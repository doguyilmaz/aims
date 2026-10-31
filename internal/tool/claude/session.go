package claude

import (
	"encoding/json"
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

// HoldPrompt blocks the prompt of a UserPromptSubmit hook; Claude Code shows
// the reason.
func (*Adapter) HoldPrompt(reason string) []byte {
	b, _ := json.Marshal(map[string]string{"decision": "block", "reason": reason})
	return append(b, '\n')
}

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
	ev := tool.SessionEvent{Session: id, Transcript: j.Get("transcript_path").String(), Mode: j.Get("permission_mode").String()}
	switch name {
	case "SessionStart":
		ev.Opened = true
		return ev, true
	case "UserPromptSubmit":
		ev.Started, ev.Prompt = true, j.Get("prompt").String()
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

// setupFlags are how a session was set up at its start; a resumed run takes
// how it was at its last turn instead, which /model, /effort and shift+tab
// may have changed.
var setupFlags = []string{"--model", "--effort", "--permission-mode"}

func (*Adapter) ResumeArgs(bin string, args []string, r tool.Resume) ([]string, error) {
	help, err := readHelp(bin)
	if err != nil {
		return nil, err
	}
	arity := tool.ParseOptions(help)
	id, prompt := r.ID, r.Prompt
	setup := setupOf(r)
	// An older claude may not know a flag, or a mode a hook reported.
	for f, v := range setup {
		if arity[f] == tool.ArgNone || (f == "--permission-mode" && !strings.Contains(optionHelp(help, f), `"`+v+`"`)) {
			delete(setup, f)
		}
	}
	// A mode carried over replaces bypassing permissions from the start.
	bypass := setup["--permission-mode"] != ""
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
			case tool.ArgRequired:
				end = min(i+2, len(args))
			case tool.ArgOptional:
				if end < len(args) && !strings.HasPrefix(args[end], "-") {
					end++
				}
			case tool.ArgVariadic:
				for end < len(args) && !strings.HasPrefix(args[end], "-") {
					end++
				}
			}
		}
		if !slices.Contains(sessionFlags, name) && (setup[name] == "" || id == "") && (name != "--dangerously-skip-permissions" || !bypass || id == "") {
			out = append(out, args[i:end]...)
		}
		i = end - 1
	}
	if id == "" {
		return out, nil
	}
	for _, f := range setupFlags {
		if v := setup[f]; v != "" {
			out = append(out, f, v)
		}
	}
	out = append(out, "--resume", id)
	if prompt != "" {
		out = append(out, "--", prompt)
	}
	return out, nil
}

// efforts are the levels --effort takes.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// setupOf is how the conversation was set up at its last turn: the model
// and effort of its last answer, and its permission mode. Bypassing
// permissions is not carried over; the flags the session started with
// decide that.
func setupOf(r tool.Resume) map[string]string {
	out := map[string]string{}
	if r.Mode != "" && r.Mode != "bypassPermissions" {
		out["--permission-mode"] = r.Mode
	}
	if r.Transcript == "" {
		return out
	}
	lines := tool.TailLines(r.Transcript, 1<<20)
	for i := len(lines) - 1; i >= 0; i-- {
		j := gjson.ParseBytes(lines[i])
		// An error ending the turn (a limit) is written as an answer too.
		if j.Get("type").String() != "assistant" || j.Get("isSidechain").Bool() || j.Get("isApiErrorMessage").Bool() {
			continue
		}
		m := j.Get("requestedModel").String()
		if m == "" || strings.HasPrefix(m, "-") || strings.HasPrefix(m, "<") {
			continue
		}
		out["--model"] = m
		if e := j.Get("effort").String(); slices.Contains(efforts, e) {
			out["--effort"] = e
		}
		break
	}
	return out
}

// optionHelp is the part of help about one option: its line and the ones
// under it, up to the next option.
func optionHelp(help, option string) string {
	loc := regexp.MustCompile(`(?m)^\s+(-\w, )?` + regexp.QuoteMeta(option) + `\b`).FindStringIndex(help)
	if loc == nil {
		return ""
	}
	rest := help[loc[1]:]
	if j := regexp.MustCompile(`\n\s{1,4}-`).FindStringIndex(rest); j != nil {
		rest = rest[:j[0]]
	}
	return rest
}

// readHelp reads the installed claude's help: which options take a value,
// so a prompt is told apart from an option's value whatever version is
// installed, and which values some of them take.
func readHelp(bin string) (string, error) {
	help, code, err := proc.Output(bin, []string{"--help"}, nil, 10*time.Second)
	if err == nil && (code != 0 || !strings.Contains(help, "--resume")) {
		err = fmt.Errorf("claude --help did not list its options")
	}
	return help, err
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
	if b, err := os.ReadFile(claudeJSON(h)); err == nil {
		out = append(out, plainSecrets(b)...)
	}
	return out
}
