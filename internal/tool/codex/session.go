package codex

import (
	"bufio"
	"io"
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
	"github.com/doguyilmaz/aims/internal/proc"
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
// the prompt, or resume's or fork's conversation. Which options take a
// value comes from the installed codex's help, and from valueFlags when
// that cannot be read.
func (*Adapter) ResumeArgs(bin string, args []string, id, prompt string) ([]string, error) {
	arity := optionArity(bin)
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, _, inline := strings.Cut(a, "=")
		switch {
		case a == "--":
			i = len(args)
		case slices.Contains(resumeFlags, a) || !strings.HasPrefix(a, "-"):
		case inline || arity[name] == tool.ArgNone:
			out = append(out, a)
		case arity[name] == tool.ArgVariadic:
			end := i + 1
			for end < len(args) && !strings.HasPrefix(args[end], "-") {
				end++
			}
			out = append(out, args[i:end]...)
			i = end - 1
		case arity[name] == tool.ArgOptional && (i+1 >= len(args) || strings.HasPrefix(args[i+1], "-")):
			out = append(out, a)
		default:
			out = append(out, args[i:min(i+2, len(args))]...)
			i++
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

// optionArity is valueFlags, with what the help of codex and of its resume
// command adds.
func optionArity(bin string) map[string]tool.Arity {
	out := map[string]tool.Arity{}
	for f := range valueFlags {
		out[f] = tool.ArgRequired
	}
	if bin == "" {
		return out
	}
	for _, args := range [][]string{{"--help"}, {"resume", "--help"}} {
		if help, code, err := proc.Output(bin, args, nil, 10*time.Second); err == nil && code == 0 {
			maps.Copy(out, tool.ParseOptions(help))
		}
	}
	return out
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
	f := &follower{sessions: sessions, since: w.Since.Add(-2 * time.Second), cwd: fsx.Real(workDir(w)), metas: map[string]rollout{}, pid: w.PID,
		homes: []string{fsx.Real(w.Home.Dir), filepath.Dir(sessions)}}
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
	// pid is codex's process; holder, once known, the one that writes into
	// homes: itself, or the real codex a wrapper started, which wide looks
	// for among its children. exact: the rollouts codex has open can be
	// read, so discover's guess is not needed.
	pid    func() int
	homes  []string
	holder int
	wide   bool
	looked time.Time
	// tracks: the open rollouts could be read at least once; exact: codex
	// was seen writing one, so discover's guess is not needed.
	tracks, exact bool
	// read is where each conversation left was, to go on from there when
	// codex returns to it.
	read map[string]place
	// strict: the file was entered part way, so a turn without a time may be
	// an old one.
	strict bool
	// lost: discover found two conversations that could be the run's; told,
	// that was reported.
	lost, told bool
}

type place struct {
	offset int64
	strict bool
}

// retryAfter is how long the follower waits to ask again after the system
// could not say which files codex has open.
const retryAfter = time.Minute

// openEvery is how often the follower asks which rollouts codex has open.
var openEvery = 5 * time.Second

// tailRead is how much of a conversation codex switched to is read back for
// what happened since the last look: far more than a resume and a failed
// turn write.
const tailRead = 1 << 20

var (
	writing  = proc.Writing
	children = proc.Children
)

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
	out := f.track()
	if f.path == "" && !f.exact {
		f.discover()
		// Where the open rollouts can be read, track settles it instead.
		if f.lost && f.path == "" && !f.told && !f.tracks {
			f.told = true
			out = append(out, tool.SessionEvent{Lost: true})
		}
	}
	if f.path == "" {
		return out
	}
	var lines [][]byte
	lines, f.offset = tool.ReadLines(f.path, f.offset)
	for _, l := range lines {
		ev, at, ok := turn(l)
		if !ok || (at.IsZero() && f.strict) {
			continue
		}
		// A forked conversation starts with its parent's turns.
		if at.IsZero() || !at.Before(f.since.Truncate(time.Second)) {
			ev.Session = f.id
			out = append(out, ev)
		}
	}
	return out
}

// track follows the conversation codex has open: of the user rollouts it
// holds for writing, the one written last. That sees a conversation opened
// inside codex (/resume, /new, /fork), and tells apart two codex sessions in
// one directory, which discover cannot.
func (f *follower) track() []tool.SessionEvent {
	if f.pid == nil || time.Since(f.looked) < openEvery {
		return nil
	}
	pid := f.pid()
	if pid <= 0 {
		return nil
	}
	last := f.looked
	f.looked = time.Now()
	if f.holder != 0 && !proc.Alive(f.holder) {
		f.holder, f.wide = 0, true
	}
	pids := []int{f.holder}
	if f.holder == 0 {
		pids = []int{pid}
		if f.wide {
			pids = append(pids, children(pid)...)
		}
	}
	held, ok := writing(pids...)
	if !ok {
		f.looked = f.looked.Add(retryAfter - openEvery)
		return nil
	}
	f.tracks = true
	if f.holder == 0 {
		for _, p := range pids {
			if slices.ContainsFunc(held[p], f.inHome) {
				f.holder = p
				break
			}
		}
		f.wide = f.holder == 0
	}
	var pick string
	var pickAt time.Time
	current := false
	for p, files := range held {
		for _, path := range files {
			name := filepath.Base(path)
			if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") || !f.meta(path).user {
				continue
			}
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			if f.holder == 0 {
				f.holder = p
			}
			f.exact = true
			current = current || same(path, f.path)
			if pick == "" || fi.ModTime().After(pickAt) {
				pick, pickAt = path, fi.ModTime()
			}
		}
	}
	if pick == "" || same(pick, f.path) {
		return nil
	}
	if fi, err := os.Stat(f.path); current && err == nil && !pickAt.After(fi.ModTime()) {
		return nil
	}
	if f.path != "" {
		if f.read == nil {
			f.read = map[string]place{}
		}
		f.read[f.path] = place{f.offset, f.strict}
	}
	f.path, f.id = pick, f.meta(pick).id
	if at, ok := f.read[pick]; ok {
		f.offset, f.strict = at.offset, at.strict
	} else {
		f.offset, f.strict = tail(pick), true
	}
	if last.After(f.since) {
		f.since = last
	}
	return []tool.SessionEvent{{Session: f.id, Opened: true}}
}

func (f *follower) inHome(path string) bool {
	return slices.ContainsFunc(f.homes, func(h string) bool { return fsx.Within(path, h) })
}

// meta is readMeta, kept once the file's first line is whole.
func (f *follower) meta(path string) rollout {
	r, ok := f.metas[path]
	if !ok {
		r = readMeta(path)
		if r.id != "" {
			f.metas[path] = r
		}
	}
	return r
}

func same(a, b string) bool { return a == b || (a != "" && b != "" && fsx.Real(a) == fsx.Real(b)) }

// tail is where the last tailRead bytes of path begin, at a line's start.
func tail(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= tailRead {
		return 0
	}
	file, err := os.Open(path)
	if err != nil {
		return fi.Size()
	}
	defer file.Close()
	start := fi.Size() - tailRead
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return fi.Size()
	}
	skipped, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil {
		return fi.Size()
	}
	return start + int64(len(skipped))
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
		r := f.meta(path)
		if r.id == "" || !r.user || fsx.Real(r.cwd) != f.cwd || (r.started.Before(f.since) && !f.anyDay) {
			continue
		}
		if pick != "" {
			f.lost = true
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
	line, err := bufio.NewReader(file).ReadString('\n')
	if err != nil || gjson.Get(line, "type").String() != "session_meta" {
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
	// A turn's own time, else when its line was written.
	when := func(field string) time.Time {
		if at := unix(p.Get(field)); !at.IsZero() {
			return at
		}
		at, _ := time.Parse(time.RFC3339Nano, j.Get("timestamp").String())
		return at
	}
	switch p.Get("type").String() {
	case "task_started":
		return tool.SessionEvent{Started: true}, when("started_at"), true
	case "turn_aborted":
		return tool.SessionEvent{}, when("completed_at"), true
	case "task_complete":
	default:
		return tool.SessionEvent{}, time.Time{}, false
	}
	at := when("completed_at")
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
