// Package bridge describes aims' homes to other applications. Inspection and
// export never run a provider, read credential files, or repair a shared entry.
package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Selection identifies the profile passed to the child for one tool.
type Selection struct {
	Tool    tool.ID `json:"tool"`
	Profile string  `json:"profile"`
}

func envMap(env []string) map[string]string {
	return proc.EnvironmentMap(env)
}

// Environment prepares an application's environment. Both tools use their
// preferred profiles unless requested pins one explicitly. A desktop app is
// not an aims-supervised session; it must not inherit its parent's move hooks.
// This does not synchronize files or change account order.
func Environment(cfg *config.Config, requested map[tool.ID]string, base []string) ([]string, []Selection, error) {
	for id := range requested {
		if _, ok := tools.Parse(string(id)); !ok {
			return nil, nil, fmt.Errorf("unknown tool %q", id)
		}
	}
	env := slices.Clone(base)
	selected := []Selection{}
	for _, a := range tools.All() {
		id := a.ID()
		name, err := cfg.Preferred(id, requested[id])
		if err != nil {
			return nil, nil, err
		}
		if name == "" {
			continue
		}
		if err := profiles.RequireIsolation(cfg, a, name); err != nil {
			return nil, nil, err
		}
		env, _ = profiles.Env(cfg, a, name, env)
		// A program may start the CLI through aims' shell integration later.
		env = append(env, config.PinVar(id)+"="+name)
		selected = append(selected, Selection{id, name})
	}
	m := envMap(env)
	// Profile environment is unrestricted. Reject cross-provider overrides
	// rather than silently running one selected tool as another login.
	for _, s := range selected {
		if m[config.PinVar(s.Tool)] != s.Profile {
			return nil, nil, fmt.Errorf("profile environment overrides %s's terminal pin; remove the cross-provider override before starting an application", s.Tool)
		}
		a := tools.Get(s.Tool)
		want, _ := profiles.Env(cfg, a, s.Profile, base)
		w := envMap(want)
		l := a.Layout()
		keys := append([]string{l.HomeEnv, l.StateEnv, l.MemoryEnv}, l.ConflictingEnv...)
		for _, k := range keys {
			if k != "" && m[k] != w[k] {
				return nil, nil, fmt.Errorf("profile environment overrides %s's %s; remove the cross-provider override before starting an application", s.Tool, k)
			}
		}
	}
	cleanHost(m)
	env = env[:0]
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	slices.Sort(env)
	return env, selected, nil
}

func cleanHost(m map[string]string) {
	for _, k := range []string{profiles.SessionVar, profiles.RunVar, profiles.ModeVar,
		"CLAUDECODE", "CLAUDE_CODE_SSE_PORT", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_HOST_SESSION_ID", "CLAUDE_PID",
		"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_TOOL_CALL_ID", "CODEX_INTERNAL_ORIGINATOR_OVERRIDE"} {
		delete(m, k)
	}
}

