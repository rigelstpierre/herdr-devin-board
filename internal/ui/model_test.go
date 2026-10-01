package ui

import (
	"context"
	"errors"
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
	opened []string
	sshed  []string
	loads  int
}

func newTestModel(s *spy) Model {
	return New(Deps{
		Load: func(context.Context) (Data, error) {
			s.loads++
			return fixtureData(), nil
		},
		OpenURL:  func(u string) error { s.opened = append(s.opened, u); return nil },
		SSH:      func(id string) error { s.sshed = append(s.sshed, id); return nil },
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

func TestEnterOpensSessionThenPR(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)

	_, cmd := update(t, m, key("enter"))
	cmd()
	m, _ = update(t, m, key("down"))
	_, cmd = update(t, m, key("enter"))
	cmd()

	want := []string{"https://devin.test/sessions/s1", "https://github.com/rootlyhq/rootly/pull/23601"}
	if len(s.opened) != 2 || s.opened[0] != want[0] || s.opened[1] != want[1] {
		t.Fatalf("opened %v", s.opened)
	}
}

func TestPOpensFirstPRAndSSSHes(t *testing.T) {
	s := &spy{}
	m := loaded(t, s)

	_, cmd := update(t, m, key("p"))
	cmd()
	_, cmd = update(t, m, key("s"))
	cmd()

	if len(s.opened) != 1 || s.opened[0] != "https://github.com/rootlyhq/rootly/pull/23601" {
		t.Fatalf("opened %v", s.opened)
	}
	if len(s.sshed) != 1 || s.sshed[0] != "s1" {
		t.Fatalf("sshed %v", s.sshed)
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

	m, _ = update(t, m, tickMsg{})

	if !m.loading {
		t.Fatal("expected still loading")
	}
	m, _ = update(t, m, loadedMsg{data: fixtureData()})
	if m.loading {
		t.Fatal("expected idle after load")
	}
	m, _ = update(t, m, tickMsg{})
	if !m.loading {
		t.Fatal("tick should start a load when idle")
	}
}

func TestActionErrorSurfacesInBanner(t *testing.T) {
	m := loaded(t, &spy{})

	m, _ = update(t, m, actionErrMsg{err: errors.New("open failed")})

	if m.err == nil || m.err.Error() != "open failed" {
		t.Fatalf("err %v", m.err)
	}
}
