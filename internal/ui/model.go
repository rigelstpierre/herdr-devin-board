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
			previous, hadSelection := m.selected()
			m.err = nil
			m.data = msg.data
			m.loaded = true
			m.refreshedAt = m.deps.Now()
			m.builtAt = m.refreshedAt
			if hadSelection {
				m.restoreSelection(previous.ID)
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
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m, m.openPR(int(msg.Runes[0] - '1'))
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

func (m *Model) settle() {
	count := len(m.sessions())
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
	return max(m.height-chromeHeight, 1)
}

func (m Model) selected() (board.Session, bool) {
	sessions := m.sessions()
	if m.cursor < 0 || m.cursor >= len(sessions) {
		return board.Session{}, false
	}
	return sessions[m.cursor], true
}

func (m *Model) restoreSelection(sessionID string) {
	for i, s := range m.sessions() {
		if s.ID == sessionID {
			m.cursor = i
			return
		}
	}
}

func (m Model) openSelected() tea.Cmd {
	session, ok := m.selected()
	if !ok {
		return nil
	}
	return m.action(func() error { return m.deps.OpenURL(session.URL) })
}

func (m Model) openFirstPR() tea.Cmd {
	return m.openPR(0)
}

func (m Model) openPR(index int) tea.Cmd {
	session, ok := m.selected()
	if !ok || index >= len(session.PRs) {
		return nil
	}
	return m.action(func() error { return m.deps.OpenURL(session.PRs[index].URL) })
}

func (m Model) sshSelected() tea.Cmd {
	session, ok := m.selected()
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
