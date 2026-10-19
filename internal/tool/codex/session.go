package codex

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
	return append(slices.Clone(noDaemon), w.Args...), f.poll, nil
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
}

type rollout struct {
	id, cwd string
	started time.Time
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
	if !f.byID {
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

// discover finds the rollout this run writes, and the next one when the
// user starts another conversation in it (/new): the newest rollout of a
// conversation in the run's working directory that started since the run
// did, and after the current one was last written. A run that opens an old conversation without naming it takes the
// newest one written to since, until a newer conversation starts.
func (f *follower) discover() {
	type candidate struct {
		path string
		mod  time.Time
	}
	var found []candidate
	consider := func(path string) {
		if fi, err := os.Stat(path); err == nil && !fi.ModTime().Before(f.since) && !slices.ContainsFunc(found, func(c candidate) bool { return c.path == path }) {
			found = append(found, candidate{path, fi.ModTime()})
		}
	}
	old := f.anyDay && f.path == ""
	if old {
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
	slices.SortFunc(found, func(a, b candidate) int { return b.mod.Compare(a.mod) })
	for _, c := range found {
		if c.path == f.path {
			return
		}
		r, ok := f.metas[c.path]
		if !ok {
			r = meta(c.path)
			f.metas[c.path] = r
		}
		if r.id == "" || fsx.Real(r.cwd) != f.cwd || (r.started.Before(f.since) && !old) {
			continue
		}
		// A conversation still written to after the newer one started is
		// not one the user left: the newer one is another codex's.
		if fi, err := os.Stat(f.path); f.path != "" && err == nil && fi.ModTime().After(r.started.Add(time.Second)) {
			continue
		}
		f.path, f.id, f.offset = c.path, r.id, 0
		if r.started.Before(f.since) {
			if fi, err := os.Stat(c.path); err == nil {
				f.offset = fi.Size()
			}
		}
		return
	}
}

// meta reads a rollout's first line, its session_meta.
func meta(path string) rollout {
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
	return rollout{id: p.Get("id").String(), cwd: p.Get("cwd").String(), started: started}
}

// turn reads a task_started or task_complete line, and when it happened: a
// turn that began, went well, or failed in a way another account can fix.
func turn(line []byte) (tool.SessionEvent, time.Time, bool) {
	j := gjson.ParseBytes(line)
	if j.Get("type").String() != "event_msg" {
		return tool.SessionEvent{}, time.Time{}, false
	}
	p := j.Get("payload")
	switch p.Get("type").String() {
	case "task_started":
		return tool.SessionEvent{Started: true}, unix(p.Get("started_at")), true
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
