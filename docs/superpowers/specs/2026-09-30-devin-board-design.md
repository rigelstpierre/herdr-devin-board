# herdr-devin-board — Design

Date: 2026-09-30
Status: Current. Updated to match the shipped behavior; the original v0.1 plan is in
`docs/superpowers/plans/`.

## Goal

A herdr plugin that shows **my own** Devin Cloud sessions in a dashboard tab:
each session's status, plus the GitHub PRs that session opened with live CI and
review state. From the board you can attach to a session in a herdr tab, open it
in the browser, `ssh` into its box, or archive it.

Non-goals: other users' sessions, sending messages to sessions, a background
sidebar badge, notifications.

## Prior art

No existing herdr plugin covers Devin Cloud. The built-in `devin` integration,
`herdr-session-titles`, and `herdr-agent-usage` all handle local Devin CLI
sessions only. `cdowell09/herdr-pr-board` is the structural and visual reference
(Go + Bubble Tea, tab pane, `bin/run` wrapper, table plus detail panel) but is not
forked.

## Verified facts (probed 2026-09-30)

- The Devin CLI stores credentials in `~/.local/share/devin/credentials.toml`
  with keys `windsurf_api_key` and `devin_api_url` (`https://api.devin.ai`).
- That key authenticates against the public v3 API as a bearer token for reads:
  - `GET /v3/self` → `user_id`, `devin_sessions_org_id` (this differs from
    `org_id`; sessions live under `devin_sessions_org_id`).
  - `GET /v3/organizations/{devin_sessions_org_id}/sessions` → 200.
  - A bad key gets 403, not 401.
- The same token is refused for single-session GET and for archiving:
  `403 "This endpoint is not accessible with a Windsurf session token"`.
- `POST /v3/organizations/{org}/sessions/{id}/archive` works with a `cog_` key
  (service user or PAT) holding `ManageOrgSessions`, and accepts the id both bare
  and with the `devin-` prefix. Legacy `apk_` keys only work on v1, which has no
  archive endpoint. `/v3/self` for a service user has no `user_id`.
- Session items include `session_id`, `url`, `title`, `status`
  (`new|claimed|running|exit|error|suspended|resuming`), `status_detail`
  (`working|waiting_for_user|waiting_for_approval|finished|inactivity|…`),
  `user_id`, `pull_requests[]` (`pr_url`, `pr_state`), and `created_at` /
  `updated_at` as unix seconds.
- The list endpoint supports `user_ids`, `is_archived`, `first` (max 200) and
  cursor pagination via `after` / `end_cursor` / `has_next_page`.
- `devin --cloud --resume <session_id>` attaches the terminal client to a cloud
  session using the bare id.

## Architecture

Single Go binary, `herdr-devin-board`, built by herdr on install.

```
herdr action "Open Devin board" ─► tab pane runs bin/run ─► herdr-devin-board
                                   (bin/run renames the tab "Devin Sessions")
                                                │
                    every 30s / on `r` ─────────┤
                                                ▼
  devin:   CLI credentials → GET /v3/self (cached; dropped after a 401/403)
           GET /v3/organizations/{org}/sessions
               ?user_ids=<me>&is_archived=false&first=200
               (follow the cursor; the 7-day window is applied client-side)
                                                │
  github:  for each PR whose Devin pr_state is open →
           gh pr view <url> --json number,state,isDraft,reviewDecision,
                                   statusCheckRollup
           (bounded parallelism: 4)
                                                ▼
  board:   sessions + PR statuses → ordered rows → ui renders

  actions: Enter → herdr tab create (workspace cwd) + pane run "devin --cloud --resume <id>"
           s     → herdr pane split + pane run "devin ssh <id>"
           o/p/n → open / xdg-open the session or PR URL
           x → y → read <config dir>/api_key → POST …/sessions/devin-<id>/archive
```

### Units

| Package | Responsibility | Depends on |
|---|---|---|
| `internal/devin` | CLI credential loading (`DEVIN_API_KEY` overrides), `Self()`, paginated `ListSessions()`, `Archive()`, archive key loading | `net/http`, `go-toml` |
| `internal/github` | `PRStatus(ctx, url)` via `gh pr view`, CI rollup with per-check dedupe; injectable runner | `os/exec` |
| `internal/board` | Pure: classify, filter, sort, merge PR status into rows | nothing external |
| `internal/host` | Side effects in herdr and the OS: open URLs, ssh pane, attach tab with per-session tab reuse | `os/exec` |
| `internal/ui` | Bubble Tea model, refresh loop, keys, confirm prompt, table rendering | `bubbletea`, `lipgloss`, `x/ansi` |
| `cmd/herdr-devin-board` | Wiring; `service` holds the cached CLI client and identity for `Load` and `Archive` | all of the above |

### Plugin manifest

- `[[build]]` macOS/Linux: `go build -o bin/herdr-devin-board ./cmd/herdr-devin-board`.
- `[[actions]] id="open"`, title "Open Devin board", context `workspace`; runs
  `herdr plugin pane open --plugin ${HERDR_PLUGIN_ID:-rigelstpierre.devin-board} --entrypoint board --focus`.
