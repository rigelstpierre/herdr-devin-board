# herdr-devin-board

A [herdr](https://herdr.dev) plugin that lists your own Devin Cloud sessions in a tab, with each session's status and the live GitHub status (CI, review, draft) of the PRs it opened. From the board you can attach to a session in a new herdr tab, open it in the browser, `ssh` into its box, or archive it.

```
Devin Sessions
8 sessions · 1 waiting · 2 running · 5 suspended

STATUS       SESSION                          REPO    PR         CI  REVIEW           UPDATED
──────────────────────────────────────────────────────────────────────────────────────────────
◐ waiting    Resolve support ticket           api     #4207      ✓   review required       3m
● running    Fix login redirect               api     #4208      ●   review required       7m
◌ suspended  Upgrade billing SDK              web     #912 +1    –   merged                1h
◌ suspended  Investigate flaky spec           –       –          –   –                     1d
──────────────────────────────────────────────────────────────────────────────────────────────
https://app.devin.ai/sessions/4f1232c9…
◐ waiting · waiting for user · updated 3m ago
1  api #4207 · open · CI passing · review required  https://github.com/acme/api/pull/4207

Enter tab · o web · ↑↓ select · p PR · 1-9 nth PR · s ssh · x archive · a all · r refresh · q quit
refreshed 12s ago · last 7 days
```

## Requirements

- herdr 0.9+ on macOS or Linux
- Go 1.24+ (herdr builds the plugin on install)
- [Devin CLI](https://docs.devin.ai), logged in with `devin auth login`. The board reuses its stored credentials to list your sessions.
- [`gh`](https://cli.github.com), logged in, for PR CI and review status. Without it the board still works, and PR columns show `?`.

## Install

```bash
herdr plugin install rigelstpierre/herdr-devin-board
```

Then run the **Open Devin board** action. The board opens in a tab named **Devin Sessions**.

## What it shows

- **Your sessions only**, newest first, with sessions waiting on you sorted to the top.
- **Hidden by default after 7 days:** finished sessions, and suspended sessions with no open PR. Running, waiting, and errored sessions always show. Press `a` to show everything. Archived sessions never show.
- **PR columns** show the session's first PR (`+1` means it has more). CI and review come from `gh`. Merged and closed PRs use Devin's own state.
- **The detail panel** under the table shows the selected session's link, status, and every PR with its link.
- **Refreshes** every 30 seconds while the tab is open, and on `r`. A failed refresh keeps the last good data and shows the error in the status line.

## Keys

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | Move between sessions |
| `Enter` | Attach to the session in a new herdr tab (`devin --cloud --resume <id>`). Pressing it again focuses that tab while it's open. |
| `o` | Open the session in the browser |
| `p` | Open the session's first PR |
| `1`–`9` | Open the session's nth PR, as numbered in the detail panel |
| `s` | `devin ssh` into the session's box in a new pane beside the board |
| `x` | Archive the session; confirm with `y`. Archiving also puts a running session to sleep. Needs an API key, see below. |
| `a` | Toggle showing all sessions |
| `r` | Refresh now |
| `q` | Quit |

Attach tabs open in the workspace's directory, so Devin's workspace-trust prompt doesn't block them. If your workspace directory isn't one Devin trusts yet, you'll get the prompt once.

## Archiving

Devin's archive endpoint rejects the CLI login token, so `x` needs a `cog_` key with the `ManageOrgSessions` permission. Legacy `apk_` keys only work on the v1 API, which has no archive endpoint. Create either:

- **Service user**: Settings → Devin API → Service users → Provision service user. This needs permission to manage service users, usually an admin.
- **Personal access token**: Settings → Devin API → PATs. Enterprise orgs must enable PATs first, and the token acts with your own permissions.

Save the key in the plugin's config directory:

```bash
dir="$(herdr plugin config-dir rigelstpierre.devin-board)"
mkdir -p "$dir" && pbpaste > "$dir/api_key" && chmod 600 "$dir/api_key"
```

The key is read on every archive, so no restart is needed. Listing keeps using the Devin CLI login.

## Configuration

| Setting | Effect |
|---|---|
| `<config dir>/api_key` | Devin API key used only for archiving |
| `DEVIN_API_KEY`, `DEVIN_API_URL` | Override the Devin CLI credentials used for listing |

## Troubleshooting

| You see | Meaning |
|---|---|
| `Devin API 401/403 — run devin auth login` | The CLI login expired or was revoked. Log in again; the board picks up new credentials on the next refresh. |
| `?` in the CI or REVIEW column | `gh` isn't installed or logged in, or the lookup failed |
| `Devin API key can't archive` | The key in `api_key` is missing, a legacy `apk_` key, or lacks `ManageOrgSessions` |
| A "trust this directory?" prompt in an attach tab | The workspace directory isn't trusted by Devin yet. Answer once. |

## Development

```bash
go test -race ./...
go build -o bin/herdr-devin-board ./cmd/herdr-devin-board
herdr plugin link .
```

`herdr plugin link` does not run build commands, so rebuild after changes. The design notes are in `docs/superpowers/specs/`.

## License

[MIT](LICENSE)
