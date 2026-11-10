package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/bridge"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func newBridgeInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use: "install", Short: "Create dedicated CLI launchers for editors and SDKs",
		Long: "Creates executable links under ~/.aims/bridge/bin (or AIMS_HOME). Set\n" +
			"a client's CLI executable to the reported path once. Each new CLI\n" +
			"process uses aims' preferred account in its working directory. Account\n" +
			"files, editor settings, global PATH and installed CLIs stay untouched.\n" +
			"No downloads or duplicate CLI installations are needed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			m, err := bridge.InstallLaunchers(self)
			if err != nil {
				return err
			}
			for _, a := range tools.All() {
				if l, ok := m.Tools[a.ID()]; ok {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", a.Title(), filepath.Join(bridge.LauncherBin(), l.Name))
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Use these paths as custom CLI executables; aims bridge commands --json prints integration commands.")
			fmt.Fprintln(cmd.OutOrStdout(), "For ACP use aims bridge acp <tool>; the upstream adapter keeps its bundled CLI version.")
			return nil
		},
	}
}

func newBridgeUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use: "uninstall", Short: "Remove only the dedicated aims launchers",
		Long: "Removes aims' executable links and their manifest. Changed or unrelated\n" +
			"files are kept. Accounts, sessions, native CLIs and app settings stay.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := bridge.UninstallLaunchers(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "aims launchers removed; accounts, conversations and native CLIs stay")
			return nil
		},
	}
}

// These commands deliberately do not use launch.Run: machine protocols must
// never be supervised, buffered, retried, or have their stdin consumed by aims.
func newBridgeAgentCmd(acp bool) *cobra.Command {
	name, short := "run", "Start a native CLI without altering its protocol or lifecycle"
	if acp {
		name, short = "acp", "Start an installed ACP adapter with an aims account"
	}
	return &cobra.Command{
		Use: name + " <tool>[@profile] [--] [args...]", Short: short,
		Long: "Selects the preferred account in this working directory, or an explicit\n" +
			"@profile. Forwards standard streams and all arguments without parsing\n" +
			"agent output, consuming input, probing authentication, or retrying. No\n" +
			"settings or credentials are migrated. The client owns active sessions.\n\n" +
			"The acp command runs an already installed claude-agent-acp or codex-acp.\n" +
			"It preserves that adapter's bundled CLI and explicit executable overrides;\n" +
			"it does not implement ACP, download packages or replace pinned runtimes.",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "--help" || args[0] == "-h" {
				return cmd.Help()
			}
			word, explicit := args[0], ""
			if m := toolAt.FindStringSubmatch(word); m != nil {
				word, explicit = m[1], m[2]
			}
			a, err := parseTool(word)
			if err != nil {
				return err
			}
			nativeArgs := args[1:]
			if len(nativeArgs) > 0 && nativeArgs[0] == "--" {
				nativeArgs = nativeArgs[1:]
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			env, _, err := bridge.AgentEnvironment(cfg, a, explicit, os.Environ())
			if err != nil {
				return err
			}
			bin := ""
			m, err := bridge.ReadLaunchers(bridge.LauncherRoot())
			if err != nil {
				return err
			}
			if acp {
				bin, err = bridge.ACPExecutable(a, m)
				if err != nil {
					return err
				}
			} else {
				self, err := os.Executable()
				if err != nil {
					return err
				}
				bin, err = bridge.NativeBinary(a, bridge.LauncherRoot(), self, m)
				if err != nil {
					return err
				}
			}
			code, err := proc.Replace(bin, nativeArgs, env)
			if err != nil {
				return err
			}
			return exitCode(code)
		},
	}
}

type agentCommand struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

type agentCommands struct {
	Tool          tool.ID       `json:"tool"`
	CLI           agentCommand  `json:"cli"`
	AppServer     *agentCommand `json:"appServer,omitempty"`
	ACP           agentCommand  `json:"acp"`
	SDKExecutable string        `json:"sdkExecutable,omitempty"`
	ProfileEnv    string        `json:"profileVariable"`
}

func newBridgeCommandsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "commands", Short: "Show generic CLI, SDK, app-server and ACP integration commands",
		Long: "Prints credential-free command/argument arrays for client authors. A\n" +
			"single-executable SDK path is included after aims bridge install. Native\n" +
			"app-server and ACP are distinct protocols; choose the client's protocol.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			self, err = filepath.EvalSymlinks(self)
			if err != nil {
				return err
			}
			m, err := bridge.ReadLaunchers(bridge.LauncherRoot())
			if err != nil {
				return err
			}
			if m != nil {
				self = m.Aims
			} else if bridge.TemporaryExecutable(self) {
				return fmt.Errorf("build or install aims as a persistent executable first; integration commands cannot point at go run's temporary binary")
			}
			out := []agentCommands{}
			for _, a := range tools.All() {
				id := string(a.ID())
				env := map[string]string{config.HomeVar: config.Home()}
				c := agentCommands{Tool: a.ID(), CLI: agentCommand{self, []string{"bridge", "run", id, "--"}, env},
					ACP: agentCommand{self, []string{"bridge", "acp", id, "--"}, env}, ProfileEnv: config.PinVar(a.ID())}
				if m != nil {
					if l, ok := m.Tools[a.ID()]; ok {
						c.SDKExecutable = filepath.Join(bridge.LauncherBin(), l.Name)
						c.CLI = agentCommand{c.SDKExecutable, []string{}, env}
					}
				}
				if id == "codex" {
					app := c.CLI
					app.Args = append(append([]string{}, app.Args...), "app-server")
					c.AppServer = &app
				}
				out = append(out, c)
			}
			if asJSON {
				return writeJSON(cmd, struct {
					Version string          `json:"schemaVersion"`
					Agents  []agentCommands `json:"agents"`
				}{"1.0.0", out})
			}
			for _, c := range out {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n  CLI: %s %s\n  ACP: %s %s\n", c.Tool, c.CLI.Command, strings.Join(c.CLI.Args, " "), c.ACP.Command, strings.Join(c.ACP.Args, " "))
				if c.SDKExecutable != "" {
					fmt.Fprintln(cmd.OutOrStdout(), "  SDK executable: "+c.SDKExecutable)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Use --json for argument arrays; no editor settings or credential stores are changed.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print command arrays and optional executable paths without secrets")
	return cmd
}
