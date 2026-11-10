package bridge

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// T3SettingsPath is T3 Code's current server settings, not Electron's UI state.
// T3CODE_HOME has the same meaning as in T3's CLI launcher.
func T3SettingsPath() string {
	home := os.Getenv("T3CODE_HOME")
	if home == "" {
		home = filepath.Join(fsx.Home(), ".t3")
	}
	return filepath.Join(fsx.Abs(home), "userdata", "settings.json")
}

type Variable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// T3Instance uses the existing-CLI driver. Managed T3 accounts own their login
// elsewhere and cannot be connected by copying auth.json or a refresh token.
type T3Instance struct {
	Driver      string            `json:"driver"`
	DisplayName string            `json:"displayName"`
	Enabled     bool              `json:"enabled"`
	Environment []Variable        `json:"environment"`
	Config      map[string]string `json:"config"`
}

// T3Plan is additive. Existing instances, model choices, threads, and all
// unknown settings remain unchanged. JSON includes only credential-free new
// entries; Merged returns the full document for a private, separate output file.
type T3Plan struct {
	Settings  string                `json:"settingsPath"`
	Add       []string              `json:"add"`
	Reuse     []string              `json:"reuse"`
	Warnings  []string              `json:"warnings,omitempty"`
	Instances map[string]T3Instance `json:"providerInstances"`
	document  map[string]json.RawMessage
	existing  map[string]json.RawMessage
}

// PlanT3 reads settings but never modifies them. Malformed input and reserved
// ID collisions are errors, rather than an excuse to replace an account.
func PlanT3(cfg *config.Config, settings string) (*T3Plan, error) {
	p := &T3Plan{Settings: fsx.Abs(settings), Add: []string{}, Reuse: []string{},
		Instances: map[string]T3Instance{}, document: map[string]json.RawMessage{}, existing: map[string]json.RawMessage{}}
	f, err := os.Open(p.Settings)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(b) > 8<<20 {
			return nil, fmt.Errorf("T3 settings exceed 8 MiB")
		}
		if err := json.Unmarshal(b, &p.document); err != nil || p.document == nil {
			return nil, fmt.Errorf("%s must contain a JSON object; it has not been changed", p.Settings)
		}
		if raw, ok := p.document["providerInstances"]; ok {
			if err := json.Unmarshal(raw, &p.existing); err != nil || p.existing == nil {
				return nil, fmt.Errorf("T3 providerInstances must be a JSON object; settings have not been changed")
			}
		}
	}
	for _, a := range tools.All() {
		for _, name := range cfg.Tools[a.ID()].Order {
			instance, err := t3Instance(cfg, a, name)
			if err != nil {
				return nil, err
			}
			// A reversible, collision-free encoding also handles dots and case
			// in profile names without accidentally merging two accounts.
			id := instance.Driver + "_aims_" + hex.EncodeToString([]byte(name))
			if raw, ok := p.existing[id]; ok {
				want, _ := json.Marshal(instance)
				var v, w any
				_ = json.Unmarshal(raw, &v)
				_ = json.Unmarshal(want, &w)
				vb, _ := json.Marshal(v)
				wb, _ := json.Marshal(w)
				if string(vb) != string(wb) {
					return nil, fmt.Errorf("T3 instance %q already exists with different settings; keep it or rename it before exporting (nothing was changed)", id)
				}
				p.Reuse = append(p.Reuse, id)
				continue
			}
			keys := make([]string, 0, len(p.existing))
			for k := range p.existing {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			found := ""
			for _, k := range keys {
				if sameT3Login(p.existing[k], cfg, a, name) {
					found = k
					break
				}
			}
			if found != "" {
				p.Reuse = append(p.Reuse, found)
				continue
			}
			p.Instances[id] = instance
			p.Add = append(p.Add, id)
		}
	}
	if len(p.Add)+len(p.Reuse) == 0 {
		return nil, fmt.Errorf("no aims profiles to connect (see: aims init)")
	}
	for _, r := range Inspect(cfg, nil) {
		for _, pr := range r.Profiles {
			for _, w := range pr.Warnings {
				p.Warnings = append(p.Warnings, string(r.Tool)+"/"+pr.Name+": "+w)
			}
		}
	}
	return p, nil
}

