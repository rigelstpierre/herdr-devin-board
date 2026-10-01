> **Historical.** This is the original v0.1 implementation plan, kept for reference. The board has changed since (table layout, attach in a tab, archiving); see the README and `docs/superpowers/specs/2026-09-30-devin-board-design.md` for current behavior.

# herdr-devin-board Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A herdr plugin that opens a tab listing my own Devin Cloud sessions with their status and the live GitHub status of the PRs each session opened.

**Architecture:** One Go binary run as a herdr tab pane. `internal/devin` lists sessions from the Devin v3 API using the Devin CLI's stored credentials; `internal/github` enriches open PRs via `gh pr view`; `internal/board` is pure filtering/sorting/classification; `internal/ui` is a Bubble Tea model; `internal/host` shells out to `open` and `herdr` for actions.

**Tech Stack:** Go 1.24+, Bubble Tea v1.3.10, Lip Gloss v1.1.0, go-toml v2, `gh` CLI, herdr 0.9 plugin manifest.

Spec: `docs/superpowers/specs/2026-09-30-devin-board-design.md`

## Verified facts this plan relies on

- Credentials: `~/.local/share/devin/credentials.toml`, keys `windsurf_api_key`, `devin_api_url`.
- `GET /v3/self` → `{"user_id": "...", "devin_sessions_org_id": "org-..."}`. Use `devin_sessions_org_id`, **not** `org_id`.
- `GET /v3/organizations/{org}/sessions?user_ids=<id>&is_archived=false&first=200[&after=<cursor>]` → `{"items":[...], "end_cursor": "...", "has_next_page": bool, "total": n}`. `created_at`/`updated_at` are **unix seconds (int)**. `pr_state` values seen: `open`, `merged`, `closed`.
- `gh pr view <url> --json number,state,isDraft,reviewDecision,statusCheckRollup` → `state` is `OPEN|MERGED|CLOSED`; rollup items are `{"__typename":"CheckRun","name","workflowName","status","conclusion","startedAt"}` or `{"__typename":"StatusContext","context","state","startedAt"}`. Rollups contain repeated runs of the same check, so dedupe to the latest per check.
- `herdr pane split --pane <id> --direction right` prints JSON; new pane id at `.result.pane.pane_id`. `herdr pane run <pane_id> "<command text>"` types the command and presses Enter.
- `herdr plugin pane open --plugin <id> --entrypoint <pane-id>` opens a plugin pane. Plugin processes receive `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PANE_ID`, `HERDR_BIN_PATH`.
- `herdr plugin link <dir>` installs a local plugin for development and does **not** run build commands.

## File structure

```
herdr-devin-board/
├── go.mod
├── .gitignore
├── herdr-plugin.toml               # manifest: build, "open" action, tab pane
├── bin/run                         # pane wrapper (herdr resolves pane commands via PATH)
├── cmd/herdr-devin-board/main.go   # wiring only
├── internal/devin/credentials.go   # LoadCredentials, DefaultCredentialsPath
├── internal/devin/client.go        # Client.Self, Client.ListSessions
├── internal/github/pr.go           # Client.PRStatus, Client.Statuses, CI rollup
├── internal/board/board.go         # Classify, Build, OpenPRURLs
├── internal/host/host.go           # OpenURL, SSH (herdr pane split + run)
├── internal/ui/model.go            # Bubble Tea model, keys, refresh loop
├── internal/ui/view.go             # rendering
├── internal/ui/testdata/board.golden
└── README.md
```

Each `*.go` file has a sibling `*_test.go`.

---

### Task 1: Scaffold the module and manifest

**Files:**
- Create: `go.mod`, `.gitignore`, `herdr-plugin.toml`, `bin/run`, `cmd/herdr-devin-board/main.go`

- [ ] **Step 1: Initialize the module and dependencies**

```bash
cd ~/Developer/herdr-devin-board
go mod init github.com/rigelstpierre/herdr-devin-board
go get github.com/charmbracelet/bubbletea@v1.3.10 github.com/charmbracelet/lipgloss@v1.1.0 github.com/pelletier/go-toml/v2@v2.4.3 github.com/muesli/termenv@v0.16.0
```

- [ ] **Step 2: Write `.gitignore`**

```
/bin/herdr-devin-board
```

- [ ] **Step 3: Write `herdr-plugin.toml`**

```toml
id = "rigelstpierre.devin-board"
name = "Devin Board"
version = "0.1.0"
min_herdr_version = "0.9.0"
description = "My Devin Cloud sessions with status and the live GitHub status of their PRs."
platforms = ["macos", "linux"]

[[build]]
platforms = ["macos", "linux"]
command = ["go", "build", "-o", "bin/herdr-devin-board", "./cmd/herdr-devin-board"]

[[actions]]
id = "open"
title = "Open Devin board"
contexts = ["workspace"]
command = ["bash", "-c", "exec \"${HERDR_BIN_PATH:-herdr}\" plugin pane open --plugin \"$HERDR_PLUGIN_ID\" --entrypoint board"]

[[panes]]
id = "board"
title = "Devin"
placement = "tab"
command = ["bash", "bin/run"]
```

