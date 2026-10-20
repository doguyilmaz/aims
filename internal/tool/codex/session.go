package codex

import (
	"bufio"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

var _ tool.Supervisor = (*Adapter)(nil)

// subcommands are codex's commands that are not a conversation in the
// terminal. resume and fork are one, as is no command at all.
var subcommands = []string{
	"exec", "e", "review", "login", "logout", "mcp", "mcp-server", "plugin", "app-server",
	"remote-control", "app", "completion", "update", "doctor", "sandbox", "debug", "apply", "a",
	"queue", "archive", "delete", "migrate-rollouts", "unarchive", "cloud", "exec-server",
	"features", "help", "agents",
}

func (*Adapter) Supervises(args []string) bool {
	for _, f := range []string{"-h", "--help", "-V", "--version"} {
		if tool.HasFlag(args, f) {
			return false
		}
	}
	cmd, _ := command(args)
	return !slices.Contains(subcommands, cmd)
}

// command is the first word of args that is neither an option nor an
// option's value, and its index; "" and -1 when there is none.
func command(args []string) (string, int) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--":
			return "", -1
		case valueFlags[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a, i
		}
	}
	return "", -1
}

// resumeFlags choose which conversation resume or fork opens; a resumed run
// names it instead.
var resumeFlags = []string{"--last", "--all", "--include-non-interactive"}

// ResumeArgs keeps the options of args and drops the command and its words:
// the prompt, or resume's or fork's conversation.
func (*Adapter) ResumeArgs(_ string, args []string, id, prompt string) ([]string, error) {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i = len(args)
		case valueFlags[a]:
			out = append(out, args[i:min(i+2, len(args))]...)
			i++
		case slices.Contains(resumeFlags, a) || !strings.HasPrefix(a, "-"):
		default:
			out = append(out, a)
		}
	}
	if id == "" {
		return out, nil
	}
	out = append(out, "resume", id)
	if prompt != "" {
		out = append(out, "--", prompt)
	}
	return out, nil
}

// noDaemon keeps the conversation in the codex process aims stops: a shared
// app-server daemon would outlive it and keep the conversation loaded. A
// config override, unlike --disable, is fine with a codex that does not
// know the feature.
var noDaemon = []string{"-c", "features.daemon_auto_start=false"}

// Watch follows the conversation's rollout, the JSONL file codex writes it
// to. Codex has no hook for a turn that fails, but the rollout records how
// every turn ended.
func (a *Adapter) Watch(w tool.Watch) ([]string, func() []tool.SessionEvent, error) {
	sessions := filepath.Join(w.Home.Dir, "sessions")
	if real, err := filepath.EvalSymlinks(sessions); err == nil {
		sessions = real // WalkDir does not follow a linked root
	}
	f := &follower{sessions: sessions, since: w.Since.Add(-2 * time.Second), cwd: fsx.Real(workDir(w)), metas: map[string]rollout{}}
	if cmd, i := command(w.Args); cmd == "resume" || cmd == "fork" {
		f.anyDay = true
		if next, _ := command(w.Args[i+1:]); cmd == "resume" && next != "" {
			f.find(next)
		}
	}
	args := slices.Clone(noDaemon)
	// Codex hands its shell commands a filtered environment.
	for _, k := range slices.Sorted(maps.Keys(w.Env)) {
		args = append(args, "-c", "shell_environment_policy.set."+k+"="+strconv.Quote(w.Env[k]))
	}
	return append(args, w.Args...), f.poll, nil
}

// workDir is where the conversation runs: -C/--cd, else aims' directory.
func workDir(w tool.Watch) string {
	dir := w.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	for i, a := range w.Args {
		if a == "--" {
			break
		}
		if (a == "-C" || a == "--cd") && i+1 < len(w.Args) {
			if filepath.IsAbs(w.Args[i+1]) {
				return filepath.Clean(w.Args[i+1])
			}
			return filepath.Join(dir, w.Args[i+1])
		}
	}
	return dir
}

// follower reads the rollout of a supervised conversation.
type follower struct {
	sessions string
	since    time.Time
	cwd      string
	// anyDay: the run opens an existing conversation (resume or fork
	// without an id), which can be in any day's folder and older than the
	// run.
	anyDay bool
	// byID: the run resumes a conversation named by its id; its file is
	// known from the start.
	byID   bool
	path   string
	id     string
	offset int64
	metas  map[string]rollout
	walked time.Time
}

type rollout struct {
	id, cwd string
	started time.Time
	// user: a conversation the user started in the terminal, not a codex
	// exec run, a subagent or a review.
	user bool
}

// find follows the rollout of conversation id, from its current end.
func (f *follower) find(id string) {
	matches, _ := filepath.Glob(filepath.Join(f.sessions, "*", "*", "*", "rollout-*-"+id+".jsonl"))
	if len(matches) == 1 {
		f.path, f.id, f.byID = matches[0], id, true
		if fi, err := os.Stat(f.path); err == nil {
			f.offset = fi.Size()
		}
	}
}

