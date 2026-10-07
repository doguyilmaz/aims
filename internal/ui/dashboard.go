package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Launch is what the dashboard asks the caller to do after it closes.
type Launch struct {
	Tool    tool.ID
	Profile string
}

type keymap struct {
	Up, Down, Use, Start, Login, Failover, Clear, Refresh, Add, Help, Quit key.Binding
}

func (k keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.Use, k.Start, k.Login, k.Failover, k.Refresh, k.Help, k.Quit}
}

func (k keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down}, {k.Use, k.Start, k.Add}, {k.Login, k.Failover, k.Clear}, {k.Refresh, k.Help, k.Quit}}
}

var keys = keymap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Use:      key.NewBinding(key.WithKeys("enter", "u"), key.WithHelp("enter", "make active")),
	Start:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
	Login:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "log in")),
	Failover: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fail over")),
	Clear:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear marks")),
	Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "check live")),
	Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add account")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
	Quit:     key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
}

type row struct {
	tool    int
	profile int
}

type dashboard struct {
	ctx       context.Context
	version   string
	threshold float64
	report    []ops.ToolStatus
	rows      []row
	cursor    int
	width     int
	height    int
	spin      spinner.Model
	help      help.Model
	checking  map[string]bool
	flash     string
	flashBad  bool
	err       error
	launch    *Launch
	self      string
	// confirm is the account `f` would mark, waiting for y.
	confirm *selection
}

type reportMsg struct {
	report []ops.ToolStatus
	err    error
}

type flashMsg struct {
	text string
	bad  bool
}

type liveDoneMsg struct{ key string }

type execDoneMsg struct{ err error }

// RunDashboard shows the dashboard. It returns a launch request when the user
// picked "start".
func RunDashboard(ctx context.Context, version string) (*Launch, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	report, err := ops.Status(ctx, nil)
	if err != nil {
		return nil, err
	}
	self, _ := os.Executable()
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(Out.Accent))
	h := help.New()
	h.Styles.ShortKey, h.Styles.FullKey = Out.Cmd, Out.Cmd
	h.Styles.ShortDesc, h.Styles.FullDesc = Out.Dim, Out.Dim
	h.Styles.ShortSeparator, h.Styles.FullSeparator = Out.Dim, Out.Dim
	m := &dashboard{
		ctx: ctx, version: version, threshold: cfg.Failover.Threshold,
		spin: sp, help: h, checking: map[string]bool{}, self: self,
	}
	m.setReport(report)
	final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil {
		return nil, err
	}
	return final.(*dashboard).launch, nil
}

func (m *dashboard) setReport(r []ops.ToolStatus) {
	var sel string
	if p, ok := m.selected(); ok {
		sel = string(p.toolID) + "/" + p.name
	}
	m.report = r
	m.rows = m.rows[:0]
	for ti, t := range r {
		for pi := range t.Profiles {
			m.rows = append(m.rows, row{ti, pi})
		}
	}
	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	for i, rw := range m.rows {
		if string(r[rw.tool].ID)+"/"+r[rw.tool].Profiles[rw.profile].Name == sel {
			m.cursor = i
		}
	}
}

type selection struct {
	toolID tool.ID
	name   string
	p      ops.ProfileStatus
	t      ops.ToolStatus
}

func (m *dashboard) selected() (selection, bool) {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return selection{}, false
	}
	rw := m.rows[m.cursor]
	t := m.report[rw.tool]
	p := t.Profiles[rw.profile]
	return selection{t.ID, p.Name, p, t}, true
}

func (m *dashboard) reload() tea.Cmd {
	return func() tea.Msg {
		r, err := ops.Status(m.ctx, nil)
		return reportMsg{r, err}
	}
}

func (m *dashboard) Init() tea.Cmd { return m.spin.Tick }