- [ ] **Step 4: Write `bin/run` and make it executable**

```bash
#!/usr/bin/env bash
set -euo pipefail
root="${HERDR_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
exec "$root/bin/herdr-devin-board"
```

Run: `chmod +x bin/run`

- [ ] **Step 5: Write a placeholder `cmd/herdr-devin-board/main.go` so the module builds**

```go
package main

func main() {}
```

- [ ] **Step 6: Verify it builds**

Run: `go build -o bin/herdr-devin-board ./cmd/herdr-devin-board && echo ok`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore herdr-plugin.toml bin/run cmd
git commit -m "chore: scaffold herdr-devin-board module and manifest"
```

---

### Task 2: Load Devin credentials

**Files:**
- Create: `internal/devin/credentials.go`
- Test: `internal/devin/credentials_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package devin_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

func noEnv(string) string { return "" }

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCredentialsReadsCLIFile(t *testing.T) {
	path := writeFile(t, "windsurf_api_key = \"key-1\"\ndevin_api_url = \"https://api.example.test\"\napi_server_url = \"ignored\"\n")

	creds, err := devin.LoadCredentials(path, noEnv)

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "key-1" || creds.APIURL != "https://api.example.test" {
		t.Fatalf("got %+v", creds)
	}
}

func TestLoadCredentialsDefaultsAPIURL(t *testing.T) {
	path := writeFile(t, "windsurf_api_key = \"key-1\"\n")

	creds, err := devin.LoadCredentials(path, noEnv)

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIURL != "https://api.devin.ai" {
		t.Fatalf("got %q", creds.APIURL)
	}
}

func TestLoadCredentialsEnvOverridesFile(t *testing.T) {
	env := map[string]string{"DEVIN_API_KEY": "env-key", "DEVIN_API_URL": "https://env.test"}

	creds, err := devin.LoadCredentials("/does/not/exist", func(k string) string { return env[k] })

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "env-key" || creds.APIURL != "https://env.test" {
		t.Fatalf("got %+v", creds)
	}
}

func TestLoadCredentialsMissingFile(t *testing.T) {
	_, err := devin.LoadCredentials(filepath.Join(t.TempDir(), "nope.toml"), noEnv)

	if !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCredentialsEmptyKey(t *testing.T) {
	path := writeFile(t, "devin_api_url = \"https://api.devin.ai\"\n")

	_, err := devin.LoadCredentials(path, noEnv)

	if !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devin/`
Expected: FAIL — `undefined: devin.LoadCredentials`

- [ ] **Step 3: Implement `internal/devin/credentials.go`**

```go
package devin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const defaultAPIURL = "https://api.devin.ai"

var ErrNoCredentials = errors.New("no Devin credentials found — run `devin auth login`")

type Credentials struct {
	APIKey string
	APIURL string
}

func DefaultCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "devin", "credentials.toml"), nil
}

func LoadCredentials(path string, getenv func(string) string) (Credentials, error) {
	if key := getenv("DEVIN_API_KEY"); key != "" {
		return Credentials{APIKey: key, APIURL: orDefault(getenv("DEVIN_API_URL"), defaultAPIURL)}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrNoCredentials
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("read Devin credentials: %w", err)
	}
	var file struct {
		APIKey string `toml:"windsurf_api_key"`
		APIURL string `toml:"devin_api_url"`
	}
	if err := toml.Unmarshal(data, &file); err != nil {
		return Credentials{}, fmt.Errorf("parse Devin credentials: %w", err)
	}
	if file.APIKey == "" {
		return Credentials{}, ErrNoCredentials
	}
	return Credentials{APIKey: file.APIKey, APIURL: orDefault(file.APIURL, defaultAPIURL)}, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devin/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/devin
git commit -m "feat(devin): load credentials from the Devin CLI file or env"
```

---

### Task 3: Devin API client

**Files:**
- Create: `internal/devin/client.go`
- Test: `internal/devin/client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package devin_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

func newClient(srv *httptest.Server) *devin.Client {
	return devin.NewClient(devin.Credentials{APIKey: "key-123", APIURL: srv.URL}, srv.Client())
}

func TestSelf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/self" {
			t.Errorf("path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key-123" {
			t.Errorf("auth %q", got)
		}
		fmt.Fprint(w, `{"user_id":"user-1","org_id":"org-other","devin_sessions_org_id":"org-s"}`)
	}))
	defer srv.Close()

	self, err := newClient(srv).Self(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if self.UserID != "user-1" || self.SessionsOrgID != "org-s" {
		t.Fatalf("got %+v", self)
	}
}

