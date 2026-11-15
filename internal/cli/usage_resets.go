package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/doguyilmaz/aims/internal/usage"
	"github.com/spf13/cobra"
)

func newUsageResetCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "reset", Short: "Manage custom reset reminders (never changes provider quota)"}
	var at, label string
	var asJSON bool
	add := &cobra.Command{Use: "add <provider> <row-id>", Short: "Add a user-defined reset reminder", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return fmt.Errorf("--at must include a timezone, for example 2026-12-01T09:00:00+03:00")
		}
		if err = usage.AddCustom(args[0], args[1], label, t); err != nil {
			return err
		}
		return printUsageResetResult(cmd, asJSON)
	}}
	add.Flags().StringVar(&at, "at", "", "reminder time in RFC 3339 with timezone")
	add.Flags().StringVar(&label, "label", "Custom reset", "reminder label")
	add.Flags().BoolVar(&asJSON, "json", false, "print updated usage snapshot")
	_ = add.MarkFlagRequired("at")
	var removeJSON bool
	remove := &cobra.Command{Use: "remove <id>", Short: "Remove a custom reminder", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := usage.RemoveCustom(args[0]); err != nil {
			return err
		}
		return printUsageResetResult(cmd, removeJSON)
	}}
	remove.Flags().BoolVar(&removeJSON, "json", false, "print updated usage snapshot")
	cmd.AddCommand(add, remove)
	return cmd
}

func printUsageResetResult(cmd *cobra.Command, asJSON bool) error {
	s, err := usage.Load("")
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(s)
	}
	for _, r := range s.CustomResets {
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %s/%s  %s  %s (user reminder)\n", r.ID, r.Provider, r.Row, r.At.Local().Format(time.RFC3339), r.Label)
	}
	return nil
}
