// Package shell writes what aims puts into shells: `aims env` exports, the
// `aims shell-init` wrappers and the block in the user's rc file.
package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Shell is a supported shell.
type Shell string

const (
	Bash       Shell = "bash"
	Zsh        Shell = "zsh"
	Fish       Shell = "fish"
	PowerShell Shell = "powershell"
)

// All lists the supported shells.
var All = []Shell{Bash, Zsh, Fish, PowerShell}

// Parse validates a shell name.
func Parse(s string) (Shell, bool) {
	for _, sh := range All {
		if string(sh) == s {
			return sh, true
		}
	}
	return "", false
}

// Detect guesses the user's shell from $SHELL.
func Detect() Shell {
	if runtime.GOOS == "windows" && os.Getenv("SHELL") == "" {
		return PowerShell
	}
	if sh, ok := Parse(filepath.Base(os.Getenv("SHELL"))); ok {
		return sh
	}
	return Bash
}

func quote(sh Shell, v string) string {
	switch sh {
	case PowerShell:
		// PowerShell also ends a single-quoted string at the typographic quotes.
		return "'" + psQuotes.Replace(v) + "'"
	case Fish:
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

var psQuotes = strings.NewReplacer("'", "''", "\u2018", "\u2018\u2018", "\u2019", "\u2019\u2019", "\u201a", "\u201a\u201a", "\u201b", "\u201b\u201b")

func set(sh Shell, k, v string) string {
	switch sh {
	case PowerShell:
		return fmt.Sprintf("$env:%s = %s", k, quote(sh, v))
	case Fish:
		return fmt.Sprintf("set -gx %s %s", k, quote(sh, v))
	}
	return fmt.Sprintf("export %s=%s", k, quote(sh, v))
}

func unset(sh Shell, k string) string {
	switch sh {
	case PowerShell:
		return fmt.Sprintf("Remove-Item Env:%s -ErrorAction SilentlyContinue", k)
	case Fish:
		return "set -e " + k
	}
	return "unset " + k
}

// EnvLines pins this terminal to profile (every tool that has it when ids is
// empty). It also sets CLAUDE_CONFIG_DIR / CODEX_HOME, so even the bare
// binaries use the right login. reset undoes the pin.
func EnvLines(sh Shell, cfg *config.Config, ids []tool.ID, profile string, reset bool) (string, error) {
	if len(ids) == 0 {
		for _, id := range tools.IDs() {
			if profile == "" || cfg.Tools[id].Profiles[profile] != nil {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return "", fmt.Errorf("no profile named %q (see: aims status)", profile)
		}
	}
	var lines []string
	for _, id := range ids {
		a := tools.Get(id)
		homeVar := a.Layout().HomeEnv
		if reset {
			lines = append(lines, unset(sh, config.PinVar(id)))
			if hub := cfg.Tools[id].HubEnv; hub != "" {
				lines = append(lines, set(sh, homeVar, hub))
			} else {
				lines = append(lines, unset(sh, homeVar))
			}
			continue
		}
		name := profile
		if name == "" {
			name, _ = cfg.Preferred(id, "")
		}
		if name == "" || cfg.Tools[id].Profiles[name] == nil {
			if len(ids) == 1 {
				return "", fmt.Errorf("no %s profile %q (see: aims status)", id, profile)
			}
			continue
		}
		lines = append(lines, set(sh, config.PinVar(id), name))
		if v := cfg.HomeEnv(id, name); v != "" {
			lines = append(lines, set(sh, homeVar, v))
		} else {
			lines = append(lines, unset(sh, homeVar))
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// Init is the script `eval "$(aims shell-init)"` loads: wrappers so plain
// `claude` and `codex` follow the active profile, and completion for aims.
func Init(sh Shell, aims []string) string {
	onPath := len(aims) == 1 && aims[0] == "aims"
	var b strings.Builder
	fmt.Fprintf(&b, "# aims shell integration (%s)\n", sh)
	inv := func(prefix string) string {
		if onPath {
			return prefix + "aims"
		}
		parts := make([]string, len(aims))
		for i, p := range aims {
			parts[i] = quote(sh, p)
		}
		if sh == PowerShell {
			return "& " + strings.Join(parts, " ")
		}
		return strings.Join(parts, " ")
	}
	for _, id := range tools.IDs() {
		switch sh {
		case PowerShell:
			fmt.Fprintf(&b, "function %s { %s %s @args }\n", id, inv(""), id)
		case Fish:
			fmt.Fprintf(&b, "function %s --wraps %s; %s %s $argv; end\n", id, id, inv("command "), id)
		default:
			fmt.Fprintf(&b, "%s() { %s %s \"$@\"; }\n", id, inv("command "), id)
		}
	}
	switch sh {
	case Zsh:
		fmt.Fprintf(&b, "(( $+functions[compdef] )) && source <(%s completion zsh)\n", inv(""))
	case Bash:
		fmt.Fprintf(&b, "source <(%s completion bash)\n", inv(""))
	case Fish:
		fmt.Fprintf(&b, "%s completion fish | source\n", inv(""))
	case PowerShell:
		fmt.Fprintf(&b, "%s completion powershell | Out-String | Invoke-Expression\n", inv(""))
	}
	return b.String()
}

const (
	blockStart = "# >>> aims >>>"
	blockEnd   = "# <<< aims <<<"
)

// RCFile is where the shell-init line goes for sh.
func RCFile(sh Shell) string {
	h := fsx.Home()
	switch sh {
	case Zsh:
		if d := os.Getenv("ZDOTDIR"); d != "" {
			return filepath.Join(d, ".zshrc")
		}
		return filepath.Join(h, ".zshrc")
	case Bash:
		return filepath.Join(h, ".bashrc")
	case Fish:
		return filepath.Join(h, ".config", "fish", "conf.d", "aims.fish")
	}
	return "" // PowerShell profiles vary; aims prints the line instead
}

// initLine is guarded so a shell still starts cleanly after aims is removed.
func initLine(sh Shell) string {
	if sh == Fish {
		return "command -q aims; and aims shell-init fish | source"
	}
	return fmt.Sprintf(`command -v aims >/dev/null 2>&1 && eval "$(aims shell-init %s)"`, sh)
}

// Install adds (or refreshes) the aims block in the shell's rc file.
func Install(sh Shell) (string, error) {
	path := RCFile(sh)
	if path == "" {
		return "", fmt.Errorf("add this to your PowerShell profile: aims shell-init powershell | Out-String | Invoke-Expression")
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, err
	}
	block := blockStart + "\n" + initLine(sh) + "\n" + blockEnd + "\n"
	updated, _ := replaceBlock(string(old), block)
	if updated == string(old) {
		return path, nil
	}
	return path, fsx.WriteFile(path, []byte(updated), 0o644)
}

// Uninstall removes the aims block from the shell's rc file.
func Uninstall(sh Shell) (string, bool, error) {
	path := RCFile(sh)
	if path == "" {
		return "", false, nil
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return path, false, nil
	}
	updated, found := replaceBlock(string(old), "")
	if !found {
		return path, false, nil
	}
	if sh == Fish && strings.TrimSpace(updated) == "" {
		return path, true, os.Remove(path) // aims.fish is ours alone
	}
	return path, true, fsx.WriteFile(path, []byte(updated), 0o644)
}

// Installed reports whether the rc file already has the aims block.
func Installed(sh Shell) bool {
	b, err := os.ReadFile(RCFile(sh))
	return err == nil && strings.Contains(string(b), blockStart)
}

// replaceBlock swaps the marked block for block ("" removes it), or appends
// block when there is none.
func replaceBlock(s, block string) (string, bool) {
	start := strings.Index(s, blockStart)
	if start >= 0 {
		if end := strings.Index(s[start:], blockEnd); end >= 0 {
			end += start + len(blockEnd)
			if end < len(s) && s[end] == '\n' {
				end++
			}
			head := s[:start]
			if block == "" && end == len(s) && strings.HasSuffix(head, "\n\n") {
				head = head[:len(head)-1] // the blank line Install put before it
			}
			return head + block + s[end:], true
		}
	}
	if block == "" {
		return s, false
	}
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if s != "" {
		s += "\n"
	}
	return s + block, false
}