func (m *dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width - 1
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case reportMsg:
		if msg.err != nil {
			m.flash, m.flashBad = msg.err.Error(), true
		} else {
			m.setReport(msg.report)
		}
	case flashMsg:
		m.flash, m.flashBad = msg.text, msg.bad
		return m, m.reload()
	case liveDoneMsg:
		delete(m.checking, msg.key)
		return m, m.reload()
	case execDoneMsg:
		if msg.err != nil {
			m.flash, m.flashBad = msg.err.Error(), true
		}
		return m, m.reload()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *dashboard) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if c := m.confirm; c != nil {
		m.confirm = nil
		if msg.String() != "y" {
			m.flash, m.flashBad = "Nothing marked", false
			return m, nil
		}
		return m, func() tea.Msg {
			res, err := ops.Failover(m.ctx, tools.Get(c.toolID), c.name, "", 0, "limit", "the dashboard")
			if err != nil {
				return flashMsg{err.Error(), true}
			}
			return flashMsg{fmt.Sprintf("%s rests (%s); %s is now active", res.From, Until(res.Until), res.To), false}
		}
	}
	sel, ok := m.selected()
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.Add):
		return m, m.exec("init")
	case !ok:
		return m, nil
	case key.Matches(msg, keys.Use):
		return m, func() tea.Msg {
			if _, err := ops.Use(tools.Get(sel.toolID), sel.name); err != nil {
				return flashMsg{err.Error(), true}
			}
			return flashMsg{fmt.Sprintf("New %s sessions use %s", tools.Get(sel.toolID).Title(), sel.name), false}
		}
	case key.Matches(msg, keys.Start):
		m.launch = &Launch{Tool: sel.toolID, Profile: sel.name}
		return m, tea.Quit
	case key.Matches(msg, keys.Login):
		return m, m.exec("login", string(sel.toolID), sel.name)
	case key.Matches(msg, keys.Failover):
		// Marking an account takes it out of use for hours: ask first.
		m.confirm = &sel
		m.flash, m.flashBad = fmt.Sprintf("Mark %s/%s as limited and make the next account active? y/n", sel.toolID, sel.name), false
	case key.Matches(msg, keys.Clear):
		return m, func() tea.Msg {
			if _, err := ops.ClearMarks(tools.Get(sel.toolID), sel.name); err != nil {
				return flashMsg{err.Error(), true}
			}
			return flashMsg{"Cleared the marks on " + sel.name, false}
		}
	case key.Matches(msg, keys.Refresh):
		return m, m.checkAll()
	}
	return m, nil
}

// exec hands the terminal to another aims command and comes back after it.
func (m *dashboard) exec(args ...string) tea.Cmd {
	if m.self == "" {
		return func() tea.Msg { return flashMsg{"cannot find the aims binary", true} }
	}
	return tea.ExecProcess(exec.Command(m.self, args...), func(err error) tea.Msg { return execDoneMsg{err} })
}

func (m *dashboard) checkAll() tea.Cmd {
	cfg, err := config.Load()
	if err != nil {
		return func() tea.Msg { return flashMsg{err.Error(), true} }
	}
	var cmds []tea.Cmd
	for _, t := range m.report {
		a := tools.Get(t.ID)
		if !t.Installed {
			continue
		}
		for _, p := range t.Profiles {
			k := string(t.ID) + "/" + p.Name
			if m.checking[k] {
				continue
			}
			m.checking[k] = true
			cmds = append(cmds, func() tea.Msg {
				profiles.Live(m.ctx, cfg, a, p.Name)
				return liveDoneMsg{k}
			})
		}
	}
	m.flash, m.flashBad = "", false
	return tea.Batch(cmds...)
}

