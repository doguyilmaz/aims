package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestInstallUninstallRestoresFile(t *testing.T) {
	for _, original := range []string{"", "# mine\n", "# no newline at end", "a\n\n# >>> aims >>>\nold\n# <<< aims <<<\nb\n"} {
		home := testutil.Sandbox(t)
		rc := filepath.Join(home, ".bashrc")
		if original != "" {
			testutil.Write(t, rc, original)
		}
		if _, err := Install(Bash); err != nil {
			t.Fatal(err)
		}
		once := testutil.Read(t, rc)
		if _, err := Install(Bash); err != nil || testutil.Read(t, rc) != once {
			t.Fatalf("second install changed the file:\n%s", testutil.Read(t, rc))
		}
		if strings.Count(once, blockStart) != 1 || !Installed(Bash) {
			t.Fatalf("block:\n%s", once)
		}
		if _, found, err := Uninstall(Bash); !found || err != nil {
			t.Fatal("uninstall did not find the block")
		}
		got := testutil.Read(t, rc)
		want := original
		if strings.Contains(original, blockStart) {
			want = "a\n\nb\n"
		} else if original == "# no newline at end" {
			want += "\n" // the one change aims cannot take back
		}
		if got != want {
			t.Errorf("after uninstall:\n%q\nwant\n%q", got, want)
		}
	}
}

func TestRCFiles(t *testing.T) {
	home := testutil.Sandbox(t)
	if RCFile(Zsh) != filepath.Join(home, ".zshrc") {
		t.Fatal("zsh")
	}
	t.Setenv("ZDOTDIR", filepath.Join(home, "zdot"))
	if RCFile(Zsh) != filepath.Join(home, "zdot", ".zshrc") {
		t.Fatal("ZDOTDIR")
	}
	if RCFile(PowerShell) != "" {
		t.Fatal("PowerShell profiles are printed, not edited")
	}
	// fish gets a file of its own, removed again on uninstall.
	if _, err := Install(Fish); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Uninstall(Fish); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(RCFile(Fish)); !os.IsNotExist(err) {
		t.Fatal("aims.fish left behind")
	}
}

func TestEnvLinesQuote(t *testing.T) {
	testutil.Sandbox(t)
	c, _ := config.Load()
	c.Tools["claude"].Profiles["work"] = &config.Profile{Dir: "/tmp/it's here"}
	c.Tools["claude"].Profiles["personal"] = &config.Profile{Existing: true}
	cases := map[Shell]string{
		Bash:       `export CLAUDE_CONFIG_DIR='/tmp/it'\''s here'`,
		Fish:       `set -gx CLAUDE_CONFIG_DIR '/tmp/it\'s here'`,
		PowerShell: `$env:CLAUDE_CONFIG_DIR = '/tmp/it''s here'`,
	}
	for sh, want := range cases {
		out, err := EnvLines(sh, c, []tool.ID{"claude"}, "work", false)
		if err != nil || !strings.Contains(out, want) {
			t.Errorf("%s:\n%s\nwant %s (%v)", sh, out, want, err)
		}
	}
	out, _ := EnvLines(Bash, c, []tool.ID{"claude"}, "personal", false)
	if !strings.Contains(out, "unset CLAUDE_CONFIG_DIR") {
		t.Errorf("existing login must unset the variable:\n%s", out)
	}
	out, _ = EnvLines(Zsh, c, nil, "", true)
	if !strings.Contains(out, "unset AIMS_CLAUDE_PROFILE") || !strings.Contains(out, "unset CODEX_HOME") {
		t.Errorf("reset:\n%s", out)
	}
	if _, err := EnvLines(Bash, c, nil, "nobody", false); err == nil {
		t.Error("unknown profile accepted")
	}
}

func TestInitQuotesPath(t *testing.T) {
	out := Init(Bash, []string{"/opt/my apps/aims"})
	if !strings.Contains(out, `claude() { '/opt/my apps/aims' claude "$@"; }`) {
		t.Fatalf("bash:\n%s", out)
	}
	if out := Init(Zsh, []string{"aims"}); !strings.Contains(out, `claude() { command aims claude "$@"; }`) {
		t.Fatalf("zsh:\n%s", out)
	}
	if out := Init(PowerShell, []string{`C:\aims\aims.exe`}); !strings.Contains(out, `function claude { & 'C:\aims\aims.exe' claude @args }`) {
		t.Fatalf("powershell:\n%s", out)
	}
}
