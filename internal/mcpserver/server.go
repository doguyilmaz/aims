// Package mcpserver is `aims mcp`: an MCP server over stdio that lets Claude
// Code or Codex see, switch and use the user's accounts.
package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

const instructions = "aims manages several Claude Code and Codex accounts on this machine. " +
	"A running session cannot change its own account: aims_switch and aims_failover affect sessions started afterwards, " +
	"and aims_run uses another account right now for a self-contained task. Logins need a browser, so give the user the command from aims_login_help."

// New builds the server.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aims", Title: "aims", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	toolList := strings.Join(tools.Names(), " or ")

	mcp.AddTool(s, &mcp.Tool{
		Name: "aims_status",
		Description: "List the accounts (profiles) aims manages for each tool: which is active, login state, plan usage and cooldowns. " +
			"The account of the current session is in env " + profiles.SessionVar + " (tool:profile). " +
			"live=true asks the providers (Codex: free account API; Claude: a one-word prompt to the smallest model).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, statusTool)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "aims_switch",
		Description: "Make a profile the active account for new sessions of one tool, or of every tool that has a profile with that name. The running session keeps its account.",
	}, switchTool)

	mcp.AddTool(s, &mcp.Tool{
		Name: "aims_failover",
		Description: "The current account hit its usage limit or its login died: mark it (until its reset time, or for `minutes`) and make the next usable account active. " +
			"Then tell the user to continue the conversation with `aims claude --continue` or `aims codex resume --last`.",
	}, failoverTool)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "aims_clear",
		Description: "Clear the cooldown and login marks of one profile, or of all profiles of a tool.",
	}, clearTool)

	mcp.AddTool(s, &mcp.Tool{
		Name: "aims_run",
		Description: "Run a self-contained task right now with another account (`claude -p` or `codex exec`), failing over to the next account on a limit or login error " +
			"when the failed attempt did nothing. Uses the tool's default permissions and returns its final answer. Tool: " + toolList + ".",
	}, runTool)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "aims_login_help",
		Description: "The terminal command a person must run to log a profile in. Logins open a browser; the model cannot complete them.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, loginHelpTool)
	return s
}

// Serve runs the server on stdio until the client disconnects or aims is
// asked to stop, then stops any task it started.
func Serve(ctx context.Context, version string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := New(version).Run(ctx, &mcp.StdioTransport{})
	proc.KillAll()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func fail(err error) (*mcp.CallToolResult, any, error) {
	r := text("Error: " + err.Error())
	r.IsError = true
	return r, nil, nil
}

func adapter(name string, required bool) (tool.Adapter, error) {
	if name == "" && !required {
		return nil, nil
	}
	a, ok := tools.Parse(name)
	if !ok {
		return nil, fmt.Errorf("tool must be one of: %s", strings.Join(tools.Names(), ", "))
	}
	return a, nil
}

type statusIn struct {
	Tool string `json:"tool,omitempty" jsonschema:"limit to one tool: claude or codex"`
	Live bool   `json:"live,omitempty" jsonschema:"also ask the providers to verify logins and usage"`
}

func statusTool(ctx context.Context, _ *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, false)
	if err != nil {
		return fail(err)
	}
	var ids []tool.ID
	if a != nil {
		ids = []tool.ID{a.ID()}
	}
	var live []string
	if in.Live {
		cfg, err := config.Load()
		if err != nil {
			return fail(err)
		}
		for _, id := range idsOrAll(ids) {
			for _, n := range cfg.Tools[id].Order {
				p := profiles.Live(ctx, cfg, tools.Get(id), n)
				line := fmt.Sprintf("%s/%s: %s", id, n, p.Status)
				if p.Detail != "" {
					line += " (" + p.Detail + ")"
				}
				live = append(live, line)
			}
		}
	}
	report, err := ops.Status(ctx, ids)
	if err != nil {
		return fail(err)
	}
	var b strings.Builder
	if s := os.Getenv(profiles.SessionVar); s != "" {
		fmt.Fprintf(&b, "This session runs as %s.\n\n", s)
	} else {
		b.WriteString("This session was not started through aims, so its account is unknown.\n\n")
	}
	b.WriteString(ui.PlainStatus(report))
	if len(live) > 0 {
		b.WriteString("\nLive check:\n" + strings.Join(live, "\n") + "\n")
	}
	r := text(b.String())
	r.StructuredContent = map[string]any{"session": os.Getenv(profiles.SessionVar), "tools": report}
	return r, nil, nil
}

type switchIn struct {
	Tool    string `json:"tool,omitempty" jsonschema:"claude or codex; omit to switch every tool that has this profile"`
	Profile string `json:"profile" jsonschema:"profile name, e.g. personal or work"`
}

func switchTool(_ context.Context, _ *mcp.CallToolRequest, in switchIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, false)
	if err != nil {
		return fail(err)
	}
	changed, err := ops.Use(a, in.Profile)
	if err != nil {
		return fail(err)
	}
	var names []string
	for _, c := range changed {
		names = append(names, c.Title())
	}
	return text(fmt.Sprintf("%s now use %q for new sessions. This session keeps its current account.", strings.Join(names, " and "), in.Profile)), nil, nil
}

type failoverIn struct {
	Tool    string `json:"tool" jsonschema:"claude or codex"`
	From    string `json:"from,omitempty" jsonschema:"profile to mark; default: the account of this session, else the one used last"`
	To      string `json:"to,omitempty" jsonschema:"profile to switch to; default: the next usable one"`
	Minutes int    `json:"minutes,omitempty" jsonschema:"cooldown length when the reset time is unknown"`
	Reason  string `json:"reason,omitempty" jsonschema:"limit (default) or login"`
}