func TestListSessionsFiltersToSelfAndPaginates(t *testing.T) {
	var calls []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/organizations/org-s/sessions" {
			t.Errorf("path %q", r.URL.Path)
		}
		q := r.URL.Query()
		calls = append(calls, q)
		if q.Get("after") == "" {
			fmt.Fprint(w, `{"items":[{"session_id":"a","url":"https://devin.test/sessions/a","title":"A","status":"running","status_detail":"working","updated_at":100,"pull_requests":[]}],"end_cursor":"c1","has_next_page":true}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"session_id":"b","status":"exit","updated_at":50,"pull_requests":[{"pr_url":"https://github.com/o/r/pull/1","pr_state":"merged"}]}],"end_cursor":"c2","has_next_page":false}`)
	}))
	defer srv.Close()

	sessions, err := newClient(srv).ListSessions(context.Background(), devin.Self{UserID: "user-1", SessionsOrgID: "org-s"})

	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].ID != "a" || sessions[1].ID != "b" {
		t.Fatalf("got %+v", sessions)
	}
	if sessions[0].Title != "A" || sessions[0].StatusDetail != "working" || sessions[0].UpdatedAt != 100 {
		t.Fatalf("fields not decoded: %+v", sessions[0])
	}
	if pr := sessions[1].PullRequests; len(pr) != 1 || pr[0].URL != "https://github.com/o/r/pull/1" || pr[0].State != "merged" {
		t.Fatalf("prs: %+v", pr)
	}
	first := calls[0]
	if first.Get("user_ids") != "user-1" || first.Get("is_archived") != "false" || first.Get("first") != "200" {
		t.Fatalf("first query: %v", first)
	}
	if len(calls) != 2 || calls[1].Get("after") != "c1" {
		t.Fatalf("calls: %v", calls)
	}
}

func TestListSessionsStopsOnRepeatedCursor(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"items":[],"end_cursor":"same","has_next_page":true}`)
	}))
	defer srv.Close()

	_, err := newClient(srv).ListSessions(context.Background(), devin.Self{UserID: "u", SessionsOrgID: "o"})

	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestUnauthorizedTellsUserToLogIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	_, err := newClient(srv).Self(context.Background())

	var apiErr *devin.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "devin auth login") {
		t.Fatalf("message %q", err.Error())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devin/`
Expected: FAIL — `undefined: devin.NewClient`

- [ ] **Step 3: Implement `internal/devin/client.go`**

```go
package devin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(creds Credentials, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(creds.APIURL, "/"), apiKey: creds.APIKey, http: httpClient}
}

type Self struct {
	UserID        string `json:"user_id"`
	SessionsOrgID string `json:"devin_sessions_org_id"`
}

type PullRequest struct {
	URL   string `json:"pr_url"`
	State string `json:"pr_state"`
}

type Session struct {
	ID           string        `json:"session_id"`
	URL          string        `json:"url"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	StatusDetail string        `json:"status_detail"`
	UpdatedAt    int64         `json:"updated_at"`
	PullRequests []PullRequest `json:"pull_requests"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.StatusCode == http.StatusUnauthorized {
		return "Devin API 401 — run `devin auth login`"
	}
	return fmt.Sprintf("Devin API %d: %s", e.StatusCode, e.Body)
}

func (c *Client) Self(ctx context.Context) (Self, error) {
	var self Self
	err := c.get(ctx, "/v3/self", nil, &self)
	return self, err
}

func (c *Client) ListSessions(ctx context.Context, self Self) ([]Session, error) {
	path := "/v3/organizations/" + url.PathEscape(self.SessionsOrgID) + "/sessions"
	var sessions []Session
	cursor := ""
	for {
		query := url.Values{"user_ids": {self.UserID}, "is_archived": {"false"}, "first": {"200"}}
		if cursor != "" {
			query.Set("after", cursor)
		}
		var page struct {
			Items       []Session `json:"items"`
			EndCursor   string    `json:"end_cursor"`
			HasNextPage bool      `json:"has_next_page"`
		}
		if err := c.get(ctx, path, query, &page); err != nil {
			return nil, err
		}
		sessions = append(sessions, page.Items...)
		if !page.HasNextPage || page.EndCursor == "" || page.EndCursor == cursor {
			return sessions, nil
		}
		cursor = page.EndCursor
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Devin API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devin/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/devin
git commit -m "feat(devin): list my sessions from the v3 API with pagination"
```

---

### Task 4: GitHub PR status via `gh`