- `[[panes]] id="board"`, title "Devin Sessions", placement `tab`, command
  `["bash", "bin/run"]` (herdr on Unix resolves pane commands via PATH, hence the
  wrapper). `bin/run` renames `HERDR_TAB_ID` to "Devin Sessions", best-effort.
- macOS and Linux only.

## Filtering and ordering

Default view: every session updated in the last 7 days, plus older sessions
that are still live. Older than 7 days, a session is hidden when it is
finished, or suspended with no open PR. Running, waiting, and errored sessions
are never hidden by age. Archived sessions are excluded server-side.
`a` toggles "show all" (drops the 7-day window).

Order: waiting sessions first, then by `updated_at` descending. Rows are only
rebuilt when new data arrives, so a session aging past the window can't move the
cursor between refreshes.

## Status classification

Evaluated top to bottom; first match wins.

| Display | Rule |
|---|---|
| `✗ error` (red) | `status = error` |
| `◐ waiting` (yellow) | `status_detail ∈ {waiting_for_user, waiting_for_approval}` |
| `○ finished` (dim) | `status = exit` or `status_detail = finished` |
| `◌ suspended` (dim) | `status ∈ {suspended, resuming}` |
| `● running` (green) | anything else (`new`, `claimed`, `running`) |

## Layout

Modeled on PR Board: title, summary counts, a table, a detail panel, and a
footer pinned to the bottom of the pane.

- **Table:** one row per session with columns STATUS, SESSION, REPO, PR, CI,
  REVIEW, UPDATED. SESSION takes the remaining width. PR shows the first PR, with
  `+N` when there are more. The selected row is drawn in reverse video across the
  full width. REPO and then REVIEW are dropped on narrow panes; any remaining
  overflow is truncated by display width.
- **CI cell:** `✓` passing, `✕` failing, `●` pending, `–` none, merged, or closed,
  `?` when the `gh` lookup failed. Reruns supersede earlier runs of the same
  check, and a queued rerun counts as pending.
- **REVIEW cell:** `approved`, `changes requested`, `review required`, `draft`,
  `merged`, `closed`, `–`, or `?`.
- **Detail panel** (5 lines): the session URL, status with humanized detail and
  relative update time, then three lines of numbered PRs with state, CI, review,
  and URL. With more than three PRs, the third line becomes "… N more".
- **Footer:** key hints, then a status line that shows refresh time and window,
  the archive confirm prompt, or the latest error.
- The view scrolls to keep the cursor visible, and the cursor follows the
  selected session across refreshes that reorder rows.

## Keybindings

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | Move between sessions |
| `Enter` | Attach in a herdr tab named `Devin · <title>`, created in the workspace cwd from `HERDR_PLUGIN_CONTEXT_JSON` so Devin's trust prompt doesn't block it; focuses the existing tab if it is still open |
| `o` | Open the session URL |
| `p` | Open the first PR |
| `1`–`9` | Open the nth PR |
| `s` | `herdr pane split` beside the board, then `devin ssh <session_id>` |
| `x` | Ask to archive; `y` archives, any other key cancels without acting |
| `a` | Toggle show-all |
| `r` | Refresh now (ignored while a refresh is in flight) |
| `q` | Quit |

URLs are only opened when their scheme is http or https. Session ids are checked
against `^[A-Za-z0-9_-]+$` before being put into any typed command.

## Refresh and errors

- Refresh on open, every 30s while open, and on `r`. One refresh in flight at
  a time.
- A failed refresh keeps the last good snapshot and shows the cause in the
  status line (e.g. `Devin API 403 — run devin auth login`).
- Missing credentials, or a 401/403: credentials are re-read on the next
  refresh, so logging in recovers without reopening the tab.
- `gh` missing, unauthenticated, or a single lookup failing: the PR falls back to
  Devin's `pr_state` and shows `?`. A GitHub failure never hides a session.
- Archive without a key, or with a key that can't archive, explains which key type
  is needed instead of suggesting `devin auth login`.
- On a successful archive the row is removed immediately and a refresh starts.

## Testing

- `board`: table-driven tests for classification, filtering (including the
  suspended/open-PR rule), ordering, PR URL parsing, and GitHub-over-Devin state.
- `devin`: `httptest` coverage of auth, filters, pagination and cursor loops,
  401/403 messages, archive path and errors, and credential and key loading.
- `github`: fake runner with recorded rollups, including reruns, queued reruns,
  status contexts, and failures.
- `host`: recorded argv for open, ssh, and attach (create, reuse, reopen after
  close, label truncation, unsafe ids).
- `ui`: model tests for keys, confirm and cancel, selection across reorders,
  scrolling, and refresh scheduling; view tests for columns, alignment, width,
  the detail panel, the pinned footer, and a golden render.
- Manual: verified in herdr against a real account — listing, PR status, attach,
  ssh, the archive prompt, and the failure paths.