func failoverTool(ctx context.Context, _ *mcp.CallToolRequest, in failoverIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, true)
	if err != nil {
		return fail(err)
	}
	from := in.From
	if from == "" {
		if t, p, ok := strings.Cut(os.Getenv(profiles.SessionVar), ":"); ok && t == string(a.ID()) {
			from = p
		}
	}
	res, err := ops.Failover(ctx, a, from, in.To, time.Duration(in.Minutes)*time.Minute, in.Reason)
	if err != nil {
		return fail(err)
	}
	resume := "aims " + string(a.ID()) + " " + strings.Join(a.Commands().Resume, " ")
	msg := fmt.Sprintf("%s: %q is cooling down until %s; %q is now active. To continue this conversation there, exit this session and run: %s",
		a.ID(), res.From, res.Until.Local().Format("Jan 2 15:04"), res.To, resume)
	if res.Pinned != "" && res.Pinned != res.To {
		msg += fmt.Sprintf("\nThis terminal is pinned to %q (%s); aims skips it while it cools down.", res.Pinned, config.PinVar(a.ID()))
	}
	r := text(msg)
	r.StructuredContent = res
	return r, nil, nil
}

type clearIn struct {
	Tool    string `json:"tool" jsonschema:"claude or codex"`
	Profile string `json:"profile,omitempty" jsonschema:"one profile; default: all profiles of the tool"`
}

func clearTool(_ context.Context, _ *mcp.CallToolRequest, in clearIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, true)
	if err != nil {
		return fail(err)
	}
	names, err := ops.ClearMarks(a, in.Profile)
	if err != nil {
		return fail(err)
	}
	return text(fmt.Sprintf("Cleared %s: %s", a.ID(), strings.Join(names, ", "))), nil, nil
}

type runIn struct {
	Tool           string `json:"tool" jsonschema:"claude or codex"`
	Prompt         string `json:"prompt" jsonschema:"the task"`
	Profile        string `json:"profile,omitempty" jsonschema:"run as this profile (no failover); default: the active one"`
	Cwd            string `json:"cwd,omitempty" jsonschema:"working directory; default: where the MCP server runs"`
	Model          string `json:"model,omitempty"`
	Continue       bool   `json:"continue,omitempty" jsonschema:"continue the most recent conversation in cwd"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty" jsonschema:"default 600"`
}

// limited keeps the last n bytes written to it.
type limited struct {
	mu  sync.Mutex
	buf bytes.Buffer
	n   int
}

func (l *limited) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Write(p)
	if over := l.buf.Len() - l.n; over > 0 {
		l.buf.Next(over)
	}
	return len(p), nil
}

func (l *limited) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

func runTool(ctx context.Context, _ *mcp.CallToolRequest, in runIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, true)
	if err != nil {
		return fail(err)
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return fail(errors.New("prompt is empty"))
	}
	timeout := time.Duration(in.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	out, errOut := &limited{n: 64 << 10}, &limited{n: 16 << 10}
	res, err := launch.Run(ctx, launch.Options{
		Tool:     a,
		Profile:  in.Profile,
		Args:     a.HeadlessArgs(tool.HeadlessOptions{Prompt: in.Prompt, Model: in.Model, Continue: in.Continue}),
		Headless: true,
		NoStdin:  true, // stdin is the MCP connection
		Stdout:   out,
		Stderr:   errOut,
		Dir:      in.Cwd,
		Timeout:  timeout,
		// The child is not nested in, or attached to the IDE of, this session.
		StripEnv: []string{"CLAUDECODE", "CLAUDE_CODE_SSE_PORT", "CLAUDE_CODE_ENTRYPOINT"},
	})
	if err != nil {
		return fail(err)
	}
	profile := res.Profile
	if profile == "" {
		profile = "default"
	}
	head := fmt.Sprintf("[%s/%s exit %d", a.ID(), profile, res.Code)
	if res.Failure != tool.FailNone {
		head += ", " + string(res.Failure) + " error"
	}
	head += "]"
	body := res.FinalText
	if body == "" && res.Code == 0 {
		body = out.String()
	}
	parts := []string{head}
	if s := strings.TrimSpace(body); s != "" {
		parts = append(parts, s)
	}
	if res.Code != 0 {
		problem := res.ErrorText
		if problem == "" {
			problem = lastLines(errOut.String(), 15)
		}
		if problem != "" {
			parts = append(parts, "--- error ---\n"+problem)
		}
	}
	r := text(strings.Join(parts, "\n"))
	r.IsError = res.Code != 0
	r.StructuredContent = map[string]any{"profile": res.Profile, "exitCode": res.Code, "failure": string(res.Failure)}
	return r, nil, nil
}

type loginIn struct {
	Tool    string `json:"tool" jsonschema:"claude or codex"`
	Profile string `json:"profile" jsonschema:"profile to log in"`
}

func loginHelpTool(_ context.Context, _ *mcp.CallToolRequest, in loginIn) (*mcp.CallToolResult, any, error) {
	a, err := adapter(in.Tool, true)
	if err != nil {
		return fail(err)
	}
	return text(fmt.Sprintf("Ask the user to run this in a terminal (it opens a browser):\n\n  aims login %s %s\n\n%s", a.ID(), in.Profile, a.Commands().LoginTip)), nil, nil
}

func idsOrAll(ids []tool.ID) []tool.ID {
	if len(ids) == 0 {
		return tools.IDs()
	}
	return ids
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