**Files:**
- Create: `internal/github/pr.go`
- Test: `internal/github/pr_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package github_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

func fakeRunner(outputs map[string]string, calls *[][]string) github.Runner {
	var mu sync.Mutex
	return func(_ context.Context, args ...string) ([]byte, error) {
		mu.Lock()
		*calls = append(*calls, args)
		mu.Unlock()
		out, ok := outputs[args[2]]
		if !ok {
			return nil, errors.New("exit status 1")
		}
		return []byte(out), nil
	}
}

func TestPRStatusParsesGhOutput(t *testing.T) {
	var calls [][]string
	run := fakeRunner(map[string]string{
		"https://github.com/o/r/pull/7": `{"number":7,"state":"OPEN","isDraft":true,"reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-09-30T10:00:00Z"}]}`,
	}, &calls)

	st, err := github.NewClient(run).PRStatus(context.Background(), "https://github.com/o/r/pull/7")

	if err != nil {
		t.Fatal(err)
	}
	want := github.PRStatus{Number: 7, State: "OPEN", IsDraft: true, Review: "REVIEW_REQUIRED", CI: github.CIPassing}
	if st != want {
		t.Fatalf("got %+v want %+v", st, want)
	}
	wantArgs := []string{"pr", "view", "https://github.com/o/r/pull/7", "--json", "number,state,isDraft,reviewDecision,statusCheckRollup"}
	if !reflect.DeepEqual(calls[0], wantArgs) {
		t.Fatalf("args %v", calls[0])
	}
}

func TestPRStatusRunnerError(t *testing.T) {
	var calls [][]string
	_, err := github.NewClient(fakeRunner(nil, &calls)).PRStatus(context.Background(), "https://github.com/o/r/pull/1")

	if err == nil {
		t.Fatal("want error")
	}
}

func TestCIRollup(t *testing.T) {
	cases := []struct {
		name   string
		rollup string
		want   github.CI
	}{
		{"no checks", `[]`, github.CINone},
		{"all passing, skipped and neutral count as passing", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":"SKIPPED"},
			{"__typename":"CheckRun","name":"c","status":"COMPLETED","conclusion":"NEUTRAL"}]`, github.CIPassing},
		{"in progress check", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","name":"b","status":"IN_PROGRESS","conclusion":""}]`, github.CIPending},
		{"failure beats pending", `[
			{"__typename":"CheckRun","name":"a","status":"IN_PROGRESS"},
			{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":"FAILURE"}]`, github.CIFailing},
		{"cancelled is not a failure", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"CANCELLED"}]`, github.CIPassing},
		{"status context failure", `[{"__typename":"StatusContext","context":"ci/semaphore","state":"FAILURE"}]`, github.CIFailing},
		{"status context pending", `[{"__typename":"StatusContext","context":"ci/semaphore","state":"PENDING"}]`, github.CIPending},
		{"rerun supersedes earlier failure", `[
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-09-30T10:00:00Z"},
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-09-30T11:00:00Z"}]`, github.CIPassing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			run := fakeRunner(map[string]string{"u": `{"number":1,"state":"OPEN","statusCheckRollup":` + tc.rollup + `}`}, &calls)

			st, err := github.NewClient(run).PRStatus(context.Background(), "u")

			if err != nil {
				t.Fatal(err)
			}
			if st.CI != tc.want {
				t.Fatalf("CI = %v want %v", st.CI, tc.want)
			}
		})
	}
}

