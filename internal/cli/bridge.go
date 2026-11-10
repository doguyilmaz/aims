package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/bridge"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newBridgeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Inspect shared homes and connect other coding apps safely",
		Long: "Shows the calling process' homes, aims' profiles, and the actual shared\n" +
			"transcript paths. Does not read credentials, run a provider, repair files,\n" +
			"or change settings. Shared history and a shared login are separate things.",
		Example: "aims bridge\naims bridge --json\naims bridge t3",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			reports := bridge.Inspect(cfg, os.Environ())
			if asJSON {
				return writeJSON(cmd, reports)
			}
			out := cmd.OutOrStdout()
			for _, r := range reports {
				fmt.Fprintf(out, "%s\n  hub: %s\n  this process: %s\n", r.Tool, fsx.Tildify(r.Hub), fsx.Tildify(r.CurrentHome))
				fmt.Fprintf(out, "  transcripts: %s (shared with hub: %t)\n", fsx.Tildify(r.CurrentHistory.Real), r.CurrentHistory.Shared)
				for _, w := range r.Warnings {
					fmt.Fprintln(out, "  ! "+w)
				}
				if len(r.ConflictingEnv) > 0 {
					fmt.Fprintln(out, "  ! login overrides in this process: "+strings.Join(r.ConflictingEnv, ", "))
				}
				for _, p := range r.Profiles {
					fmt.Fprintf(out, "  %s: %s (share: %s, transcripts shared: %t)\n", p.Name, fsx.Tildify(p.Home), p.Share, p.History.Shared)
					for _, w := range p.Warnings {
						fmt.Fprintln(out, "    ! "+w)
					}
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, "Use aims exec -- <program> for apps that inherit CLI homes.")
			fmt.Fprintln(out, "Use aims bridge t3 for T3 Code's existing-CLI connections and history import guidance.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print paths and diagnostics as JSON (no credentials)")
	cmd.AddCommand(newBridgeT3Cmd())
	return cmd
}

func newBridgeT3Cmd() *cobra.Command {
	var settings, output string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "t3",
		Short: "Plan T3 Code connections to aims' existing CLI accounts",
		Long: "Builds an additive plan for T3 Code's existing-CLI providers. Existing\n" +
			"providers and all other settings are preserved. Managed T3 logins stay\n" +
			"separate; aims' credential files are never read or copied.\n\n" +
			"By default this only previews the plan. --json prints the new entries.\n" +
			"--output writes merged settings to a NEW private file and refuses to\n" +
			"overwrite any file or write to live settings. Review it, close T3 fully,\n" +
			"and back up its settings before installing it. Never replace settings\n" +
			"while T3 is running; regenerate the export if its settings have changed.",
		Example: "aims bridge t3\naims bridge t3 --json\naims bridge t3 --output ./t3-settings.aims.json\naims bridge t3 --settings /path/to/settings.json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output != "" && asJSON {
				return fmt.Errorf("choose --json or --output")
			}
			if settings == "" {
				settings = bridge.T3SettingsPath()
			}
			if output != "" && (fsx.Real(output) == fsx.Real(settings) || fsx.Real(output) == fsx.Real(bridge.T3SettingsPath())) {
				return fmt.Errorf("--output must be a new export file, never live T3 settings")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			plan, err := bridge.PlanT3(cfg, settings)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd, plan)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "T3 settings: "+fsx.Tildify(plan.Settings))
			for _, id := range plan.Add {
				fmt.Fprintln(out, "  add: "+plan.Instances[id].DisplayName+" (existing CLI login)")
			}
			for _, id := range plan.Reuse {
				fmt.Fprintln(out, "  keep existing connection: "+id)
			}
			for _, w := range plan.Warnings {
				fmt.Fprintln(out, "  ! "+w)
			}
			if output != "" {
				b, err := plan.Merged()
				if err != nil {
					return err
				}
				if err := bridge.WriteNew(output, b); err != nil {
					return err
				}
				fmt.Fprintln(out, "Wrote separate export: "+fsx.Tildify(fsx.Abs(output)))
				fmt.Fprintln(out, "Keep the export private: it retains any secrets already present in T3 settings.")
				fmt.Fprintln(out, "Review it, close T3 fully, and back up its settings before installing it.")
			} else {
				fmt.Fprintln(out, "Preview only. Write a separate merged file with --output <new-file>.")
			}
			fmt.Fprintln(out, "Managed T3 accounts and running sessions have not been changed.")
			fmt.Fprintln(out, "In T3, select an existing-CLI connection and use its session/project import to bring CLI conversations into the thread list.")
			fmt.Fprintln(out, "Shared transcript files do not automatically create T3 threads. T3 owns its thread list and import limits.")
			return nil
		},
	}
	cmd.Flags().StringVar(&settings, "settings", "", "T3 server settings file (default: $T3CODE_HOME/userdata/settings.json or ~/.t3/userdata/settings.json)")
	cmd.Flags().StringVar(&output, "output", "", "write merged settings to a NEW private export file")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the additive plan as JSON; existing settings and secrets are omitted")
	return cmd
}

func newExecCmd() *cobra.Command {
	requested := map[tool.ID]*string{}
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "exec [--claude profile] [--codex profile] [--dry-run] -- <program> [args...]",
		Short: "Run another application with aims' preferred CLI accounts",
		Long: "Passes the preferred accounts' CLI homes and shared database/memory\n" +
			"paths to a child program. Explicit flags pin an account. Shell API keys\n" +
			"that override those logins are dropped, just as for aims claude/codex.\n\n" +
			"The child keeps its own session lifecycle and account switching; aims\n" +
			"does not restart it. Apps with their own provider settings (such as T3\n" +
			"managed accounts) can override the environment. Already running desktop\n" +
			"apps do not acquire a new environment. No files or logins are changed\n" +
			"by aims; the child program may change them.",
		Example: "aims exec -- my-orchestrator\naims exec --claude work --codex personal -- my-orchestrator --help\naims exec --dry-run -- my-orchestrator",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			pins := map[tool.ID]string{}
			for id, value := range requested {
				if cmd.Flags().Changed(string(id)) {
					if *value == "" {
						return fmt.Errorf("--%s needs a profile name", id)
					}
					pins[id] = *value
				}
			}
			env, selected, err := bridge.Environment(cfg, pins, os.Environ())
			if err != nil {
				return err
			}
			if dryRun {
				// Do not print the complete environment or command arguments:
				// either may contain a token supplied by the user.
				return writeJSON(cmd, selected)
			}
			bin := args[0]
			if filepath.Base(bin) == "aims" {
				return fmt.Errorf("aims exec is for another application; use aims claude or aims codex directly")
			}
			code, err := proc.RunAttached(bin, args[1:], env, "")
			if err != nil {
				return err
			}
			return exitCode(code)
		},
	}
	// Provider/application arguments, including --help and --version, belong to
	// the child after its name; they must never be interpreted as aims flags.
	cmd.Flags().SetInterspersed(false)
	for _, a := range tools.All() {
		requested[a.ID()] = cmd.Flags().String(string(a.ID()), "", "pin the child's "+a.Title()+" account")
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print selected profiles without starting the program (no secrets)")
	return cmd
}
