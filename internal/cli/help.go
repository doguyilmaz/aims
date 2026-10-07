package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/doguyilmaz/aims/internal/ui"
)

// installHelp replaces Cobra's help with a styled one: grouped commands,
// examples you can copy, flags in their own block.
func installHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) { renderHelp(cmd.OutOrStdout(), cmd) })
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		renderHelp(cmd.ErrOrStderr(), cmd)
		return nil
	})
}

func renderHelp(w io.Writer, cmd *cobra.Command) {
	s := ui.NewStyles(w)
	head := func(t string) string { return s.Bold.Render(t) }
	var b strings.Builder

	if !cmd.HasParent() {
		b.WriteString("\n  " + s.Title.Render("aims") + "  " + s.Dim.Render(cmd.Short) + "\n\n")
		for _, l := range strings.Split(cmd.Long, "\n") {
			b.WriteString("  " + l + "\n")
		}
	} else {
		b.WriteString("\n  " + s.Bold.Render(cmd.CommandPath()) + "  " + s.Dim.Render(cmd.Short) + "\n")
		if cmd.Long != "" {
			b.WriteString("\n")
			for _, l := range strings.Split(cmd.Long, "\n") {
				b.WriteString("  " + l + "\n")
			}
		}
	}

	if cmd.Runnable() {
		b.WriteString("\n" + head("Usage") + "\n  " + s.Cmd.Render(cmd.UseLine()) + "\n")
	}
	if len(cmd.Aliases) > 0 {
		b.WriteString("  " + s.Dim.Render("also: "+strings.Join(cmd.Aliases, ", ")) + "\n")
	}

	if cmd.Example != "" {
		b.WriteString("\n" + head("Examples") + "\n")
		for _, l := range strings.Split(strings.Trim(cmd.Example, "\n"), "\n") {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "#") {
				b.WriteString("  " + s.Dim.Render(t) + "\n")
			} else if t != "" {
				b.WriteString("  " + s.Cmd.Render(t) + "\n")
			} else {
				b.WriteString("\n")
			}
		}
	}

	subs := cmd.Commands()
	if len(subs) > 0 {
		width := 0
		for _, c := range subs {
			if c.IsAvailableCommand() {
				width = max(width, len(c.Name()))
			}
		}
		groups := cmd.Groups()
		printed := map[string]bool{}
		writeCmds := func(title, group string) {
			var lines []string
			for _, c := range subs {
				if !c.IsAvailableCommand() || c.GroupID != group {
					continue
				}
				lines = append(lines, "  "+s.Cmd.Render(ui.Pad(c.Name(), width))+"   "+c.Short)
			}
			if len(lines) > 0 {
				b.WriteString("\n" + head(title) + "\n" + strings.Join(lines, "\n") + "\n")
			}
			printed[group] = true
		}
		for _, g := range groups {
			writeCmds(g.Title, g.ID)
		}
		if !printed[""] {
			writeCmds("Commands", "")
		}
	}

	if fl := flagLines(s, cmd.LocalNonPersistentFlags()); fl != "" {
		b.WriteString("\n" + head("Flags") + "\n" + fl)
	}
	if fl := flagLines(s, cmd.InheritedFlags()); fl != "" {
		b.WriteString("\n" + head("Global flags") + "\n" + fl)
	}
	if len(subs) > 0 {
		b.WriteString("\n  " + s.Dim.Render("Run ") + s.Cmd.Render(cmd.CommandPath()+" <command> --help") + s.Dim.Render(" for details.") + "\n")
	}
	if !cmd.HasParent() {
		b.WriteString("  " + s.Dim.Render("Docs: https://doguyilmaz.github.io/aims/") + "\n")
	}
	fmt.Fprintln(w, b.String())
}

func flagLines(s *ui.Styles, fs *pflag.FlagSet) string {
	type entry struct{ name, usage string }
	var entries []entry
	width := 0
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		if t := f.Value.Type(); t != "bool" {
			name += " " + valueName(f)
		}
		usage := f.Usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" && f.DefValue != "0" {
			usage += s.Dim.Render(" (default " + f.DefValue + ")")
		}
		entries = append(entries, entry{name, usage})
		width = max(width, len(name))
	})
	var b strings.Builder
	for _, e := range entries {
		b.WriteString("  " + s.Cmd.Render(ui.Pad(e.name, width)) + "   " + e.usage + "\n")
	}
	return b.String()
}

func valueName(f *pflag.Flag) string {
	if name, _ := pflag.UnquoteUsage(f); name != "" {
		return name
	}
	return f.Value.Type()
}