func TestStatusesSkipsFailures(t *testing.T) {
	var calls [][]string
	run := fakeRunner(map[string]string{
		"https://github.com/o/r/pull/1": `{"number":1,"state":"OPEN","statusCheckRollup":[]}`,
	}, &calls)

	got := github.NewClient(run).Statuses(context.Background(), []string{"https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"}, 4)

	if len(got) != 1 || got["https://github.com/o/r/pull/1"].Number != 1 {
		t.Fatalf("got %+v", got)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/github/`
Expected: FAIL — `undefined: github.NewClient`

- [ ] **Step 3: Implement `internal/github/pr.go`**

```go
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

type CI int

const (
	CIUnknown CI = iota
	CINone
	CIPassing
	CIPending
	CIFailing
)

type PRStatus struct {
	Number  int
	State   string
	IsDraft bool
	CI      CI
	Review  string
}

type Runner func(ctx context.Context, args ...string) ([]byte, error)

func GHRunner(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", args...).Output()
}

type Client struct {
	run Runner
}

func NewClient(run Runner) *Client {
	return &Client{run: run}
}

func (c *Client) PRStatus(ctx context.Context, prURL string) (PRStatus, error) {
	out, err := c.run(ctx, "pr", "view", prURL, "--json", "number,state,isDraft,reviewDecision,statusCheckRollup")
	if err != nil {
		return PRStatus{}, fmt.Errorf("gh pr view %s: %w", prURL, err)
	}
	var raw struct {
		Number         int     `json:"number"`
		State          string  `json:"state"`
		IsDraft        bool    `json:"isDraft"`
		ReviewDecision string  `json:"reviewDecision"`
		Rollup         []check `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return PRStatus{}, fmt.Errorf("decode gh output for %s: %w", prURL, err)
	}
	return PRStatus{
		Number:  raw.Number,
		State:   raw.State,
		IsDraft: raw.IsDraft,
		Review:  raw.ReviewDecision,
		CI:      rollupCI(latestPerCheck(raw.Rollup)),
	}, nil
}

func (c *Client) Statuses(ctx context.Context, urls []string, parallel int) map[string]PRStatus {
	statuses := make(map[string]PRStatus, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallel)
	for _, prURL := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			status, err := c.PRStatus(ctx, prURL)
			if err != nil {
				return
			}
			mu.Lock()
			statuses[prURL] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	return statuses
}

type check struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Workflow   string `json:"workflowName"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	StartedAt  string `json:"startedAt"`
}

func (c check) key() string {
	if c.Typename == "StatusContext" {
		return "status:" + c.Context
	}
	return "check:" + c.Workflow + "/" + c.Name
}

func latestPerCheck(checks []check) []check {
	latest := map[string]check{}
	var order []string
	for _, c := range checks {
		key := c.key()
		previous, seen := latest[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || c.StartedAt >= previous.StartedAt {
			latest[key] = c
		}
	}
	result := make([]check, 0, len(order))
	for _, key := range order {
		result = append(result, latest[key])
	}
	return result
}

func rollupCI(checks []check) CI {
	if len(checks) == 0 {
		return CINone
	}
	overall := CIPassing
	for _, c := range checks {
		switch checkResult(c) {
		case CIFailing:
			return CIFailing
		case CIPending:
			overall = CIPending
		}
	}
	return overall
}

func checkResult(c check) CI {
	if c.Typename == "StatusContext" {
		switch c.State {
		case "FAILURE", "ERROR":
			return CIFailing
		case "PENDING", "EXPECTED":
			return CIPending
		}
		return CIPassing
	}
	if c.Status != "COMPLETED" {
		return CIPending
	}
	switch c.Conclusion {
	case "FAILURE", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return CIFailing
	}
	return CIPassing
}
```

- [ ] **Step 4: Run tests to verify they pass (with the race detector)**

Run: `go test -race ./internal/github/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/github
git commit -m "feat(github): fetch PR CI and review status via gh"
```

---

### Task 5: Board — classify, filter, sort, merge PR status

**Files:**
- Create: `internal/board/board.go`
- Test: `internal/board/board_test.go`

- [ ] **Step 1: Write the failing tests**

```go
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
			{URL: "https://github.com/rootlyhq/rootly/pull/10", State: "open"},
			{URL: "https://github.com/rootlyhq/rootly/pull/11", State: "open"},
			{URL: "https://github.com/rootlyhq/docs/pull/12", State: "merged"},
		},
	}}
	statuses := map[string]github.PRStatus{
		"https://github.com/rootlyhq/rootly/pull/10": {Number: 10, State: "OPEN", IsDraft: true, CI: github.CIFailing, Review: "REVIEW_REQUIRED"},
	}

	got := board.Build(sessions, statuses, opts(false))[0]

	if got.Title != "(untitled)" {
		t.Fatalf("title %q", got.Title)
	}
	want := []board.PR{
		{URL: "https://github.com/rootlyhq/rootly/pull/10", Repo: "rootly", Number: 10, State: "draft", CI: github.CIFailing, Review: "REVIEW_REQUIRED"},
		{URL: "https://github.com/rootlyhq/rootly/pull/11", Repo: "rootly", Number: 11, State: "open", Unknown: true},
		{URL: "https://github.com/rootlyhq/docs/pull/12", Repo: "docs", Number: 12, State: "merged"},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/board/`
Expected: FAIL — `undefined: board.Classify`

- [ ] **Step 3: Implement `internal/board/board.go`**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/board/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/board
git commit -m "feat(board): classify, filter, and sort sessions with PR status"
```

---

### Task 6: Host actions — open URL and ssh pane

**Files:**
- Create: `internal/host/host.go`
- Test: `internal/host/host_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package host_test

import (
	"reflect"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/host"
)

type recorder struct {
	calls   [][]string
	outputs [][]byte
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(r.outputs) == 0 {
		return nil, nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil
}

func TestOpenURLUsesOpenOnMac(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "darwin", "", "").OpenURL("https://example.test"); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(r.calls, [][]string{{"open", "https://example.test"}}) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestOpenURLUsesXdgOpenOnLinux(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "linux", "", "").OpenURL("https://example.test"); err != nil {
		t.Fatal(err)
	}

	if r.calls[0][0] != "xdg-open" {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHSplitsBesideBoardAndRunsDevinSSH(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{"pane":{"pane_id":"w1:p9"}}}`)}}

	if err := host.New(r.run, "darwin", "w1:p2", "/opt/herdr").SSH("4f1232c9d91c4214"); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"/opt/herdr", "pane", "split", "--direction", "right", "--pane", "w1:p2"},
		{"/opt/herdr", "pane", "run", "w1:p9", "devin ssh 4f1232c9d91c4214"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHFallsBackToCurrentPaneAndHerdrOnPath(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{"pane":{"pane_id":"p1"}}}`)}}

	if err := host.New(r.run, "darwin", "", "").SSH("abc"); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(r.calls[0], []string{"herdr", "pane", "split", "--direction", "right", "--current"}) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHRejectsUnsafeSessionID(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "darwin", "", "").SSH("abc; rm -rf ~"); err == nil {
		t.Fatal("want error")
	}
	if len(r.calls) != 0 {
		t.Fatalf("ran %v", r.calls)
	}
}

func TestSSHErrorsWhenSplitReturnsNoPane(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{}}`)}}

	if err := host.New(r.run, "darwin", "", "").SSH("abc"); err == nil {
		t.Fatal("want error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/host/`
Expected: FAIL — `undefined: host.New`

- [ ] **Step 3: Implement `internal/host/host.go`**

```go
package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
)

type Runner func(name string, args ...string) ([]byte, error)

func ExecRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Host struct {
	run       Runner
	goos      string
	paneID    string
	herdrPath string
}

func New(run Runner, goos, paneID, herdrPath string) *Host {
	if herdrPath == "" {
		herdrPath = "herdr"
	}
	return &Host{run: run, goos: goos, paneID: paneID, herdrPath: herdrPath}
}

func (h *Host) OpenURL(target string) error {
	opener := "xdg-open"
	if h.goos == "darwin" {
		opener = "open"
	}
	if _, err := h.run(opener, target); err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	return nil
}

func (h *Host) SSH(sessionID string) error {
	if !sessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("refusing to ssh: unexpected session id %q", sessionID)
	}
	paneID, err := h.splitBesideBoard()
	if err != nil {
		return err
	}
	if _, err := h.run(h.herdrPath, "pane", "run", paneID, "devin ssh "+sessionID); err != nil {
		return fmt.Errorf("herdr pane run: %w", err)
	}
	return nil
}

func (h *Host) splitBesideBoard() (string, error) {
	args := []string{"pane", "split", "--direction", "right"}
	if h.paneID != "" {
		args = append(args, "--pane", h.paneID)
	} else {
		args = append(args, "--current")
	}
	out, err := h.run(h.herdrPath, args...)
	if err != nil {
		return "", fmt.Errorf("herdr pane split: %w", err)
	}
	var resp struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode herdr pane split: %w", err)
	}
	if resp.Result.Pane.PaneID == "" {
		return "", errors.New("herdr pane split returned no pane id")
	}
	return resp.Result.Pane.PaneID, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/host/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/host
git commit -m "feat(host): open URLs and ssh into a session in a new herdr pane"
```

---

### Task 7: UI model — state, refresh loop, keys

**Files:**
- Create: `internal/ui/model.go`
- Test: `internal/ui/model_test.go`

The view lives in Task 8; this task adds a minimal `View()` stub so the model satisfies `tea.Model`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/`
Expected: FAIL — `undefined: New`

- [ ] **Step 3: Implement `internal/ui/model.go`**

```go
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
```

- [ ] **Step 4: Add a temporary `internal/ui/view.go` stub**

```go
package ui

func (m Model) View() string { return "" }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): board model with refresh loop and key actions"
```

---

### Task 8: UI view and golden render

**Files:**
- Modify: `internal/ui/view.go` (replace the stub)
- Test: `internal/ui/view_test.go`, `internal/ui/testdata/board.golden`

- [ ] **Step 1: Write the failing golden test**

```go
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
	if !strings.Contains(view, "#23601 rootly  open   ? ?") {
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run View`
Expected: FAIL — golden file missing, and the banner/empty-state assertions fail against the stub.

- [ ] **Step 3: Replace `internal/ui/view.go`**

```go
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

const (
	defaultWidth  = 80
	statusWidth   = 11
	minTitleWidth = 10
	footer        = "[enter] open  [p] open PR  [s] ssh  [a] show all  [r] refresh  [q] quit"
)

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleDim    = lipgloss.NewStyle().Faint(true)
	kindStyles  = map[board.Kind]lipgloss.Style{
		board.Running:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		board.Waiting:   lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		board.Suspended: styleDim,
		board.Finished:  styleDim,
		board.Errored:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
)

func (m Model) View() string {
	sessions := m.sessions()
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.header(len(sessions))) + "\n")
	if m.err != nil {
		b.WriteString(styleError.Render(m.err.Error()) + "\n")
	}
	b.WriteString("\n")
	switch {
	case !m.loaded && m.err == nil:
		b.WriteString(styleDim.Render("Loading sessions…") + "\n")
	case m.loaded && len(sessions) == 0:
		b.WriteString(styleDim.Render(emptyText(m.showAll)) + "\n")
	}
	for i, l := range flatten(sessions) {
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		session := sessions[l.session]
		if l.pr < 0 {
			b.WriteString(prefix + m.sessionLine(session) + "\n")
		} else {
			b.WriteString(prefix + prLine(session.PRs[l.pr]) + "\n")
		}
	}
	b.WriteString("\n" + styleDim.Render(footer) + "\n")
	return b.String()
}