func (f *follower) poll() []tool.SessionEvent {
	if f.path == "" {
		f.discover()
	}
	if f.path == "" {
		return nil
	}
	var lines [][]byte
	lines, f.offset = tool.ReadLines(f.path, f.offset)
	var out []tool.SessionEvent
	for _, l := range lines {
		// A forked conversation starts with its parent's turns.
		if ev, at, ok := turn(l); ok && (at.IsZero() || !at.Before(f.since.Truncate(time.Second))) {
			ev.Session = f.id
			out = append(out, ev)
		}
	}
	return out
}

// discover finds the rollout this run writes: the one conversation the user
// started in the run's working directory since the run did. A run that
// opens an old conversation without naming it takes the one written to
// since. When two conversations qualify, the other may be another codex's:
// the run is not followed rather than followed into the wrong one.
func (f *follower) discover() {
	var found []string
	consider := func(path string) {
		if fi, err := os.Stat(path); err == nil && !fi.ModTime().Before(f.since) && !slices.Contains(found, path) {
			found = append(found, path)
		}
	}
	if f.anyDay {
		if time.Since(f.walked) < 10*time.Second {
			return
		}
		f.walked = time.Now()
		_ = filepath.WalkDir(f.sessions, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "rollout-") && strings.HasSuffix(d.Name(), ".jsonl") {
				consider(p)
			}
			return nil
		})
	} else {
		for _, day := range []time.Time{f.since, time.Now()} {
			matches, _ := filepath.Glob(filepath.Join(f.sessions, day.Format("2006/01/02"), "rollout-*.jsonl"))
			for _, m := range matches {
				consider(m)
			}
		}
	}
	var pick string
	var meta rollout
	for _, path := range found {
		r, ok := f.metas[path]
		if !ok {
			r = readMeta(path)
			f.metas[path] = r
		}
		if r.id == "" || !r.user || fsx.Real(r.cwd) != f.cwd || (r.started.Before(f.since) && !f.anyDay) {
			continue
		}
		if pick != "" {
			return
		}
		pick, meta = path, r
	}
	if pick == "" {
		return
	}
	f.path, f.id, f.offset = pick, meta.id, 0
	if meta.started.Before(f.since) {
		if fi, err := os.Stat(pick); err == nil {
			f.offset = fi.Size()
		}
	}
}

// readMeta reads a rollout's first line, its session_meta.
func readMeta(path string) rollout {
	file, err := os.Open(path)
	if err != nil {
		return rollout{}
	}
	defer file.Close()
	line, _ := bufio.NewReader(file).ReadString('\n')
	if gjson.Get(line, "type").String() != "session_meta" {
		return rollout{}
	}
	p := gjson.Get(line, "payload")
	started, _ := time.Parse(time.RFC3339Nano, p.Get("timestamp").String())
	user := p.Get("originator").String() == "codex-tui" && p.Get("thread_source").String() == "user"
	return rollout{id: p.Get("id").String(), cwd: p.Get("cwd").String(), started: started, user: user}
}

// turn reads a task_started, turn_aborted or task_complete line, and when it
// happened: a turn that began, ended (stopped or went well), or failed in a
// way another account can fix.
func turn(line []byte) (tool.SessionEvent, time.Time, bool) {
	j := gjson.ParseBytes(line)
	if j.Get("type").String() != "event_msg" {
		return tool.SessionEvent{}, time.Time{}, false
	}
	p := j.Get("payload")
	switch p.Get("type").String() {
	case "task_started":
		return tool.SessionEvent{Started: true}, unix(p.Get("started_at")), true
	case "turn_aborted":
		return tool.SessionEvent{}, unix(p.Get("completed_at")), true
	case "task_complete":
	default:
		return tool.SessionEvent{}, time.Time{}, false
	}
	at := unix(p.Get("completed_at"))
	e := p.Get("error")
	if !e.Exists() || e.Type == gjson.Null {
		return tool.SessionEvent{}, at, true
	}
	ev := tool.SessionEvent{Text: e.Get("message").String()}
	switch e.Get("codex_error_info").String() {
	case "usage_limit_exceeded":
		ev.Failure = tool.FailLimit
	case "unauthorized":
		ev.Failure = tool.FailLogin
	case "rate_limit_exceeded":
		ev.Failure = tool.FailBusy
	default:
		ev.Failure = tool.Classify(ev.Text)
	}
	return ev, at, ev.Failure != tool.FailNone
}

// unix reads a time in Unix seconds; zero when it is missing.
func unix(v gjson.Result) time.Time {
	if !v.Exists() {
		return time.Time{}
	}
	return time.Unix(v.Int(), 0)
}
