package cli

import (
	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/ui"
)

// newMoveCmd is `aims move`: run from inside a session aims moves between
// accounts, it moves that conversation to another account.
func newMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move [profile]",
		Short: "Move the conversation of this session to another account",
		Long: "Run inside a session started with `aims claude` or `aims codex` (without\n" +
			"@profile), for example as `!aims move work` in Claude Code. aims restarts the\n" +
			"session on that account and resumes the conversation: right away when the\n" +
			"session is idle, once the current turn ends otherwise. The conversation then\n" +
			"stays on that account while it works. Without a profile, the next usable\n" +
			"account takes it.",
		Example: `aims move work
aims move`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return profileNames(""), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			to := ""
			if len(args) == 1 {
				to = args[0]
			}
			res, err := ops.Move(cmd.Context(), to)
			if err != nil {
				return err
			}
			ui.Done("%s", movedText(res))
			return nil
		},
	}
}

func movedText(res ops.MoveResult) string {
	if res.To == res.From {
		return "This conversation stays on " + res.To + ", also once the first account works again"
	}
	return "This conversation moves to " + res.To + " (now if the session is idle, once the current turn ends otherwise)"
}