func (m Model) header(count int) string {
	parts := []string{"Devin Cloud", fmt.Sprintf("%d sessions", count)}
	if m.loaded {
		parts = append(parts, "refreshed "+relative(m.deps.Now(), m.refreshedAt))
	}
	if m.loading {
		parts = append(parts, "refreshing…")
	}
	if m.showAll {
		parts = append(parts, "showing all")
	}
	return strings.Join(parts, " · ")
}

func (m Model) sessionLine(s board.Session) string {
	width := m.width
	if width == 0 {
		width = defaultWidth
	}
	ago := relative(m.deps.Now(), s.UpdatedAt)
	titleWidth := max(width-2-statusWidth-2-2-len(ago), minTitleWidth)
	status := kindStyles[s.Kind].Render(fmt.Sprintf("%-*s", statusWidth, kindLabel(s.Kind)))
	return fmt.Sprintf("%s  %-*s  %s", status, titleWidth, truncate(s.Title, titleWidth), ago)
}

func prLine(pr board.PR) string {
	detail := strings.TrimSpace(ciText(pr.CI) + "  " + reviewText(pr.Review))
	if pr.Unknown {
		detail = styleDim.Render("? ?")
	}
	return strings.TrimRight(fmt.Sprintf("    #%d %s  %-6s %s", pr.Number, pr.Repo, pr.State, detail), " ")
}

