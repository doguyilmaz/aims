package cli

import (
	"fmt"
	"strings"

	"github.com/doguyilmaz/aims/internal/desktop"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ui"
	"github.com/spf13/cobra"
)

func newAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "app", Short: "Install or open the macOS menu bar and Edge Dock",
		Long: "Aims Usage includes the menu bar, optional Edge Dock and pinned overview.\nIt bundles aims; an existing standalone CLI keeps its own installer.\nThe public cask is separate from aims. Source previews can use install --from.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runAppMenu(cmd) },
	}
	var from string
	install := &cobra.Command{Use: "install", Short: "Install with Homebrew or an explicitly selected local app", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return installApp(cmd, from) }}
	install.Flags().StringVar(&from, "from", "", "copy a signature-verified local Aims Usage.app to ~/Applications; never replace an existing app")
	var asJSON bool
	status := &cobra.Command{Use: "status", Short: "Show app and terminal command installation", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return printAppStatus(cmd, desktop.Inspect(), asJSON) }}
	status.Flags().BoolVar(&asJSON, "json", false, "machine-readable installation status")
	var linkJSON bool
	link := &cobra.Command{Use: "link-cli", Short: "Expose the app's bundled CLI in ~/.local/bin when no CLI exists", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := desktop.LinkCLI("")
			if err != nil {
				return err
			}
			if linkJSON {
				return writeJSON(cmd, struct {
					Path string `json:"path"`
				}{path})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Terminal command: "+fsx.Tildify(path))
			fmt.Fprintln(cmd.OutOrStdout(), "Existing commands are retained. If ~/.local/bin is not on your shell PATH, add that folder to PATH before using aims by name.")
			return nil
		}}
	link.Flags().BoolVar(&linkJSON, "json", false, "return the terminal command path")
	cmd.AddCommand(install, status, &cobra.Command{Use: "open", Short: "Open the installed Aims Usage app", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return desktop.Open(cmd.Context()) }}, link)
	return cmd
}

func runAppMenu(cmd *cobra.Command) error {
	s := desktop.Inspect()
	if !ui.Interactive() {
		return printAppStatus(cmd, s, false)
	}
	choice := "install"
	opts := []ui.Option{{Label: "Install Aims Usage", Value: "install", Hint: "public Homebrew cask; retains your existing CLI"}, {Label: "Install a local app…", Value: "local", Hint: "source preview or a downloaded bundle"}}
	if s.Installed {
		choice = "open"
		opts = []ui.Option{{Label: "Open Aims Usage", Value: "open"}, {Label: "Show installation", Value: "status"}}
	}
	opts = append(opts, ui.Option{Label: "Back", Value: "back"})
	if err := ui.NewRail().Select("macOS menu bar and Edge Dock", "Optional; accounts and history stay where they are.", opts, &choice); err != nil {
		return err
	}
	switch choice {
	case "local":
		path := "dist/Aims Usage.app"
		if err := ui.NewRail().Input("Aims Usage.app path", "The signature is verified; existing apps are kept.", path, &path, func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("choose an Aims Usage.app path")
			}
			return nil
		}); err != nil {
			return err
		}
		return installApp(cmd, fsx.ExpandHome(path))
	case "install":
		return installApp(cmd, "")
	case "open":
		return desktop.Open(cmd.Context())
	case "status":
		return printAppStatus(cmd, s, false)
	}
	return nil
}

func installApp(cmd *cobra.Command, from string) error {
	if status := desktop.Inspect(); from == "" && status.Installed {
		return printAppStatus(cmd, status, false)
	}
	if err := desktop.Install(cmd.Context(), from, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return err
	}
	if from != "" && desktop.Inspect().CLIPath == "" {
		path, err := desktop.LinkCLI("")
		if err != nil {
			return fmt.Errorf("app installed; terminal command was kept: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Terminal command: "+fsx.Tildify(path)+" (add ~/.local/bin to your shell PATH if needed)")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Aims Usage installed. Open with: aims app open")
	return nil
}

func printAppStatus(cmd *cobra.Command, s desktop.Status, asJSON bool) error {
	if asJSON {
		return writeJSON(cmd, s)
	}
	if s.Installed {
		fmt.Fprintf(cmd.OutOrStdout(), "Aims Usage %s: %s\n", s.Version, fsx.Tildify(s.Path))
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Aims Usage is optional and not installed. Install with: aims app install")
	}
	if s.CLIPath != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Terminal CLI: "+fsx.Tildify(s.CLIPath))
	}
	return nil
}

func offerDesktopApp(cmd *cobra.Command) error {
	if !desktop.Supported() || desktop.Inspect().Installed {
		return nil
	}
	install := false
	if err := ui.NewRail().Confirm("Add the macOS menu bar and Edge Dock?", "Optional Homebrew app. You can also install later with aims app install.", &install); err != nil {
		return err
	}
	if install {
		return runAppMenu(cmd)
	}
	return nil
}

// appDoctorCheck is informational even when an optional installation is absent.
func appDoctorCheck() doctorCheck {
	return desktopDoctorCheck(desktop.Inspect())
}

func desktopDoctorCheck(s desktop.Status) doctorCheck {
	if s.Installed {
		return doctorCheck{"macos-app", "ok", "Aims Usage " + s.Version + ": " + fsx.Tildify(s.Path), "aims app open"}
	}
	return doctorCheck{"macos-app", "info", "optional macOS menu bar and Edge Dock are not installed", "aims app install"}
}
