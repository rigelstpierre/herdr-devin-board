package board

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

type Kind int

const (
	Running Kind = iota
	Waiting
	Suspended
	Finished
	Errored
)

type PR struct {
	URL     string
	Repo    string
	Number  int
	State   string
	CI      github.CI
	Review  string
	Unknown bool
}

type Session struct {
	ID        string
	URL       string
	Title     string
	Kind      Kind
	Detail    string
	UpdatedAt time.Time
	PRs       []PR
}

type Options struct {
	Now     time.Time
	ShowAll bool
	Window  time.Duration
}

func Classify(status, detail string) Kind {
	switch {
	case status == "error":
		return Errored
	case detail == "waiting_for_user" || detail == "waiting_for_approval":
		return Waiting
	case status == "exit" || detail == "finished":
		return Finished
	case status == "suspended" || status == "resuming":
		return Suspended
	}
	return Running
}

func Build(sessions []devin.Session, statuses map[string]github.PRStatus, opts Options) []Session {
	cutoff := opts.Now.Add(-opts.Window)
	var rows []Session
	for _, s := range sessions {
		kind := Classify(s.Status, s.StatusDetail)
		updated := time.Unix(s.UpdatedAt, 0)
		if !opts.ShowAll && updated.Before(cutoff) && isStale(kind, s.PullRequests) {
			continue
		}
		rows = append(rows, Session{
			ID:        s.ID,
			URL:       s.URL,
			Title:     titleOrPlaceholder(s.Title),
			Kind:      kind,
			Detail:    s.StatusDetail,
			UpdatedAt: updated,
			PRs:       buildPRs(s.PullRequests, statuses),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		iWaiting, jWaiting := rows[i].Kind == Waiting, rows[j].Kind == Waiting
		if iWaiting != jWaiting {
			return iWaiting
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	return rows
}

func OpenPRURLs(sessions []devin.Session) []string {
	seen := map[string]bool{}
	var urls []string
	for _, s := range sessions {
		for _, pr := range s.PullRequests {
			if pr.State == "open" && !seen[pr.URL] {
				seen[pr.URL] = true
				urls = append(urls, pr.URL)
			}
		}
	}
	return urls
}

func isStale(kind Kind, prs []devin.PullRequest) bool {
	switch kind {
	case Finished:
		return true
	case Suspended:
		return !hasOpenPR(prs)
	}
	return false
}

func hasOpenPR(prs []devin.PullRequest) bool {
	for _, pr := range prs {
		if pr.State == "open" {
			return true
		}
	}
	return false
}

func titleOrPlaceholder(title string) string {
	if strings.TrimSpace(title) == "" {
		return "(untitled)"
	}
	return title
}

func buildPRs(prs []devin.PullRequest, statuses map[string]github.PRStatus) []PR {
	var out []PR
	for _, p := range prs {
		out = append(out, buildPR(p, statuses))
	}
	return out
}

func buildPR(p devin.PullRequest, statuses map[string]github.PRStatus) PR {
	repo, number := parsePRURL(p.URL)
	pr := PR{URL: p.URL, Repo: repo, Number: number, State: p.State}
	if p.State != "open" {
		return pr
	}
	status, ok := statuses[p.URL]
	if !ok {
		pr.Unknown = true
		return pr
	}
	pr.State = strings.ToLower(status.State)
	if status.IsDraft && pr.State == "open" {
		pr.State = "draft"
	}
	pr.CI = status.CI
	pr.Review = status.Review
	return pr
}

func parsePRURL(raw string) (string, int) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", 0
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", 0
	}
	number, _ := strconv.Atoi(parts[3])
	return parts[1], number
}
