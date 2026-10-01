package ui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

func goldenData() Data {
	return Data{
		Sessions: []devin.Session{
			{ID: "s1", Title: "Fix IR-7158", Status: "running", StatusDetail: "working", UpdatedAt: fixedNow.Add(-2 * time.Minute).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23601", State: "open"}}},
			{ID: "s2", Title: "Add SCIM phone normalisation", Status: "running", StatusDetail: "waiting_for_user", UpdatedAt: fixedNow.Add(-18 * time.Minute).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23588", State: "open"}}},
			{ID: "s3", Title: "Bump herdr config", Status: "exit", UpdatedAt: fixedNow.Add(-3 * time.Hour).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23540", State: "merged"}}},
			{ID: "s4", Title: "Investigate flaky alert spec", Status: "error", UpdatedAt: fixedNow.Add(-26 * time.Hour).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23500", State: "open"}}},
			{ID: "s5", Title: "Idle research", Status: "suspended", StatusDetail: "inactivity", UpdatedAt: fixedNow.Add(-50 * time.Hour).Unix()},
		},
		Statuses: map[string]github.PRStatus{
			"https://github.com/rootlyhq/rootly/pull/23601": {Number: 23601, State: "OPEN", CI: github.CIPassing, Review: "REVIEW_REQUIRED"},
			"https://github.com/rootlyhq/rootly/pull/23588": {Number: 23588, State: "OPEN", IsDraft: true, CI: github.CIFailing},
		},
	}
}

func TestViewGolden(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: goldenData()})
	m.refreshedAt = fixedNow.Add(-12 * time.Second)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})

	got := m.View()

	path := filepath.Join("testdata", "board.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Fatalf("view mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestViewShowsErrorBannerAndUnknownPR(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	data := goldenData()
	delete(data.Statuses, "https://github.com/rootlyhq/rootly/pull/23601")
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: data})
	m, _ = update(t, m, actionErrMsg{err: errorString("Devin API 401 — run `devin auth login`")})

	view := m.View()

	if !strings.Contains(view, "Devin API 401") {
		t.Fatalf("missing banner:\n%s", view)
	}
	if !strings.Contains(view, "#23601   rootly  open    ? ?") {
		t.Fatalf("missing unknown PR row:\n%s", view)
	}
}

func TestViewEmptyState(t *testing.T) {
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: Data{}})

	if !strings.Contains(m.View(), "No sessions in the last 7 days") {
		t.Fatalf("view:\n%s", m.View())
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestHeaderNamesBoardAndCount(t *testing.T) {
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: fixtureData()})

	if !strings.HasPrefix(m.View(), "Devin Sessions · 1 · refreshed") {
		t.Fatalf("view:\n%s", m.View())
	}
}

func goldenLines(t *testing.T, width int) []string {
	t.Helper()
	lipgloss.SetColorProfile(termenv.Ascii)
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: goldenData()})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 40})
	return strings.Split(m.View(), "\n")
}

func TestSessionRowsStayCompactOnWidePanes(t *testing.T) {
	for _, l := range goldenLines(t, 200) {
		if strings.Contains(l, "Fix IR-7158") && ansi.StringWidth(l) > 60 {
			t.Fatalf("row stretched to %d columns: %q", ansi.StringWidth(l), l)
		}
	}
}

func TestPRColumnsAlignUnderTitles(t *testing.T) {
	var titleColumn int
	var prColumns []int
	for _, l := range goldenLines(t, 80) {
		if i := strings.Index(l, "Fix IR-7158"); i >= 0 {
			titleColumn = ansi.StringWidth(l[:i])
		}
		if i := strings.Index(l, "#"); i >= 0 {
			prColumns = append(prColumns, ansi.StringWidth(l[:i]))
			state := strings.Index(l, "rootly  ")
			if state < 0 {
				t.Fatalf("repo column not padded: %q", l)
			}
		}
	}
	if len(prColumns) == 0 {
		t.Fatal("no PR rows")
	}
	for _, c := range prColumns {
		if c != titleColumn {
			t.Fatalf("PR row starts at %d, titles at %d", c, titleColumn)
		}
	}
}

func TestTitlesTruncateByDisplayWidth(t *testing.T) {
	data := Data{Sessions: []devin.Session{{ID: "w", Title: strings.Repeat("修正", 40), Status: "running", UpdatedAt: fixedNow.Unix()}}}
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: data})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 20})

	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "修正") && ansi.StringWidth(l) > 50 {
			t.Fatalf("wide title overflows: %d columns", ansi.StringWidth(l))
		}
	}
}
