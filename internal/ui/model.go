package ui

import (
	"context"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

const (
	finishedWindow  = 7 * 24 * time.Hour
	defaultInterval = 30 * time.Second
	loadTimeout     = 60 * time.Second
)

type Data struct {
	Sessions []devin.Session
	Statuses map[string]github.PRStatus
}

type Deps struct {
	Load     func(context.Context) (Data, error)
	OpenURL  func(string) error
	SSH      func(string) error
	Now      func() time.Time
	Interval time.Duration
}

type Model struct {
	deps        Deps
	data        Data
	loaded      bool
	loading     bool
	err         error
	showAll     bool
	cursor      int
	width       int
	height      int
	offset      int
	refreshedAt time.Time
	builtAt     time.Time
}

type loadedMsg struct {
	data Data
	err  error
}

type tickMsg struct{}

type actionErrMsg struct {
	err error
}

type line struct {
	session int
	pr      int
}

type selection struct {
	sessionID string
	pr        int
}

func New(deps Deps) Model {
	if deps.Interval <= 0 {
		deps.Interval = defaultInterval
	}
	return Model{deps: deps, loading: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			previous, hadSelection := m.selection()
			m.err = nil
			m.data = msg.data
			m.loaded = true
			m.refreshedAt = m.deps.Now()
			m.builtAt = m.refreshedAt
			if hadSelection {
				m.restoreSelection(previous)
			}
		}
		m.settle()
		return m, nil
	case tickMsg:
		if m.loading {
			return m, m.tick()
		}
		m.loading = true
		return m, tea.Batch(m.load(), m.tick())
	case actionErrMsg:
		m.err = msg.err
		m.settle()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.settle()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
		m.settle()
	case "k", "up":
		m.cursor--
		m.settle()
	case "a":
		m.showAll = !m.showAll
		m.settle()
	case "r":
		if !m.loading {
			m.loading = true
			return m, m.load()
		}
	case "enter":
		return m, m.openSelected()
	case "p":
		return m, m.openFirstPR()
	case "s":
		return m, m.sshSelected()
	}
	return m, nil
}

func (m Model) sessions() []board.Session {
	return board.Build(m.data.Sessions, m.data.Statuses, board.Options{
		Now:     m.builtAt,
		ShowAll: m.showAll,
		Window:  finishedWindow,
	})
}

func flatten(sessions []board.Session) []line {
	var lines []line
	for i, s := range sessions {
		lines = append(lines, line{session: i, pr: -1})
		for j := range s.PRs {
			lines = append(lines, line{session: i, pr: j})
		}
	}
	return lines
}

func (m *Model) settle() {
	count := len(flatten(m.sessions()))
	m.cursor = min(m.cursor, count-1)
	m.cursor = max(m.cursor, 0)
	rows := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(min(m.offset, count-rows), 0)
}

func (m Model) visibleRows() int {
	if m.height == 0 {
		return math.MaxInt32
	}
	chrome := 4
	if m.err != nil {
		chrome++
	}
	if !m.loaded || len(m.sessions()) == 0 {
		chrome++
	}
	return max(m.height-chrome, 1)
}

func (m Model) selected() (board.Session, line, bool) {
	sessions := m.sessions()
	lines := flatten(sessions)
	if m.cursor < 0 || m.cursor >= len(lines) {
		return board.Session{}, line{}, false
	}
	l := lines[m.cursor]
	return sessions[l.session], l, true
}

func (m Model) selection() (selection, bool) {
	session, l, ok := m.selected()
	return selection{sessionID: session.ID, pr: l.pr}, ok
}

func (m *Model) restoreSelection(previous selection) {
	sessions := m.sessions()
	for i, l := range flatten(sessions) {
		if sessions[l.session].ID == previous.sessionID && l.pr == previous.pr {
			m.cursor = i
			return
		}
	}
}

func (m Model) openSelected() tea.Cmd {
	session, l, ok := m.selected()
	if !ok {
		return nil
	}
	if l.pr >= 0 {
		return m.action(func() error { return m.deps.OpenURL(session.PRs[l.pr].URL) })
	}
	return m.action(func() error { return m.deps.OpenURL(session.URL) })
}

func (m Model) openFirstPR() tea.Cmd {
	session, _, ok := m.selected()
	if !ok || len(session.PRs) == 0 {
		return nil
	}
	return m.action(func() error { return m.deps.OpenURL(session.PRs[0].URL) })
}

func (m Model) sshSelected() tea.Cmd {
	session, _, ok := m.selected()
	if !ok {
		return nil
	}
	return m.action(func() error { return m.deps.SSH(session.ID) })
}

func (m Model) action(run func() error) tea.Cmd {
	return func() tea.Msg {
		if err := run(); err != nil {
			return actionErrMsg{err: err}
		}
		return nil
	}
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()
		data, err := m.deps.Load(ctx)
		return loadedMsg{data: data, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.deps.Interval, func(time.Time) tea.Msg { return tickMsg{} })
}
