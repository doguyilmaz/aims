package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/release"
	"github.com/doguyilmaz/aims/internal/ui"
)

func newUpgradeCmd(version string) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Upgrade aims with the tool that installed it",
		Long: "aims never overwrites its own binary. It finds out how it was installed\n" +
			"(Homebrew, npm, go install or the install script) and runs that tool's upgrade.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			latest, err := release.Default(release.CacheDir()).Latest(cmd.Context(), true)
			switch {
			case err != nil:
				ui.Warn("could not check for a new release: %v", err)
			case release.Compare(version, latest) >= 0 && version != "dev":
				ui.Done("aims %s is the latest release", version)
				return nil
			default:
				ui.Info("aims %s is available (you have %s)", latest, version)
			}
			exe, _ := os.Executable()
			exe, _ = filepath.EvalSymlinks(exe)
			method := release.Detect(exe, goBin())
			steps := release.UpgradeCommand(method)
			if len(steps) == 0 {
				ui.Info("this copy was installed by hand; get the new one with:")
				fmt.Println("  " + ui.Kbd(ui.Out, release.InstallLine))
				return nil
			}
			var lines []string
			for _, s := range steps {
				lines = append(lines, strings.Join(s, " "))
			}
			if check {
				fmt.Println("  " + ui.Kbd(ui.Out, strings.Join(lines, " && ")))
				return nil
			}
			for i, s := range steps {
				ui.Info("%s", strings.Join(s, " "))
				c := exec.CommandContext(cmd.Context(), s[0], s[1:]...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					// `brew update` refreshes every tap; one broken tap must not block the upgrade.
					if method == release.Homebrew && i == 0 {
						ui.Warn("brew update failed (%v); upgrading anyway", err)
						continue
					}
					return err
				}
			}
			ui.Done("upgraded")
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only show what would run")
	return cmd
}

func goBin() string {
	if v := os.Getenv("GOBIN"); v != "" {
		return v
	}
	if v := os.Getenv("GOPATH"); v != "" {
		return filepath.Join(strings.Split(v, string(os.PathListSeparator))[0], "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "go", "bin")
}

// updateNotice checks for a new release while a command runs and mentions it
// afterwards, at most once a day, on terminals only.
type updateNotice struct{ done chan string }

func startUpdateCheck(ctx context.Context, version string, args []string) *updateNotice {
	if version == "dev" || os.Getenv("AIMS_NO_UPDATE_CHECK") != "" || !ui.IsTerminal(os.Stderr) {
		return nil
	}
	if len(args) > 0 {
		switch args[0] {
		case "mcp", "statusline", "shell-init", "env", "completion", "__complete", "__completeNoDesc", "upgrade", "update", "claude", "codex", "run":
			return nil
		}
	}
	n := &updateNotice{done: make(chan string, 1)}
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		latest, _ := release.Default(release.CacheDir()).Latest(ctx, false)
		n.done <- latest
	}()
	return n
}

func (n *updateNotice) finish(version string) {
	if n == nil {
		return
	}
	select {
	case latest := <-n.done:
		if latest != "" && release.Compare(version, latest) < 0 {
			fmt.Fprintln(os.Stderr)
			ui.Info("aims %s is available (you have %s). Run %s", latest, version, ui.Kbd(ui.Err, "aims upgrade"))
		}
	case <-time.After(150 * time.Millisecond):
		// Never hold a finished command up for a version check.
	}
}
