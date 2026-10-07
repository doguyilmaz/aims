// Package ui is everything aims draws: styles, the status table, the setup
// wizard and the dashboard.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Palette. Each colour has one meaning, with a light and a dark variant so
// both terminal themes read well.
var (
	accent = lipgloss.AdaptiveColor{Light: "#5B43E6", Dark: "#9D8CFF"} // aims, selection, the active account
	good   = lipgloss.AdaptiveColor{Light: "#0F8A5F", Dark: "#4ADE9B"} // ready, done
	warnC  = lipgloss.AdaptiveColor{Light: "#A35C00", Dark: "#FFB547"} // limited, attention
	bad    = lipgloss.AdaptiveColor{Light: "#C62B3B", Dark: "#FF6B7A"} // broken, needs a login
	cmdC   = lipgloss.AdaptiveColor{Light: "#0B7285", Dark: "#5FD7E8"} // something to type
	subtle = lipgloss.AdaptiveColor{Light: "#8A8794", Dark: "#6E6A7C"} // rails, secondary text
	track  = lipgloss.AdaptiveColor{Light: "#D9D6E0", Dark: "#3A3646"} // empty part of a bar
)

// Styles bound to one output (stdout or stderr), so colour follows whether
// that stream is a terminal.
type Styles struct {
	r                                *lipgloss.Renderer
	Bold, Dim, Accent, OK, Warn, Bad lipgloss.Style
	Cmd, Rail, Track, Title          lipgloss.Style
}

// NewStyles builds styles for w. Colour is off for non-terminals and when
// NO_COLOR is set.
func NewStyles(w io.Writer) *Styles {
	r := lipgloss.NewRenderer(w)
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor || !IsTerminal(w) {
		r.SetColorProfile(termenv.Ascii)
	}
	s := &Styles{r: r}
	s.Bold = r.NewStyle().Bold(true)
	s.Dim = r.NewStyle().Foreground(subtle)
	s.Accent = r.NewStyle().Foreground(accent)
	s.OK = r.NewStyle().Foreground(good)
	s.Warn = r.NewStyle().Foreground(warnC)
	s.Bad = r.NewStyle().Foreground(bad)
	s.Cmd = r.NewStyle().Foreground(cmdC)
	s.Rail = r.NewStyle().Foreground(subtle)
	s.Track = r.NewStyle().Foreground(track)
	s.Title = r.NewStyle().Bold(true).Foreground(accent)
	return s
}

// Renderer exposes the lipgloss renderer for composite styles.
func (s *Styles) Renderer() *lipgloss.Renderer { return s.r }

// Color reports whether this output gets colour.
func (s *Styles) Color() bool { return s.r.ColorProfile() != termenv.Ascii }

var (
	// Out styles stdout, Err styles stderr.
	Out = NewStyles(os.Stdout)
	Err = NewStyles(os.Stderr)
)

// IsTerminal reports whether w is a terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// Interactive reports whether aims can show prompts and full-screen views.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// Width is the terminal width, 80 when unknown.
func Width() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func message(glyph string, st lipgloss.Style, format string, args ...any) {
	prefix := st.Render(glyph)
	if !IsTerminal(os.Stderr) {
		prefix = "aims:"
	}
	fmt.Fprintln(os.Stderr, prefix+" "+fmt.Sprintf(format, args...))
}

// Done reports something that worked.
func Done(format string, args ...any) { message("✓", Err.OK, format, args...) }

// Info reports progress.
func Info(format string, args ...any) { message("›", Err.Accent, format, args...) }

// Warn reports something to look at.
func Warn(format string, args ...any) { message("!", Err.Warn, format, args...) }

// Error reports a failure.
func Error(err error) { message("✗", Err.Bad, "%s", err.Error()) }

// Hint prints a dim follow-up line to stderr.
func Hint(format string, args ...any) {
	fmt.Fprintln(os.Stderr, "  "+Err.Dim.Render(fmt.Sprintf(format, args...)))
}

// Kbd styles a command the reader can type.
func Kbd(s *Styles, cmd string) string { return s.Cmd.Render(cmd) }

// Bar draws a usage meter: filled cells coloured by how close the window is
// to its limit.
func Bar(s *Styles, pct, threshold float64, width int) string {
	pct = max(0, min(pct, 100))
	filled := int(pct/100*float64(width) + 0.5)
	if pct > 0 && filled == 0 {
		filled = 1
	}
	st := s.OK
	switch {
	case pct >= threshold:
		st = s.Bad
	case pct >= 70:
		st = s.Warn
	}
	if !s.Color() {
		return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
	}
	return st.Render(strings.Repeat("━", filled)) + s.Track.Render(strings.Repeat("━", width-filled))
}

// Pad right-pads s to w visible columns.
func Pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