func kindLabel(k board.Kind) string {
	switch k {
	case board.Waiting:
		return "◐ waiting"
	case board.Suspended:
		return "◌ suspended"
	case board.Finished:
		return "○ finished"
	case board.Errored:
		return "✗ error"
	}
	return "● running"
}

func ciText(ci github.CI) string {
	switch ci {
	case github.CIPassing:
		return "✓ CI"
	case github.CIFailing:
		return "✗ CI"
	case github.CIPending:
		return "⧗ CI"
	}
	return ""
}

func reviewText(review string) string {
	switch review {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review required"
	}
	return ""
}

func emptyText(showAll bool) string {
	if showAll {
		return "No sessions."
	}
	return "No sessions in the last 7 days. [a] show all"
}

func relative(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}
```

- [ ] **Step 4: Generate the golden file and inspect it**

Run: `go test ./internal/ui/ -run TestViewGolden -update && cat internal/ui/testdata/board.golden`
Expected: output shaped like the spec mockup — header `Devin Cloud · 5 sessions · refreshed 12s ago`; the waiting session first with `#23588 rootly  draft  ✗ CI` under it; then running `Fix IR-7158` with `#23601 rootly  open   ✓ CI  review required`; then finished, error (with `#23500 … ? ?`), suspended `Idle research`; footer line. If the shape is wrong, fix `view.go` and regenerate — do not hand-edit the golden.

- [ ] **Step 5: Run the full UI suite**

Run: `go test ./internal/ui/`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): render sessions, PR rows, header banner, and footer"
```

---

### Task 9: Wire `main`

**Files:**
- Modify: `cmd/herdr-devin-board/main.go` (replace placeholder)
- Create: `cmd/herdr-devin-board/loader.go`
- Test: `cmd/herdr-devin-board/loader_test.go`

- [ ] **Step 1: Write the failing loader test**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

func TestLoaderFetchesSelfOnceAndEnrichesOpenPRs(t *testing.T) {
	selfCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/self" {
			selfCalls++
			fmt.Fprint(w, `{"user_id":"u","devin_sessions_org_id":"o"}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"session_id":"s","status":"running","updated_at":1,"pull_requests":[{"pr_url":"https://github.com/o/r/pull/1","pr_state":"open"}]}],"has_next_page":false}`)
	}))
	defer srv.Close()
	t.Setenv("DEVIN_API_KEY", "k")
	t.Setenv("DEVIN_API_URL", srv.URL)
	gh := github.NewClient(func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"number":1,"state":"OPEN","statusCheckRollup":[]}`), nil
	})
	load := newLoader("/unused", gh)

	for range 2 {
		data, err := load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Sessions) != 1 || data.Statuses["https://github.com/o/r/pull/1"].Number != 1 {
			t.Fatalf("data %+v", data)
		}
	}
	if selfCalls != 1 {
		t.Fatalf("self calls = %d", selfCalls)
	}
}

func TestLoaderReportsMissingCredentialsAndRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	load := newLoader(path, github.NewClient(nil))

	if _, err := load(context.Background()); !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}

	if err := os.WriteFile(path, []byte("windsurf_api_key = \"k\"\ndevin_api_url = \"http://127.0.0.1:1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(context.Background()); errors.Is(err, devin.ErrNoCredentials) {
		t.Fatal("loader should re-read credentials after they appear")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/herdr-devin-board/`
Expected: FAIL — `undefined: newLoader`

- [ ] **Step 3: Implement `cmd/herdr-devin-board/loader.go`**

```go
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

const ghParallelism = 4

func newLoader(credentialsPath string, gh *github.Client) func(context.Context) (ui.Data, error) {
	var client *devin.Client
	var self *devin.Self
	return func(ctx context.Context) (ui.Data, error) {
		if client == nil {
			creds, err := devin.LoadCredentials(credentialsPath, os.Getenv)
			if err != nil {
				return ui.Data{}, err
			}
			client = devin.NewClient(creds, &http.Client{Timeout: 20 * time.Second})
		}
		if self == nil {
			fetched, err := client.Self(ctx)
			if err != nil {
				return ui.Data{}, err
			}
			self = &fetched
		}
		sessions, err := client.ListSessions(ctx, *self)
		if err != nil {
			return ui.Data{}, err
		}
		return ui.Data{
			Sessions: sessions,
			Statuses: gh.Statuses(ctx, board.OpenPRURLs(sessions), ghParallelism),
		}, nil
	}
}
```

- [ ] **Step 4: Replace `cmd/herdr-devin-board/main.go`**

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/host"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

const refreshInterval = 30 * time.Second

func main() {
	credentialsPath, err := devin.DefaultCredentialsPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
	h := host.New(host.ExecRunner, runtime.GOOS, os.Getenv("HERDR_PANE_ID"), os.Getenv("HERDR_BIN_PATH"))
	model := ui.New(ui.Deps{
		Load:     newLoader(credentialsPath, github.NewClient(github.GHRunner)),
		OpenURL:  h.OpenURL,
		SSH:      h.SSH,
		Now:      time.Now,
		Interval: refreshInterval,
	})
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-devin-board:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run the whole suite, vet, and build**

Run: `go test -race ./... && go vet ./... && go build -o bin/herdr-devin-board ./cmd/herdr-devin-board && echo built`
Expected: all packages `ok`, no vet output, `built`

- [ ] **Step 6: Commit**

```bash
git add cmd
git commit -m "feat: wire loader, host actions, and UI into the binary"
```

---

### Task 10: Live verification in herdr

No code; this proves the plugin against the real account. Record anything that diverges and fix it with a test before moving on.

- [ ] **Step 1: Smoke-test the binary outside herdr**

Run: `./bin/herdr-devin-board` in a terminal.
Expected: header shows your session count (7 non-archived at time of writing), waiting sessions first, PR rows with CI glyphs for open PRs. Press `a`, `r`, `j`/`k`, `enter` (browser opens the session), then `q`.

- [ ] **Step 2: Link the plugin into herdr**

Run: `herdr plugin link ~/Developer/herdr-devin-board && herdr plugin list`
Expected: `rigelstpierre.devin-board` listed and enabled.

- [ ] **Step 3: Open the board via the action**

Run: `herdr plugin action invoke rigelstpierre.devin-board.open`
Expected: a new tab titled `Devin` showing the board. If nothing appears, run `herdr plugin log list --plugin rigelstpierre.devin-board` and fix the manifest command.

- [ ] **Step 4: Exercise `s` (ssh) inside herdr**

Select a running session and press `s`.
Expected: a pane opens to the right of the board running `devin ssh <id>`. If the split JSON shape differs from `.result.pane.pane_id`, update `host.splitBesideBoard` and its test to the real shape.

- [ ] **Step 5: Exercise the failure paths**

Run the board with `DEVIN_API_KEY=bogus ./bin/herdr-devin-board`.
Expected: red banner `Devin API 401 — run devin auth login`, no crash.
Run with `PATH=/usr/bin:/bin ./bin/herdr-devin-board` (no `gh`).
Expected: sessions still listed; open PRs show `? ?`.

---

### Task 11: README and GitHub repo

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write `README.md`**

````markdown
# herdr-devin-board

A [herdr](https://herdr.dev) plugin that shows your own Devin Cloud sessions in a tab: each session's status, plus the live GitHub status (CI, review, draft) of the PRs it opened.

## Requirements

- herdr 0.9+
- Go 1.24+ (herdr builds the plugin on install)
- Devin CLI, logged in (`devin auth login`) — the board reuses its stored credentials
- `gh`, logged in, for PR CI and review status

## Install

```bash
herdr plugin install rigelstpierre/herdr-devin-board
```

Then run the **Open Devin board** action.

## Keys

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | Move |
| `enter` | Open the session (or the PR on a PR row) in the browser |
| `p` | Open the session's first PR |
| `s` | `devin ssh` into the session in a new pane |
| `a` | Show all (include finished, and suspended without an open PR, older than 7 days) |
| `r` | Refresh now (auto-refreshes every 30s) |
| `q` | Quit |

## Configuration

`DEVIN_API_KEY` (and optionally `DEVIN_API_URL`) override the Devin CLI credentials file.

## Development

```bash
go test -race ./...
go build -o bin/herdr-devin-board ./cmd/herdr-devin-board
herdr plugin link .
```
````

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README"
```

- [ ] **Step 3: Create the private GitHub repo and push**

```bash
gh repo create rigelstpierre/herdr-devin-board --private --source . --push
```

Expected: repo created, `master` (or `main`) pushed. Note `herdr plugin install` against a private repo needs `gh`/git credentials; `herdr plugin link` keeps working locally regardless.