func t3Instance(cfg *config.Config, a tool.Adapter, name string) (T3Instance, error) {
	id, l := a.ID(), a.Layout()
	p := cfg.Tools[id].Profiles[name]
	if !config.ValidName(name) {
		return T3Instance{}, fmt.Errorf("invalid %s profile name %q", id, name)
	}
	if len(p.Env) != 0 {
		return T3Instance{}, fmt.Errorf("%s/%s has custom environment settings; use aims exec or configure its T3 instance manually (profile environment and credentials are never exported)", id, name)
	}
	home := cfg.HomeEnv(id, name)
	if id == "claude" && home != "" && !filepath.IsAbs(home) {
		return T3Instance{}, fmt.Errorf("claude/%s uses a relative or tilde home; T3 expands it and would change its keychain identity; use aims exec instead", name)
	}
	bin := profiles.Bin(a)
	if bin == "" {
		bin = l.Bin
	}
	i := T3Instance{Driver: string(id), DisplayName: "aims · " + a.Title() + " / " + name, Enabled: true,
		Config: map[string]string{"binaryPath": bin, "homePath": home}}
	if id == "claude" {
		i.Driver = "claudeAgent"
		// T3 normalizes homePath and sets CLAUDE_CONFIG_DIR from it. Leave
		// it empty and pass the exact environment, preserving the macOS
		// default keychain entry and ~/.claude.json for adopted logins.
		i.Config["homePath"] = ""
	} else {
		i.Config["setupMode"], i.Config["shadowHomePath"] = "existing", ""
	}
	m := routing(cfg, a, name)
	for _, k := range l.ConflictingEnv {
		m[k] = ""
	}
	m[config.HomeVar] = config.Home()
	m[config.PinVar(id)] = name
	m[profiles.SessionVar] = string(id) + ":" + name
	m[profiles.ModeVar] = string(profiles.Unwatched)
	m[profiles.RunVar] = ""
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		i.Environment = append(i.Environment, Variable{k, m[k]})
	}
	return i, nil
}

// sameT3Login recognizes an already configured direct CLI. Shared session
// symlinks and a managed account's shared home are deliberately insufficient.
func sameT3Login(raw json.RawMessage, cfg *config.Config, a tool.Adapter, name string) bool {
	var i struct {
		Driver      string     `json:"driver"`
		Enabled     *bool      `json:"enabled"`
		Environment []Variable `json:"environment"`
		Config      struct {
			Home   string `json:"homePath"`
			Shadow string `json:"shadowHomePath"`
			Mode   string `json:"setupMode"`
		} `json:"config"`
	}
	if json.Unmarshal(raw, &i) != nil || (i.Enabled != nil && !*i.Enabled) || i.Config.Mode == "managed" || i.Config.Shadow != "" {
		return false
	}
	wantDriver := string(a.ID())
	if a.ID() == "claude" {
		wantDriver = "claudeAgent"
	}
	if i.Driver != wantDriver {
		return false
	}
	m := map[string]string{}
	for _, v := range i.Environment {
		m[v.Name] = v.Value
	}
	for _, k := range a.Layout().ConflictingEnv {
		if m[k] != "" {
			return false
		}
	}
	got := i.Config.Home
	if strings.TrimSpace(got) != "" {
		got = fsx.Abs(got)
	} else {
		// Without an explicit variable, T3 inherits its server's environment.
		// We cannot know that environment from an unrelated shell.
		var present bool
		got, present = m[a.Layout().HomeEnv]
		if !present {
			return false
		}
	}
	want := cfg.HomeEnv(a.ID(), name)
	// Claude keys credentials by the exact value, including unset vs. an
	// explicitly spelled default. Do not collapse those to the same folder.
	return got == want
}

// Merged returns a separate replacement document, retaining all original
// fields and provider instances. It never copies any auth file.
func (p *T3Plan) Merged() ([]byte, error) {
	instances := make(map[string]json.RawMessage, len(p.existing)+len(p.Instances))
	for k, v := range p.existing {
		instances[k] = v
	}
	for k, v := range p.Instances {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		instances[k] = b
	}
	b, err := json.Marshal(instances)
	if err != nil {
		return nil, err
	}
	doc := make(map[string]json.RawMessage, len(p.document)+1)
	for k, v := range p.document {
		doc[k] = v
	}
	doc["providerInstances"] = b
	out, err := json.MarshalIndent(doc, "", "  ")
	return append(out, '\n'), err
}

// WriteNew writes a private export file and refuses every existing path,
// including dangling symlinks. The live settings file is never a destination.
func WriteNew(path string, data []byte) error {
	f, err := os.OpenFile(fsx.Abs(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
