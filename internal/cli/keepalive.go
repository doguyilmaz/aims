package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/schedule"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/ui"
)

// newKeepAliveCmd is `aims keepalive`: renew logins that ran out, now, or
// every hour in the background with `on`.
func newKeepAliveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keepalive [on|off]",
		Short: "Renew logins before they lapse while an account is not used",
		Long: "A login lasts while it is used: the tool renews it on each request once its\n" +
			"short-lived access token runs out. An account left alone for days can lose\n" +
			"its login. aims keepalive renews every login whose access token has run\n" +
			"out, through the tool itself; a login that is still fresh costs nothing.\n" +
			"Codex renews without a request to the model; Claude Code has no way to, so\n" +
			"aims sends a one-word prompt to its smallest model.\n\n" +
			"on runs it every hour in the background (launchd, systemd or the Task\n" +
			"Scheduler); off stops that.",
		Example: `aims keepalive on
aims keepalive`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return switchKeepAlive(args[0])
			}
			res, err := ops.KeepAlive(cmd.Context())
			if err != nil {
				return err
			}
			for _, r := range res {
				reportKeepAlive(r)
			}
			if _, on := schedule.Installed(ops.KeepAliveName); !on {
				ui.Hint("keep them alive in the background: aims keepalive on")
			}
			return nil
		},
	}
}

func switchKeepAlive(arg string) error {
	switch arg {
	case "on":
		path, err := ops.InstallKeepAlive()
		if err != nil {
			return err
		}
		ui.Done("Logins are checked every hour and renewed when they run out (%s)", fsx.Tildify(path))
	case "off":
		path, found, err := ops.UninstallKeepAlive()
		switch {
		case err != nil:
			return err
		case !found:
			ui.Info("The keepalive was not on")
		default:
			ui.Done("Stopped the keepalive (%s removed)", fsx.Tildify(path))
		}
	default:
		return fmt.Errorf("on or off, not %q", arg)
	}
	return nil
}

func reportKeepAlive(r ops.KeepAliveResult) {
	who := string(r.Tool) + "/" + r.Profile
	switch {
	case r.LoggedIn != nil && !*r.LoggedIn:
		ui.Warn("%s: not logged in", who)
		ui.Hint("aims login %s %s", r.Tool, r.Profile)
	case r.Renewed && r.Probe.Status == tool.ProbeLogin:
		ui.Warn("%s: the login has lapsed (%s)", who, r.Probe.Detail)
		ui.Hint("aims login %s %s", r.Tool, r.Profile)
	case r.Renewed && r.Probe.Status == tool.ProbeError:
		ui.Warn("%s: could not renew the login (%s)", who, r.Probe.Detail)
	case r.Renewed:
		ui.Done("%s: renewed", who)
	case r.Expires.IsZero():
		ui.Info("%s: login age unknown, left alone", who)
	default:
		ui.Info("%s: fresh for %s", who, profiles.Duration(time.Until(r.Expires)))
	}
}
