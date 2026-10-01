package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

var fixedNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func fixtureData() Data {
	return Data{Sessions: []devin.Session{
		{ID: "s1", URL: "https://devin.test/sessions/s1", Title: "Fix IR-7158", Status: "running", StatusDetail: "working", UpdatedAt: fixedNow.Add(-2 * time.Minute).Unix(),
			PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23601", State: "merged"}}},
		{ID: "s2", URL: "https://devin.test/sessions/s2", Title: "Old work", Status: "exit", UpdatedAt: fixedNow.Add(-10 * 24 * time.Hour).Unix()},
	}}
}

type spy struct {
	opened     []string
	sshed      []string
	archived   []string
	archiveErr error
	loads      int
}

func newTestModel(s *spy) Model {
	return New(Deps{
		Load: func(context.Context) (Data, error) {
			s.loads++
			return fixtureData(), nil
		},
		OpenURL: func(u string) error { s.opened = append(s.opened, u); return nil },
		SSH:     func(id string) error { s.sshed = append(s.sshed, id); return nil },
		Archive: func(_ context.Context, id string) error {
			s.archived = append(s.archived, id)
			return s.archiveErr
		},
		Now:      func() time.Time { return fixedNow },
		Interval: 30 * time.Second,
	})
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func loaded(t *testing.T, s *spy) Model {
	t.Helper()
	m, _ := update(t, newTestModel(s), loadedMsg{data: fixtureData()})
	return m
}

func TestLoadedDataHidesOldFinishedUntilShowAll(t *testing.T) {
	m := loaded(t, &spy{})

	if got := len(m.sessions()); got != 1 {
		t.Fatalf("default sessions = %d", got)
	}
	m, _ = update(t, m, key("a"))
	if got := len(m.sessions()); got != 2 {
		t.Fatalf("show all sessions = %d", got)
	}
}

func TestEnterOpensSessionAndDownMovesToNextSession(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)
	m, _ = update(t, m, key("a"))

	_, cmd := update(t, m, key("enter"))
	run(t, cmd)
	m, _ = update(t, m, key("down"))
	_, cmd = update(t, m, key("enter"))
	run(t, cmd)

	want := []string{"https://devin.test/sessions/s1", "https://devin.test/sessions/s2"}
	if len(s.opened) != 2 || s.opened[0] != want[0] || s.opened[1] != want[1] {
		t.Fatalf("opened %v", s.opened)
	}
}

func TestPOpensFirstPRAndSSHes(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)

	_, cmd := update(t, m, key("p"))
	run(t, cmd)
	_, cmd = update(t, m, key("s"))
	run(t, cmd)

	if len(s.opened) != 1 || s.opened[0] != "https://github.com/rootlyhq/rootly/pull/23601" {
		t.Fatalf("opened %v", s.opened)
	}
	if len(s.sshed) != 1 || s.sshed[0] != "s1" {
		t.Fatalf("sshed %v", s.sshed)
	}
}

func TestNumberKeysOpenTheNthPR(t *testing.T) {
	s := &spy{}
	data := Data{Sessions: []devin.Session{{ID: "s", URL: "u", Status: "running", UpdatedAt: fixedNow.Unix(),
		PullRequests: []devin.PullRequest{
			{URL: "https://github.com/o/r/pull/1", State: "merged"},
			{URL: "https://github.com/o/r/pull/2", State: "merged"},
		}}}}
	m, _ := update(t, newTestModel(s), loadedMsg{data: data})

	_, cmd := update(t, m, key("2"))
	run(t, cmd)
	if _, cmd := update(t, m, key("3")); cmd != nil {
		t.Fatal("no third PR, expected no command")
	}

	if len(s.opened) != 1 || s.opened[0] != "https://github.com/o/r/pull/2" {
		t.Fatalf("opened %v", s.opened)
	}
}

func TestFailedRefreshKeepsLastGoodData(t *testing.T) {
	m := loaded(t, &spy{})

	m, _ = update(t, m, loadedMsg{err: errors.New("Devin API 500")})

	if m.err == nil || len(m.sessions()) != 1 {
		t.Fatalf("err=%v sessions=%d", m.err, len(m.sessions()))
	}
}

func TestTickSkipsLoadWhileOneIsInFlight(t *testing.T) {
	m := newTestModel(&spy{})

	m, cmd := update(t, m, tickMsg{})

	if !m.loading || cmd == nil {
		t.Fatalf("expected still loading and re-armed tick, loading=%v cmd=%v", m.loading, cmd)
	}
	m, _ = update(t, m, loadedMsg{data: fixtureData()})
	if m.loading {
		t.Fatal("expected idle after load")
	}
	m, cmd = update(t, m, tickMsg{})
	if !m.loading || cmd == nil {
		t.Fatal("tick should start a load and re-arm when idle")
	}
}

func TestActionErrorSurfacesInBanner(t *testing.T) {
	m := loaded(t, &spy{})

	m, _ = update(t, m, actionErrMsg{err: errors.New("open failed")})

	if m.err == nil || m.err.Error() != "open failed" {
		t.Fatalf("err %v", m.err)
	}
}

func run(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	cmd()
}

func selectedID(t *testing.T, m Model) string {
	t.Helper()
	session, ok := m.selected()
	if !ok {
		t.Fatal("nothing selected")
	}
	return session.ID
}

