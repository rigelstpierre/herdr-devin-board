package board_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) int64 { return now.Add(-d).Unix() }

func opts(showAll bool) board.Options {
	return board.Options{Now: now, ShowAll: showAll, Window: 7 * 24 * time.Hour}
}

func ids(sessions []board.Session) []string {
	var out []string
	for _, s := range sessions {
		out = append(out, s.ID)
	}
	return out
}

func TestClassify(t *testing.T) {
	cases := []struct {
		status, detail string
		want           board.Kind
	}{
		{"error", "waiting_for_user", board.Errored},
		{"running", "waiting_for_user", board.Waiting},
		{"running", "waiting_for_approval", board.Waiting},
		{"exit", "", board.Finished},
		{"running", "finished", board.Finished},
		{"suspended", "inactivity", board.Suspended},
		{"resuming", "", board.Suspended},
		{"running", "working", board.Running},
		{"new", "", board.Running},
		{"claimed", "", board.Running},
	}
	for _, tc := range cases {
		if got := board.Classify(tc.status, tc.detail); got != tc.want {
			t.Errorf("Classify(%q, %q) = %v want %v", tc.status, tc.detail, got, tc.want)
		}
	}
}

func TestBuildHidesStaleSessionsUnlessShowAll(t *testing.T) {
	openPR := []devin.PullRequest{{URL: "https://github.com/o/r/pull/1", State: "open"}}
	sessions := []devin.Session{
		{ID: "old-finished", Status: "exit", UpdatedAt: ago(8 * 24 * time.Hour)},
		{ID: "old-finished-open-pr", Status: "exit", UpdatedAt: ago(8 * 24 * time.Hour), PullRequests: openPR},
		{ID: "recent-finished", Status: "exit", UpdatedAt: ago(2 * 24 * time.Hour)},
		{ID: "old-suspended", Status: "suspended", UpdatedAt: ago(30 * 24 * time.Hour)},
		{ID: "old-suspended-open-pr", Status: "suspended", UpdatedAt: ago(30 * 24 * time.Hour), PullRequests: openPR},
		{ID: "recent-suspended", Status: "suspended", UpdatedAt: ago(time.Hour)},
		{ID: "old-running", Status: "running", UpdatedAt: ago(30 * 24 * time.Hour)},
	}

	got := ids(board.Build(sessions, nil, opts(false)))

	want := []string{"recent-suspended", "recent-finished", "old-suspended-open-pr", "old-running"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default: got %v want %v", got, want)
	}
	if got := ids(board.Build(sessions, nil, opts(true))); len(got) != len(sessions) {
		t.Fatalf("show all: %v", got)
	}
}

func TestBuildSortsWaitingFirstThenNewest(t *testing.T) {
	sessions := []devin.Session{
		{ID: "running-old", Status: "running", UpdatedAt: ago(3 * time.Hour)},
		{ID: "waiting-old", Status: "running", StatusDetail: "waiting_for_user", UpdatedAt: ago(5 * time.Hour)},
		{ID: "running-new", Status: "running", UpdatedAt: ago(time.Minute)},
		{ID: "waiting-new", Status: "running", StatusDetail: "waiting_for_user", UpdatedAt: ago(time.Hour)},
	}

	got := ids(board.Build(sessions, nil, opts(false)))

	want := []string{"waiting-new", "waiting-old", "running-new", "running-old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildMergesPRStatus(t *testing.T) {
	sessions := []devin.Session{{
		ID: "s", Status: "running", UpdatedAt: ago(time.Minute), Title: "",
		PullRequests: []devin.PullRequest{
			{URL: "https://github.com/acme/api/pull/10", State: "open"},
			{URL: "https://github.com/acme/api/pull/11", State: "open"},
			{URL: "https://github.com/acme/docs/pull/12", State: "merged"},
		},
	}}
	statuses := map[string]github.PRStatus{
		"https://github.com/acme/api/pull/10": {Number: 10, State: "OPEN", IsDraft: true, CI: github.CIFailing, Review: "REVIEW_REQUIRED"},
	}

	got := board.Build(sessions, statuses, opts(false))[0]

	if got.Title != "(untitled)" {
		t.Fatalf("title %q", got.Title)
	}
	want := []board.PR{
		{URL: "https://github.com/acme/api/pull/10", Repo: "api", Number: 10, State: "draft", CI: github.CIFailing, Review: "REVIEW_REQUIRED"},
		{URL: "https://github.com/acme/api/pull/11", Repo: "api", Number: 11, State: "open", Unknown: true},
		{URL: "https://github.com/acme/docs/pull/12", Repo: "docs", Number: 12, State: "merged"},
	}
	if !reflect.DeepEqual(got.PRs, want) {
		t.Fatalf("got %+v\nwant %+v", got.PRs, want)
	}
}

func TestOpenPRURLs(t *testing.T) {
	sessions := []devin.Session{
		{PullRequests: []devin.PullRequest{{URL: "a", State: "open"}, {URL: "b", State: "merged"}}},
		{PullRequests: []devin.PullRequest{{URL: "a", State: "open"}, {URL: "c", State: "open"}}},
	}

	got := board.OpenPRURLs(sessions)

	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("got %v", got)
	}
}

func TestBuildPrefersGitHubStateOverDevin(t *testing.T) {
	sessions := []devin.Session{{
		ID: "s", Status: "running", UpdatedAt: ago(time.Minute),
		PullRequests: []devin.PullRequest{
			{URL: "https://github.com/o/r/pull/1", State: "open"},
			{URL: "https://github.com/o/r/pull/2", State: "open"},
		},
	}}
	statuses := map[string]github.PRStatus{
		"https://github.com/o/r/pull/1": {Number: 1, State: "MERGED"},
		"https://github.com/o/r/pull/2": {Number: 2, State: "CLOSED", IsDraft: true},
	}

	prs := board.Build(sessions, statuses, opts(false))[0].PRs

	if prs[0].State != "merged" || prs[1].State != "closed" {
		t.Fatalf("got %q, %q", prs[0].State, prs[1].State)
	}
}

func TestBuildParsesPRURLs(t *testing.T) {
	cases := []struct {
		url    string
		repo   string
		number int
	}{
		{"https://github.com/acme/api/pull/23601", "api", 23601},
		{"https://github.com/acme/api/pull/23601/files", "api", 23601},
		{"https://github.com/acme/api/issues/5", "", 0},
		{"not a url", "", 0},
	}
	for _, tc := range cases {
		sessions := []devin.Session{{ID: "s", UpdatedAt: ago(time.Minute), PullRequests: []devin.PullRequest{{URL: tc.url, State: "merged"}}}}

		pr := board.Build(sessions, nil, opts(false))[0].PRs[0]

		if pr.Repo != tc.repo || pr.Number != tc.number {
			t.Errorf("%q: got %q #%d want %q #%d", tc.url, pr.Repo, pr.Number, tc.repo, tc.number)
		}
	}
}
