package claude

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

var (
	_ tool.Supervisor = (*Adapter)(nil)
	_ tool.Hooker     = (*Adapter)(nil)
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

// hookEvents are the Claude Code hooks a supervised session reports: a
// prompt sent, the end of every turn, and a turn ended by an API error such
// as a usage limit.
var hookEvents = []string{"UserPromptSubmit", "Stop", "StopFailure"}

// Watch passes Claude Code the hooks with --settings, merged into the
// user's own --settings when there is one, so the user's settings files stay
// untouched and a session started without aims reports nothing. The hooks
// record the turns in the run's directory.
func (*Adapter) Watch(w tool.Watch) ([]string, func() []tool.SessionEvent, error) {
	args, err := withSettings(w.Args, shellQuote(w.Self)+" hook claude")
	if err != nil {
		return nil, nil, err
	}
	return args, tool.FollowEvents(w.RunDir), nil
}

func withSettings(args []string, command string) ([]string, error) {
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
		merged, err := withHooks(value, command)
		if err != nil {
			return nil, err
		}
		out := slices.Clone(args)
		if inline {
			out[i] = "--settings=" + merged
		} else {
			out[i+1] = merged
		}
		return out, nil
	}
	merged, _ := withHooks("{}", command)
	return append([]string{"--settings", merged}, args...), nil
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

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// HookEvent reads a Stop or StopFailure hook. Of the errors a turn can end
// on, only a limit and a dead login are ones another account fixes.
func (*Adapter) HookEvent(raw []byte) (tool.SessionEvent, bool) {
	j := gjson.ParseBytes(raw)
	name, id := j.Get("hook_event_name").String(), j.Get("session_id").String()
	if id == "" || !slices.Contains(hookEvents, name) {
		return tool.SessionEvent{}, false
	}
	ev := tool.SessionEvent{Session: id}
	switch name {
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
