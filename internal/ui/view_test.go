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
			{ID: "s1", URL: "https://devin.test/sessions/s1", Title: "Fix IR-7158", Status: "running", StatusDetail: "working", UpdatedAt: fixedNow.Add(-2 * time.Minute).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23601", State: "open"}}},
			{ID: "s2", URL: "https://devin.test/sessions/s2", Title: "Add SCIM phone normalisation", Status: "running", StatusDetail: "waiting_for_user", UpdatedAt: fixedNow.Add(-18 * time.Minute).Unix(),
				PullRequests: []devin.PullRequest{
					{URL: "https://github.com/rootlyhq/rootly/pull/23588", State: "open"},
					{URL: "https://github.com/rootlyhq/terraform-rootly/pull/1550", State: "merged"},
				}},
			{ID: "s3", URL: "https://devin.test/sessions/s3", Title: "Bump herdr config", Status: "exit", UpdatedAt: fixedNow.Add(-3 * time.Hour).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23540", State: "merged"}}},
			{ID: "s4", URL: "https://devin.test/sessions/s4", Title: "Investigate flaky alert spec", Status: "error", UpdatedAt: fixedNow.Add(-26 * time.Hour).Unix(),
				PullRequests: []devin.PullRequest{{URL: "https://github.com/rootlyhq/rootly/pull/23500", State: "open"}}},
			{ID: "s5", URL: "https://devin.test/sessions/s5", Title: "Idle research", Status: "suspended", StatusDetail: "inactivity", UpdatedAt: fixedNow.Add(-50 * time.Hour).Unix()},
		},
		Statuses: map[string]github.PRStatus{
			"https://github.com/rootlyhq/rootly/pull/23601": {Number: 23601, State: "OPEN", CI: github.CIPassing, Review: "REVIEW_REQUIRED"},
			"https://github.com/rootlyhq/rootly/pull/23588": {Number: 23588, State: "OPEN", IsDraft: true, CI: github.CIFailing},
		},
	}
}

func render(t *testing.T, data Data, width, height int) Model {
	t.Helper()
	lipgloss.SetColorProfile(termenv.Ascii)
	m, _ := update(t, newTestModel(&spy{}), loadedMsg{data: data})
	m.refreshedAt = fixedNow.Add(-12 * time.Second)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func viewLines(m Model) []string {
	return strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n")
}

func TestViewGolden(t *testing.T) {
	got := render(t, goldenData(), 100, 0).View()

	path := filepath.Join("testdata", "board.golden")
	if *updateGolden {
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

func TestTableHasColumnHeadersAndOneRowPerSession(t *testing.T) {
	lines := viewLines(render(t, goldenData(), 120, 0))

	header := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "STATUS") {
			header = i
		}
	}
	if header < 0 {
		t.Fatalf("no header row:\n%s", strings.Join(lines, "\n"))
	}
	for _, col := range []string{"SESSION", "REPO", "PR", "CI", "REVIEW", "UPDATED"} {
		if !strings.Contains(lines[header], col) {
			t.Fatalf("header missing %s: %q", col, lines[header])
		}
	}
	rows := lines[header+2 : header+2+5]
	for _, title := range []string{"Add SCIM phone normalisation", "Fix IR-7158", "Bump herdr config", "Investigate flaky alert spec", "Idle research"} {
		found := false
		for _, r := range rows {
			found = found || strings.Contains(r, title)
		}
		if !found {
			t.Fatalf("no row for %q in:\n%s", title, strings.Join(rows, "\n"))
		}
	}
}

func TestColumnsAlignAcrossRows(t *testing.T) {
	lines := viewLines(render(t, goldenData(), 120, 0))

	column := func(l, header string) int { return ansi.StringWidth(l[:strings.Index(l, header)]) }
	var headerLine string
	for _, l := range lines {
		if strings.HasPrefix(l, "STATUS") {
			headerLine = l
		}
	}
	prColumn := column(headerLine, "  PR ") + 2
	for _, l := range lines {
		if i := strings.Index(l, "#23"); i >= 0 && !strings.Contains(l, "github.com") && !strings.Contains(l, "rootlyhq/") {
			if got := ansi.StringWidth(l[:i]); got != prColumn {
				t.Fatalf("PR cell at column %d, header at %d: %q", got, prColumn, l)
			}
		}
	}
}

func TestDetailPanelShowsSelectedSessionAndAllItsPRs(t *testing.T) {
	view := render(t, goldenData(), 120, 0).View()

	for _, want := range []string{
		"https://devin.test/sessions/s2",
		"waiting for user",
		"https://github.com/rootlyhq/rootly/pull/23588",
		"https://github.com/rootlyhq/terraform-rootly/pull/1550",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail missing %q:\n%s", want, view)
		}
	}
}

func TestFooterIsPinnedToTheBottom(t *testing.T) {
	lines := viewLines(render(t, goldenData(), 120, 40))

	if len(lines) != 40 {
		t.Fatalf("view has %d lines, want 40", len(lines))
	}
	if !strings.Contains(lines[39], "refreshed 12s ago") {
		t.Fatalf("last line %q", lines[39])
	}
	if !strings.Contains(lines[38], "Enter") || !strings.Contains(lines[38], "quit") {
		t.Fatalf("key hints line %q", lines[38])
	}
}

func TestRowsFitTheWidth(t *testing.T) {
	data := goldenData()
	data.Sessions[0].Title = strings.Repeat("修正", 60)
	for _, width := range []int{60, 100, 200} {
		for _, l := range viewLines(render(t, data, width, 0)) {
			if strings.Contains(l, "github.com") || strings.Contains(l, "devin.test") {
				continue
			}
			if w := ansi.StringWidth(l); w > width {
				t.Fatalf("width %d: line is %d columns: %q", width, w, l)
			}
		}
	}
}

func TestErrorShowsInStatusLine(t *testing.T) {
	m := render(t, goldenData(), 120, 30)
	m, _ = update(t, m, actionErrMsg{err: errorString("Devin API 403 — run `devin auth login`")})

	lines := viewLines(m)

	if !strings.Contains(lines[len(lines)-1], "Devin API 403") {
		t.Fatalf("last line %q", lines[len(lines)-1])
	}
}

func TestUnknownPRStatusShowsQuestionMark(t *testing.T) {
	data := goldenData()
	delete(data.Statuses, "https://github.com/rootlyhq/rootly/pull/23601")

	for _, l := range viewLines(render(t, data, 120, 0)) {
		if strings.Contains(l, "Fix IR-7158") && !strings.Contains(l, "?") {
			t.Fatalf("expected ? for unknown CI: %q", l)
		}
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
