package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

// ErrCancelled is returned when the user leaves a prompt with Ctrl+C or Esc.
var ErrCancelled = errors.New("cancelled")

// Rail draws a guided flow as one vertical line: answered steps stay visible
// above the current question, each with its answer.
type Rail struct {
	w io.Writer
	s *Styles
}

// NewRail writes to stdout.
func NewRail() *Rail { return &Rail{w: os.Stdout, s: Out} }

const (
	glyphStart  = "┌"
	glyphBar    = "│"
	glyphEnd    = "└"
	glyphActive = "◆"
	glyphDone   = "◇"
	glyphOK     = "✓"
	glyphFail   = "✗"
	glyphStop   = "■"
)

func (r *Rail) line(s string) { fmt.Fprintln(r.w, s) }

func (r *Rail) bar() string { return r.s.Rail.Render(glyphBar) }

// Intro opens the rail.
func (r *Rail) Intro(title, subtitle string) {
	badge := r.s.Renderer().NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(accent).Padding(0, 1).Render(title)
	if !r.s.Color() {
		badge = "[" + title + "]"
	}
	line := r.s.Rail.Render(glyphStart) + "  " + badge
	if subtitle != "" {
		line += "  " + r.s.Dim.Render(subtitle)
	}
	r.line(line)
	r.Gap()
}

// Gap prints an empty rail segment.
func (r *Rail) Gap() { r.line(r.bar()) }

// Text prints rail lines under the current step.
func (r *Rail) Text(lines ...string) {
	for _, l := range lines {
		r.line(r.bar() + "  " + l)
	}
}

// Section prints a finished step with a heading and lines.
func (r *Rail) Section(title string, lines ...string) {
	r.line(r.s.OK.Render(glyphDone) + "  " + title)
	r.Text(lines...)
	r.Gap()
}

// Box prints lines in a rounded box on the rail.
func (r *Rail) Box(lines ...string) {
	box := r.s.Renderer().NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1).Render(strings.Join(lines, "\n"))
	for _, l := range strings.Split(box, "\n") {
		r.line(r.bar() + "  " + l)
	}
}

// OK prints a success line on the rail.
func (r *Rail) OK(text string) { r.line(r.bar() + "  " + r.s.OK.Render(glyphOK) + " " + text) }

// Fail prints a failure line on the rail.
func (r *Rail) Fail(text string) { r.line(r.bar() + "  " + r.s.Bad.Render(glyphFail) + " " + text) }

// Hint prints a dim line on the rail.
func (r *Rail) Hint(text string) { r.line(r.bar() + "  " + r.s.Dim.Render(text)) }

// Outro closes the rail.
func (r *Rail) Outro(text string) { r.line(r.s.Rail.Render(glyphEnd) + "  " + text) }

// Cancel closes the rail after Ctrl+C.
func (r *Rail) Cancel() {
	r.line(r.s.Warn.Render(glyphStop) + "  " + r.s.Dim.Render("Cancelled, nothing else was changed."))
}

func (r *Rail) theme() *huh.Theme {
	t := huh.ThemeBase()
	re := r.s.Renderer()
	ns := func() lipgloss.Style { return re.NewStyle() }
	f := &t.Focused
	f.Base = ns().Border(lipgloss.Border{Left: glyphBar}, false, false, false, true).BorderForeground(accent).PaddingLeft(2)
	f.Card = f.Base
	f.Title = ns().Bold(true)
	f.Description = ns().Foreground(subtle)
	f.ErrorIndicator = ns().Foreground(bad).SetString(" *")
	f.ErrorMessage = ns().Foreground(bad)
	f.SelectSelector = ns().Foreground(accent).SetString("● ")
	f.MultiSelectSelector = ns().Foreground(accent).SetString("› ")
	f.Option = ns()
	f.UnselectedOption = ns()
	f.SelectedOption = ns().Foreground(accent)
	f.SelectedPrefix = ns().Foreground(good).SetString("◼ ")
	f.UnselectedPrefix = ns().Foreground(subtle).SetString("◻ ")
	f.FocusedButton = ns().Bold(true).Foreground(lipgloss.Color("0")).Background(accent).Padding(0, 2).MarginRight(1)
	f.BlurredButton = ns().Foreground(subtle).Padding(0, 2).MarginRight(1)
	f.TextInput.Cursor = ns().Foreground(accent)
	f.TextInput.Placeholder = ns().Foreground(subtle)
	f.TextInput.Prompt = ns().Foreground(accent)
	f.Directory = ns().Foreground(cmdC)
	t.Blurred = t.Focused
	t.Help.ShortKey = ns().Foreground(subtle)
	t.Help.ShortDesc = ns().Foreground(subtle)
	t.Help.ShortSeparator = ns().Foreground(subtle)
	return t
}

