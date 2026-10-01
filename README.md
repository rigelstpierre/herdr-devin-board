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
| `j`/`k`, `↑`/`↓` | Move between sessions |
| `enter` | Attach to the session in a new herdr tab (`devin --cloud --resume`); focuses it if already open |
| `o` | Open the session in the browser |
| `p` | Open the session's first PR |
| `1`–`9` | Open the session's nth PR (listed in the detail panel) |
| `s` | `devin ssh` into the session in a new pane |
| `x` | Archive the session (asks `y` to confirm; sleeps it if running) |
| `a` | Show all (include finished, and suspended without an open PR, older than 7 days) |
| `r` | Refresh now (auto-refreshes every 30s) |
| `q` | Quit |

## Archiving

Devin's archive endpoint rejects the CLI login token, so `x` needs a `cog_` key with
the `ManageOrgSessions` permission. Legacy `apk_` keys only work on the v1 API, which has no
archive endpoint. Either:

- **Service user**: Settings → Devin API → Service users → Provision service user (needs
  permission to manage service users, usually an admin), or
- **Personal access token**: Settings → Devin API → PATs (enterprise orgs must enable PATs
  first; the token acts with your own permissions).

Put the key in the plugin's config dir:

```bash
dir="$(herdr plugin config-dir rigelstpierre.devin-board)"
mkdir -p "$dir" && pbpaste > "$dir/api_key" && chmod 600 "$dir/api_key"
```

The key is read on every archive, so no restart is needed. Listing keeps using the Devin
CLI login.

## Configuration

`DEVIN_API_KEY` (and optionally `DEVIN_API_URL`) override the Devin CLI credentials file.

## Development

```bash
go test -race ./...
go build -o bin/herdr-devin-board ./cmd/herdr-devin-board
herdr plugin link .
```
