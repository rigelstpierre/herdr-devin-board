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
