# herdr-devin-board — Design

Date: 2026-09-30
Status: Approved (brainstorming)

## Goal

A herdr plugin that shows **my own** Devin Cloud sessions in a dashboard tab:
each session's status, plus the GitHub PRs that session opened with live CI and
review state.

Non-goals: other users' sessions, write actions against Devin (send message,
sleep, terminate), a background sidebar badge, notifications.

## Prior art

No existing herdr plugin covers Devin Cloud. The built-in `devin` integration,
`herdr-session-titles`, and `herdr-agent-usage` all handle local Devin CLI
sessions only. `cdowell09/herdr-pr-board` is the structural reference (Go +
Bubble Tea, tab pane, `bin/run` wrapper) but is not forked — it is ~33k lines
of mostly agent-review code.

## Verified facts (probed 2026-09-30)

- The Devin CLI stores credentials in `~/.local/share/devin/credentials.toml`
  with keys `windsurf_api_key` and `devin_api_url` (`https://api.devin.ai`).
- That key authenticates against the public v3 API as a bearer token:
  - `GET /v3/self` → `user_id`, `devin_sessions_org_id` (note: this differs
    from `org_id`; sessions live under `devin_sessions_org_id`).
  - `GET /v3/organizations/{devin_sessions_org_id}/sessions` → 200.
- Session items include `session_id`, `url`, `title`, `status`
  (`new|claimed|running|exit|error|suspended|resuming`), `status_detail`
  (`working|waiting_for_user|waiting_for_approval|finished|inactivity|…`),
  `user_id`, `pull_requests[]` (`pr_url`, `pr_state`), `created_at`,
  `updated_at`, `tags`.
- The list endpoint supports `user_ids`, `is_archived`, `updated_after`,
  `first` (max 200) and cursor pagination via `after` / `end_cursor` /
  `has_next_page`.

## Architecture

Single Go binary, `herdr-devin-board`, built by herdr on install.

```
herdr action "Open Devin board" ─► tab pane runs bin/run ─► herdr-devin-board
                                                │
                    every 30s / on `r` ─────────┤
                                                ▼
  devin:   load credentials → GET /v3/self (cached for process lifetime)
           GET /v3/organizations/{org}/sessions
               ?user_ids=<me>&is_archived=false&first=200
               (follow cursor until has_next_page=false; the 7-day window is
               applied client-side because unfinished sessions of any age stay)
                                                │
  github:  for each PR whose Devin pr_state is open →
           gh pr view <url> --json number,state,isDraft,reviewDecision,
                                   statusCheckRollup
           (bounded parallelism: 4)
                                                ▼
  board:   sessions + PR statuses → ordered rows → ui renders
```

### Units

| Package | Responsibility | Depends on |
|---|---|---|
| `internal/devin` | Credential loading (`DEVIN_API_KEY` env overrides the file), `Self()`, `ListMySessions()` with pagination | `net/http`, `go-toml` |
| `internal/github` | `PRStatus(ctx, url)` via `gh pr view`; command runner is injectable | `os/exec` |
| `internal/board` | Pure: filter, sort, classify status, build rows | nothing external |
| `internal/ui` | Bubble Tea model, refresh loop, keybindings, rendering | `bubbletea`, `lipgloss` |
| `cmd/herdr-devin-board` | Wiring and the cached Devin loader | all of the above |

### Plugin manifest

- `[[build]]` macOS/Linux: `go build -o bin/herdr-devin-board ./cmd/herdr-devin-board`.
- `[[actions]] id="open"`, title "Open Devin board", context `workspace`;
  runs `herdr plugin pane open --plugin $HERDR_PLUGIN_ID --entrypoint board`.
- `[[panes]] id="board"`, placement `tab`, command `["bash", "bin/run"]`
  (herdr 0.8+ on Unix resolves pane commands via PATH, hence the wrapper).
- macOS and Linux only.

## Filtering and ordering

Default view: every session that is not finished, plus finished sessions with
`updated_at` in the last 7 days. Archived sessions are excluded server-side.
`a` toggles "show all" (drops the 7-day window).

Order: waiting sessions first, then by `updated_at` descending.

## Status classification

Evaluated top to bottom; first match wins.

| Display | Rule |
|---|---|
| `✗ error` (red) | `status = error` |
| `◐ waiting` (yellow) | `status_detail ∈ {waiting_for_user, waiting_for_approval}` |
| `○ finished` (dim) | `status = exit` or `status_detail = finished` |
| `◌ suspended` (dim) | `status ∈ {suspended, resuming}` |
| `● running` (green) | anything else (`new`, `claimed`, `running`) |

## Rows

Session row: status glyph + label, title (truncated to width), relative
`updated_at`.

PR rows, indented under their session: `#<number>`, repo name (parsed from the PR URL), state
(`open|draft|merged|closed`), CI (`✓` all passing, `✗` any failing, `⧗` any
pending, blank if no checks), review (`approved`, `changes requested`,
`review required`, blank if none).

Merged and closed PRs use Devin's `pr_state` and skip the `gh` call.

## Keybindings

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | Move cursor across session and PR rows |
| `enter` | Open the selected session URL, or the PR URL on a PR row (`open`) |
| `p` | Open the selected session's first PR |
| `s` | `herdr pane split` beside the board, then run `devin ssh <session_id>` in it |
| `a` | Toggle show-all |
| `r` | Refresh now |
| `q` | Quit (closes the tab) |

## Refresh and errors

- Refresh on open, every 30s while open, and on `r`. One refresh in flight at
  a time; a manual refresh during an in-flight one is ignored.
- A failed refresh keeps the last good snapshot and shows a red header banner
  with the cause (e.g. `Devin API 401 — run devin auth login`).
- Missing or unreadable credentials: an empty state that says to run
  `devin auth login`.
- `gh` missing, unauthenticated, or a single lookup failing: the PR row falls
  back to Devin's `pr_state` with a dim `?` in the CI and review columns.
  A GitHub failure never hides a session.
- PR statuses are fetched once per refresh cycle; only open PRs are queried.

## Testing

- `board`: table-driven tests over fixture sessions for filtering, ordering,
  and every status classification rule.
- `devin`: `httptest` server covering auth header, `user_ids` filter,
  pagination across two pages, 401 handling, credential file parsing and the
  env override.
- `github`: fake command runner returning recorded `gh` JSON, including
  failing, pending, and no-check rollups and a non-zero exit.
- `ui`: one golden-text render of a fixture snapshot.
- Manual: install from the local path into herdr and confirm live sessions,
  PR status, `enter`, `p`, and `s` against the real account.

## Repository

`~/Developer/herdr-devin-board`, its own git repo; private GitHub repo under
`rigelstpierre`.