// AgentEnvironment routes only the selected tool. The client owns its process,
// protocol, retries and lifecycle: there is no auth probe, migration or failover.
func AgentEnvironment(cfg *config.Config, a tool.Adapter, explicit string, base []string) ([]string, string, error) {
	name, err := cfg.Preferred(a.ID(), explicit)
	if err != nil {
		return nil, "", err
	}
	env := base
	if name != "" {
		if err := profiles.RequireIsolation(cfg, a, name); err != nil {
			return nil, "", err
		}
		env, _ = profiles.Env(cfg, a, name, base)
	}
	m := envMap(env)
	cleanHost(m)
	if name != "" {
		m[profiles.SessionVar] = string(a.ID()) + ":" + name
		m[profiles.ModeVar] = string(profiles.Unwatched)
		m[config.PinVar(a.ID())] = name
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out, name, nil
}

// History describes the actual transcript directory, following links without
// opening conversation files. A shared directory says nothing about the login.
type History struct {
	Path   string `json:"path"`
	Real   string `json:"realPath"`
	Exists bool   `json:"exists"`
	Shared bool   `json:"sharedWithHub"`
}

// Profile contains routing information only. In particular it has no login
// status, e-mail, token, or arbitrary profile environment values.
type Profile struct {
	Name        string            `json:"name"`
	Home        string            `json:"home"`
	HomeEnv     string            `json:"homeEnvValue"`
	Share       string            `json:"share"`
	Environment map[string]string `json:"environment"`
	History     History           `json:"history"`
	Warnings    []string          `json:"warnings,omitempty"`
}

// ToolReport compares the calling process' home with aims' configured hub.
type ToolReport struct {
	Tool           tool.ID   `json:"tool"`
	HomeVariable   string    `json:"homeVariable"`
	Hub            string    `json:"hub"`
	Preferred      string    `json:"preferred,omitempty"`
	CurrentHome    string    `json:"currentHome"`
	CurrentProfile string    `json:"currentProfile,omitempty"`
	CurrentHistory History   `json:"currentHistory"`
	ConflictingEnv []string  `json:"conflictingEnvironment,omitempty"`
	Profiles       []Profile `json:"profiles"`
	Warnings       []string  `json:"warnings,omitempty"`
}

func history(home, hub string, a tool.Adapter) History {
	name := a.Layout().TranscriptDir
	p, h := filepath.Join(home, name), filepath.Join(hub, name)
	fi := fsx.Stat(p)
	return History{Path: p, Real: fsx.Real(p), Exists: fi != nil && fi.IsDir(), Shared: samePath(p, h)}
}

func samePath(a, b string) bool {
	if fsx.Real(a) == fsx.Real(b) {
		return true
	}
	ai, bi := fsx.Stat(a), fsx.Stat(b)
	return ai != nil && bi != nil && os.SameFile(ai, bi)
}

func routing(cfg *config.Config, a tool.Adapter, name string) map[string]string {
	env, _ := profiles.Env(cfg, a, name, nil)
	m := envMap(env)
	out := map[string]string{}
	l := a.Layout()
	for _, k := range []string{l.HomeEnv, l.StateEnv, l.MemoryEnv} {
		if k != "" {
			out[k] = m[k] // Empty means unset, including inherited overrides.
		}
	}
	return out
}

// Inspect is strictly local and read-only, including when links are broken or
// the caller inherited an orchestrator's temporary home.
func Inspect(cfg *config.Config, base []string) []ToolReport {
	m := envMap(base)
	out := make([]ToolReport, 0, len(tools.All()))
	for _, a := range tools.All() {
		id, l := a.ID(), a.Layout()
		hub := cfg.EnsureHub(id)
		current := filepath.Join(fsx.Home(), l.DefaultHome)
		if m[l.HomeEnv] != "" {
			current = fsx.Abs(m[l.HomeEnv])
		}
		r := ToolReport{Tool: id, HomeVariable: l.HomeEnv, Hub: hub, CurrentHome: current,
			CurrentProfile: cfg.ProfileForHomeEnv(id, m[l.HomeEnv]), CurrentHistory: history(current, hub, a), Profiles: []Profile{}}
		r.Preferred, _ = cfg.Preferred(id, "")
		for _, k := range l.ConflictingEnv {
			if m[k] != "" {
				r.ConflictingEnv = append(r.ConflictingEnv, k)
			}
		}
		if r.CurrentProfile == "" && !samePath(current, hub) {
			r.Warnings = append(r.Warnings, "this process uses an external home; sharing transcripts does not share its login")
		}
		if l.ExactHomeEnv && r.CurrentProfile == "" && samePath(current, hub) && m[l.HomeEnv] != cfg.Tools[id].HubEnv {
			r.Warnings = append(r.Warnings, "the home variable differs from the saved login's literal value; the same folder can select another keychain entry and account config")
		}
		for _, n := range cfg.Tools[id].Order {
			p := cfg.Tools[id].Profiles[n]
			pr := Profile{Name: n, Home: cfg.Dir(id, n), HomeEnv: cfg.HomeEnv(id, n), Share: p.ShareMode(), Environment: routing(cfg, a, n)}
			actualHome := pr.Home
			if v := pr.Environment[l.HomeEnv]; v != "" {
				actualHome = fsx.Abs(v)
			}
			pr.History = history(actualHome, hub, a)
			if p.ShareMode() == config.ShareAll && !slices.ContainsFunc(p.Exclude, l.IsHistory) && !pr.History.Shared {
				pr.Warnings = append(pr.Warnings, "transcripts are not linked to the hub; close affected tools, then run aims sync")
			}
			if l.StateEnv != "" && p.ShareMode() == config.ShareAll && pr.Environment[l.StateEnv] == "" && !p.Existing {
				pr.Warnings = append(pr.Warnings, "databases remain private; aims will not discard an independent database")
			}
			if len(p.Env) > 0 {
				pr.Warnings = append(pr.Warnings, "custom profile environment is omitted from this report and from T3 exports; aims exec passes it to the child")
			}
			r.Profiles = append(r.Profiles, pr)
		}
		out = append(out, r)
	}
	return out
}