func TestSelectionFollowsSessionWhenRefreshReorders(t *testing.T) {
	first := Data{Sessions: []devin.Session{
		{ID: "newer", Status: "running", UpdatedAt: fixedNow.Add(-time.Minute).Unix()},
		{ID: "older", Status: "running", UpdatedAt: fixedNow.Add(-time.Hour).Unix()},
	}}
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: first})
	m, _ = update(t, m, key("down"))
	if got := selectedID(t, m); got != "older" {
		t.Fatalf("setup selected %q", got)
	}

	reordered := Data{Sessions: []devin.Session{
		{ID: "newer", Status: "running", UpdatedAt: fixedNow.Add(-time.Minute).Unix()},
		{ID: "older", Status: "running", StatusDetail: "waiting_for_user", UpdatedAt: fixedNow.Add(-time.Hour).Unix()},
	}}
	m, _ = update(t, m, loadedMsg{data: reordered})

	if got := selectedID(t, m); got != "older" {
		t.Fatalf("selection jumped to %q", got)
	}
}

func TestRowsOnlyChangeWhenDataArrives(t *testing.T) {
	now := fixedNow
	m := New(Deps{
		Load:     func(context.Context) (Data, error) { return Data{}, nil },
		OpenURL:  func(string) error { return nil },
		SSH:      func(string) error { return nil },
		Now:      func() time.Time { return now },
		Interval: 30 * time.Second,
	})
	data := Data{Sessions: []devin.Session{
		{ID: "live", Status: "running", UpdatedAt: fixedNow.Unix()},
		{ID: "aging", Status: "exit", UpdatedAt: fixedNow.Add(-7*24*time.Hour + time.Minute).Unix()},
	}}
	m, _ = update(t, m, loadedMsg{data: data})
	m, _ = update(t, m, key("down"))

	now = fixedNow.Add(2 * time.Minute)
	_, cmd := update(t, m, key("s"))

	if cmd == nil {
		t.Fatal("expected ssh command for the still-visible row")
	}
}

func TestSelectedIsSafeWhenCursorIsOutOfRange(t *testing.T) {
	m := loaded(t, &spy{})
	m.cursor = 99

	if _, cmd := update(t, m, key("enter")); cmd != nil {
		t.Fatal("expected no command for an out-of-range cursor")
	}
}

func TestCursorScrollsWithinWindowHeight(t *testing.T) {
	var sessions []devin.Session
	for i := range 12 {
		sessions = append(sessions, devin.Session{
			ID:        fmt.Sprintf("s%02d", i),
			Title:     fmt.Sprintf("session %02d", i),
			Status:    "running",
			UpdatedAt: fixedNow.Add(-time.Duration(i) * time.Minute).Unix(),
		})
	}
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: Data{Sessions: sessions}})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	for range 11 {
		m, _ = update(t, m, key("down"))
	}
	view := m.View()

	if !strings.Contains(view, "session 11") {
		t.Fatalf("cursor row not visible:\n%s", view)
	}
	if strings.Contains(view, "session 00") {
		t.Fatalf("top row should have scrolled off:\n%s", view)
	}
	if lines := strings.Count(view, "\n") + 1; lines != 20 {
		t.Fatalf("view is %d lines, taller than the window:\n%s", lines, view)
	}
}

func TestZeroIntervalDefaults(t *testing.T) {
	m := New(Deps{Now: func() time.Time { return fixedNow }})

	if m.deps.Interval != defaultInterval {
		t.Fatalf("interval %v", m.deps.Interval)
	}
}

func runMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	return cmd()
}

func TestArchiveAsksForConfirmationThenRemovesTheRow(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)

	m, cmd := update(t, m, key("x"))
	if cmd != nil || len(s.archived) != 0 {
		t.Fatal("x must only ask, not archive")
	}
	if !strings.Contains(m.statusLine(), `Archive "Fix IR-7158"?`) {
		t.Fatalf("status line %q", m.statusLine())
	}
	m, cmd = update(t, m, key("y"))
	msg := runMsg(t, cmd)
	m, _ = update(t, m, msg)

	if len(s.archived) != 1 || s.archived[0] != "s1" {
		t.Fatalf("archived %v", s.archived)
	}
	if len(m.sessions()) != 0 {
		t.Fatalf("archived session still listed: %+v", m.sessions())
	}
}

func TestAnyOtherKeyCancelsArchiveWithoutActing(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)

	m, _ = update(t, m, key("x"))
	m, cmd := update(t, m, key("s"))

	if cmd != nil || len(s.sshed) != 0 || len(s.archived) != 0 {
		t.Fatalf("cancel key acted: cmd=%v sshed=%v archived=%v", cmd != nil, s.sshed, s.archived)
	}
	if strings.Contains(m.statusLine(), "Archive") {
		t.Fatalf("prompt still showing: %q", m.statusLine())
	}
}

func TestArchiveFailureShowsInStatusLine(t *testing.T) {
	s := &spy{archiveErr: errors.New("archiving needs a Devin API key — see the README")}
	m := loaded(t, s)

	m, _ = update(t, m, key("x"))
	m, cmd := update(t, m, key("y"))
	m, _ = update(t, m, runMsg(t, cmd))

	if !strings.Contains(m.statusLine(), "needs a Devin API key") || len(m.sessions()) != 1 {
		t.Fatalf("status %q, sessions %d", m.statusLine(), len(m.sessions()))
	}
}