// ask runs one huh field below an active "◆ title" line, then replaces both
// with the answered "◇ title / │ answer".
func (r *Rail) ask(title string, field huh.Field, answer func() string) error {
	r.line(r.s.Accent.Render(glyphActive) + "  " + r.s.Bold.Render(title))
	form := huh.NewForm(huh.NewGroup(field)).
		WithTheme(r.theme()).
		WithShowHelp(false).
		WithAccessible(os.Getenv("ACCESSIBLE") != "")
	err := form.Run()
	if r.s.Color() {
		fmt.Fprint(r.w, "\x1b[1A\x1b[2K") // drop the active line
	}
	if errors.Is(err, huh.ErrUserAborted) {
		r.line(r.s.Warn.Render(glyphDone) + "  " + r.s.Dim.Render(title))
		return ErrCancelled
	}
	if err != nil {
		return err
	}
	r.line(r.s.OK.Render(glyphDone) + "  " + title)
	r.line(r.bar() + "  " + r.s.Dim.Render(answer()))
	r.Gap()
	return nil
}

// Option is a choice in Select and MultiSelect.
type Option struct {
	Label string
	Value string
	Hint  string
}

func (r *Rail) options(opts []Option) []huh.Option[string] {
	out := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		label := o.Label
		if o.Hint != "" {
			label += r.s.Dim.Render("  " + o.Hint)
		}
		out[i] = huh.NewOption(label, o.Value)
	}
	return out
}

func labelOf(opts []Option, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

// Select asks for one of opts.
func (r *Rail) Select(title, description string, opts []Option, value *string) error {
	f := huh.NewSelect[string]().Options(r.options(opts)...).Value(value)
	if description != "" {
		f.Description(description)
	}
	return r.ask(title, f, func() string { return labelOf(opts, *value) })
}

// MultiSelect asks for any of opts; pre-selected values stay checked.
func (r *Rail) MultiSelect(title, description string, opts []Option, values *[]string) error {
	hopts := r.options(opts)
	for i, o := range opts {
		for _, v := range *values {
			if v == o.Value {
				hopts[i] = hopts[i].Selected(true)
			}
		}
	}
	f := huh.NewMultiSelect[string]().Options(hopts...).Value(values)
	if description != "" {
		f.Description(description)
	}
	return r.ask(title, f, func() string {
		if len(*values) == 0 {
			return "none"
		}
		labels := make([]string, len(*values))
		for i, v := range *values {
			labels[i] = labelOf(opts, v)
		}
		return strings.Join(labels, ", ")
	})
}

// Input asks for a line of text.
func (r *Rail) Input(title, description, placeholder string, value *string, validate func(string) error) error {
	f := huh.NewInput().Placeholder(placeholder).Value(value)
	if description != "" {
		f.Description(description)
	}
	if validate != nil {
		f.Validate(validate)
	}
	return r.ask(title, f, func() string { return *value })
}

// Confirm asks a yes/no question.
func (r *Rail) Confirm(title, description string, value *bool) error {
	f := huh.NewConfirm().Affirmative("Yes").Negative("No").Value(value)
	if description != "" {
		f.Description(description)
	}
	return r.ask(title, f, func() string {
		if *value {
			return "Yes"
		}
		return "No"
	})
}

// Task runs fn behind a spinner and prints its result line.
func (r *Rail) Task(label string, fn func() (string, error)) error {
	var detail string
	var err error
	run := func() { detail, err = fn() }
	if Interactive() {
		sty := r.s.Renderer().NewStyle()
		_ = spinner.New().Type(spinner.MiniDot).Style(sty.Foreground(accent)).TitleStyle(sty.Foreground(subtle)).Title(" " + label).Action(run).Run()
	} else {
		run()
	}
	text := label
	if detail != "" {
		text += r.s.Dim.Render("  " + detail)
	}
	if err != nil {
		r.Fail(text + r.s.Bad.Render("  "+err.Error()))
		return err
	}
	r.OK(text)
	return nil
}

// Spin runs fn behind a spinner on stderr (directly when not on a terminal).
func Spin(label string, fn func()) error {
	if !Interactive() {
		fn()
		return nil
	}
	sty := Err.Renderer().NewStyle()
	return spinner.New().Type(spinner.MiniDot).Output(os.Stderr).Style(sty.Foreground(accent)).TitleStyle(sty.Foreground(subtle)).Title(" " + label).Action(fn).Run()
}