func (m *dashboard) View() string {
	s := Out
	w := max(m.width, 60)
	var b strings.Builder

	title := s.Title.Render("aims") + s.Dim.Render("  accounts for "+strings.Join(titles(), " and "))
	ver := s.Dim.Render(m.version)
	b.WriteString("\n " + title + strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(ver)-2)) + ver + "\n\n")

	i := 0
	for _, t := range m.report {
		head := " " + s.Bold.Render(t.Title)
		where := s.Dim.Render(fsx.Tildify(t.Hub))
		if !t.Installed {
			where = s.Dim.Render("not installed")
		}
		b.WriteString(head + strings.Repeat(" ", max(1, w-lipgloss.Width(head)-lipgloss.Width(where)-1)) + where + "\n")
		if len(t.Profiles) == 0 {
			b.WriteString("     " + s.Dim.Render("no accounts yet, press ") + s.Cmd.Render("a") + "\n\n")
			continue
		}
		nameW, acctW := 0, 0
		for _, p := range t.Profiles {
			nameW = max(nameW, len(p.Name))
			acctW = max(acctW, lipgloss.Width(Account(s, p)))
		}
		for _, p := range t.Profiles {
			cursor := "   "
			name := p.Name
			if i == m.cursor {
				cursor = s.Accent.Render(" ▸ ")
				name = s.Accent.Bold(true).Render(name)
			}
			mark := "  "
			if p.Active {
				mark = s.Accent.Render("● ")
			}
			state := State(s, p)
			if m.checking[string(t.ID)+"/"+p.Name] {
				state = m.spin.View() + s.Dim.Render(" checking")
			}
			line := cursor + mark + Pad(name, nameW) + "   " + Pad(Account(s, p), acctW) + "   " + Pad(state, 16)
			if u := Usage(s, p.Usage, m.threshold); u != "" {
				line += "  " + u
			}
			b.WriteString(line + "\n")
			i++
		}
		b.WriteString("\n")
	}

	if sel, ok := m.selected(); ok {
		card := m.card(sel, min(w-2, 78))
		// Drop the card rather than push the list off a short terminal.
		if m.height == 0 || lipgloss.Height(b.String())+lipgloss.Height(card)+3 <= m.height {
			b.WriteString(card + "\n")
		}
	}
	if m.flash != "" {
		st, glyph := s.OK, glyphOK
		switch {
		case m.confirm != nil:
			st, glyph = s.Accent, "?"
		case m.flashBad:
			st, glyph = s.Bad, glyphFail
		}
		b.WriteString(" " + st.Render(glyph) + " " + m.flash + "\n")
	}
	b.WriteString("\n" + s.Renderer().NewStyle().PaddingLeft(1).Render(m.help.View(keys)) + "\n")
	return b.String()
}

func titles() []string {
	var out []string
	for _, a := range tools.All() {
		out = append(out, a.Title())
	}
	return out
}

// card is the detail box of the selected profile.
func (m *dashboard) card(sel selection, width int) string {
	s := Out
	p := sel.p
	var tags []string
	if p.Active {
		tags = append(tags, "active")
	}
	if p.Existing {
		tags = append(tags, "default login")
	}
	switch p.Share {
	case "none":
		tags = append(tags, "isolated")
	case "settings":
		tags = append(tags, "own history")
	}
	if p.Pinned {
		tags = append(tags, "pinned in this terminal")
	}
	head := s.Bold.Render(string(sel.toolID)+" / "+p.Name) + "  " + s.Dim.Render(strings.Join(tags, " · "))
	label := func(l string) string { return s.Dim.Render(Pad(l, 10)) }
	lines := []string{head, ""}
	lines = append(lines, label("folder")+fsx.Tildify(p.Dir))
	lines = append(lines, label("account")+Account(s, p))
	lines = append(lines, label("state")+State(s, p))
	if p.MarkedBy != "" {
		why := "by " + p.MarkedBy
		if !p.MarkedAt.IsZero() {
			why += ", " + ago(p.MarkedAt)
		}
		if p.Detail != "" {
			why += ": " + p.Detail
		}
		lines = append(lines, label("marked")+s.Dim.Render(why))
	}
	for _, w := range p.Usage {
		line := label(w.Label) + Bar(s, w.Percent, m.threshold, 24) + fmt.Sprintf(" %3.0f%%", w.Percent)
		if w.ResetsAt.After(time.Now()) {
			line += s.Dim.Render("  resets in " + profiles.Duration(time.Until(w.ResetsAt)))
		}
		lines = append(lines, line)
	}
	if !p.UsageAt.IsZero() {
		lines = append(lines, label("measured")+s.Dim.Render(ago(p.UsageAt)))
	}
	if !p.LastUsed.IsZero() {
		lines = append(lines, label("last used")+s.Dim.Render(ago(p.LastUsed)))
	}
	switch next := Next(sel.toolID, p); {
	case next != "":
		lines = append(lines, "", s.Warn.Render("press l to log in")+s.Dim.Render("  or run ")+s.Cmd.Render(next))
	case !p.Until.IsZero():
		lines = append(lines, "", s.Dim.Render("works again? press ")+s.Cmd.Render("c")+s.Dim.Render(" to clear the mark"))
	}
	return s.Renderer().NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1).MarginLeft(1).Width(width).Render(strings.Join(lines, "\n"))
}

func ago(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	return profiles.Duration(d) + " ago"
}
