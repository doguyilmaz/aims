package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func entry(dir, level string) string {
	return "[projects." + strconv.Quote(dir) + "]\ntrust_level = \"" + level + "\"\n"
}

func trusts(dir string) []string {
	return []string{"-c", "projects={" + strconv.Quote(dir) + "={trust_level=\"trusted\"}}"}
}

func TestTrustArgs(t *testing.T) {
	root := fsx.Real(t.TempDir())
	repo := filepath.Join(root, "repo")
	sub := filepath.Join(repo, "sub")
	plain := filepath.Join(root, "plain")
	worktree := filepath.Join(root, "worktree")
	settings := filepath.Join(root, "settings")
	settingsAtTop := filepath.Join(root, "settings-at-top")
	mkdir(t, sub, plain, filepath.Join(settings, "sub", ".codex"), filepath.Join(settingsAtTop, ".codex"), filepath.Join(settingsAtTop, "sub"), filepath.Join(root, ".codex"))
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "x")
	git(t, repo, "worktree", "add", "-q", worktree)
	git(t, settings, "init", "-q")
	git(t, settingsAtTop, "init", "-q")

	cases := []struct {
		name, dir, config string
		args, want        []string
	}{
		{"repository with no entry", repo, "", nil, trusts(repo)},
		{"subfolder", sub, "", nil, trusts(sub)},
		{"worktree", worktree, "", nil, trusts(worktree)},
		{"Codex does not ask outside git", plain, "", nil, nil},
		{"trusted parent does not count", repo, entry(root, "trusted"), nil, trusts(repo)},
		{"settings above the repository do not count", repo, "", nil, trusts(repo)},
		{"other values naming the folder do not count", repo, "[sandbox_workspace_write]\nwritable_roots = [" + strconv.Quote(repo) + "]\n[mcp_servers.x]\ncwd = " + strconv.Quote(repo) + "\n", nil, trusts(repo)},
		{"already trusted", repo, entry(repo, "trusted"), nil, nil},
		{"untrusted stays", repo, entry(repo, "untrusted"), nil, nil},
		{"untrusted repository covers its subfolder", sub, entry(repo, "untrusted"), nil, nil},
		{"untrusted main worktree covers a worktree", worktree, entry(repo, "untrusted"), nil, nil},
		{"literal key", repo, "[projects.'" + repo + "']\n", nil, nil},
		{"dotted key", repo, "projects." + strconv.Quote(repo) + ".trust_level = \"untrusted\"\n", nil, nil},
		{"inline table", repo, "projects = { " + strconv.Quote(repo) + " = { trust_level = \"untrusted\" } }\n", nil, nil},
		{"folder settings stay the user's question", filepath.Join(settings, "sub"), "", nil, nil},
		{"folder settings at the top count from a subfolder", filepath.Join(settingsAtTop, "sub"), "", nil, nil},
		{"-C names the folder", plain, "", []string{"-C", repo, "resume", "S"}, trusts(repo)},
		{"--cd= names the folder", plain, "", []string{"--cd=" + repo}, trusts(repo)},
		{"-C attached", plain, "", []string{"-C" + repo}, trusts(repo)},
		{"relative --cd", root, "", []string{"--cd", "repo"}, trusts(repo)},
		{"-C elsewhere", repo, "", []string{"-C", plain}, nil},
		{"a prompt is not an option", plain, "", []string{"resume", "S", "--", "-C", repo}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "config.toml")
			if c.config != "" {
				if err := os.WriteFile(path, []byte(c.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := New().TrustArgs(tool.Home{Dir: home}, c.dir, c.args)
			if err != nil || !slices.Equal(got, c.want) {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
			if b, _ := os.ReadFile(path); string(b) != c.config {
				t.Fatalf("config.toml changed: %s", b)
			}
		})
	}
}

func TestTrustArgsLeavesFolderSettingsToTheUser(t *testing.T) {
	for _, name := range folderSettings {
		t.Run(name, func(t *testing.T) {
			repo := fsx.Real(t.TempDir())
			git(t, repo, "init", "-q")
			if err := os.Symlink(filepath.Join(repo, "elsewhere"), filepath.Join(repo, name)); err != nil {
				t.Fatal(err)
			}
			if got, err := New().TrustArgs(tool.Home{Dir: t.TempDir()}, repo, nil); got != nil || err != nil {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
