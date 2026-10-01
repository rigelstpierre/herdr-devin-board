package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

const (
	finishedWindow = 7 * 24 * time.Hour
	loadTimeout    = 60 * time.Second
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
	refreshedAt time.Time
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

func New(deps Deps) Model {
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
			m.err = nil
			m.data = msg.data
			m.loaded = true
			m.refreshedAt = m.deps.Now()
		}
		m.clampCursor()
		return m, nil
	case tickMsg:
		if m.loading {
			return m, m.tick()
		}
		m.loading = true
		return m, tea.Batch(m.load(), m.tick())
	case actionErrMsg:
		m.err = msg.err
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
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
		m.clampCursor()
	case "k", "up":
		m.cursor--
		m.clampCursor()
	case "a":
		m.showAll = !m.showAll
		m.clampCursor()
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
		Now:     m.deps.Now(),
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

func (m *Model) clampCursor() {
	count := len(flatten(m.sessions()))
	if m.cursor >= count {
		m.cursor = count - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) selected() (board.Session, line, bool) {
	sessions := m.sessions()
	lines := flatten(sessions)
	if len(lines) == 0 {
		return board.Session{}, line{}, false
	}
	l := lines[m.cursor]
	return sessions[l.session], l, true
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
