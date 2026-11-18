# lmao: plan

`lmao` is a terminal UI for reviewing unreviewed transactions in
[Lunch Money](https://lunchmoney.app) through its
[v2 API](https://developers.lunchmoney.app/introduction). It is written in Go,
with the same structure and libraries as `clonager`.

## Goals

- `lmao` opens a full-screen view of every unreviewed transaction.
- The list on the left shows date, payee, category and amount. Up and down
  arrows move the selection.
- A panel on the right shows every field of the selected transaction.
- Keys:
  - `r` marks the transaction reviewed.
  - `c` sets its category, which also marks it reviewed.
  - `n` edits its notes.
  - `u` undoes the review.
- A transaction stays in the list after it is reviewed, in green, until the
  next reload, so a review can be undone.

## Stack

- Go, with **Cobra** for commands and **Bubble Tea v2**, **Lip Gloss v2** and
  **Bubbles v2** (`table`/`list`, `viewport`, `textarea`, `textinput`) for the
  TUI. The versions match clonager's `go.mod`.
- `net/http` and `encoding/json` from the standard library for the API. There
  is no generated client.
- Colours are basic ANSI (0–7), so they follow the terminal theme, as in
  clonager.

## Layout

```
main.go                      → os.Exit(cmd.Execute())
cmd/root.go                  Cobra root; a bare `lmao` runs the TUI; exit codes
cmd/browse.go                Bubble Tea model (list, detail, modes, key handling)
cmd/browse_test.go           model tests driven by key messages against a fake client
cmd/style.go                 Lip Gloss styles
cmd/version.go               version reporting (later, with releases)
internal/lunchmoney/
  client.go                  Client, request helper, errors, rate-limit handling
  transactions.go            Transaction type, List/Update
  categories.go              Category type, List
  client_test.go             tests against an httptest server
internal/auth/
  auth.go                    TokenSource interface; env var implementation
CLAUDE.md                    commands, layout and design decisions
README.md                    user-facing docs
```

## API usage

Base URL: `https://api.lunchmoney.dev/v2`. Every request sends
`Authorization: Bearer <token>`.

| Purpose | Request |
| --- | --- |
| Unreviewed transactions | `GET /transactions?status=unreviewed&limit=1000&offset=N`, following `has_more` |
| Assignable categories | `GET /categories?is_group=false` (plus groups, for showing `Group › Category`) |
| Accounts (for names in the detail panel) | `GET /manual_accounts`, `GET /plaid_accounts` |
| Review / unreview | `PUT /transactions/:id` `{"status": "reviewed" \| "unreviewed"}` |
| Set category | `PUT /transactions/:id` `{"category_id": N, "status": "reviewed"}` |
| Set notes | `PUT /transactions/:id` `{"notes": "..."}` |

Things to confirm against the OpenAPI spec
(`github.com/lunch-money/lunch-money-js-v2`) before writing the client:

- The values the `status` query parameter accepts. If it can't filter on
  `unreviewed`, the client filters on the `status` field itself.
- The exact field names allowed in a `PUT` body. The API returns 400 for
  unknown fields.
- The account endpoints and the fields they return.
- Whether a date range is needed to reach old unreviewed transactions. If it
  is, pass a wide `start_date`.

Details that shape the code:

- Amounts are decimal strings, and positive means money going out. Keep them as
  strings or as `int64` minor units, never `float64`. Display them with the
  transaction's `currency`.
- Transactions carry only IDs for category, account and tags. Fetch those
  lists once at startup and resolve names locally.
- Split parents and grouped children are left out by default. Review the
  transactions the API returns and don't request the hidden ones.
- Rate limit: 100 requests a minute. On a 429, wait for `Retry-After` and
  retry once, and show the wait in the status line.
- Errors come back as `{"message", "errors": [{"errMsg"}]}`. Show `errMsg`
  in the status line.

## Auth

- `internal/auth` defines `TokenSource` with one method,
  `Token(ctx) (string, error)`. The client asks it for a token on every
  request, so a refreshing OAuth source can replace the env var source
  without changes elsewhere.
- The first implementation reads `LUNCHMONEY_TOKEN`. If it is missing, `lmao`
  exits with a message that says where to create a token
  (`https://my.lunchmoney.app/developers`).
- Lunch Money plans to add OAuth. When it arrives, add an `OAuthSource`
  (browser login, token stored in the Keychain, refresh) and an `lmao login`
  command.
- `LUNCHMONEY_API_URL` overrides the base URL, for example to point at the
  mock server (`https://mock.lunchmoney.dev/v2`).

## TUI

### Layout

```
┌ Unreviewed (42) ─────────────────────────────┐┌ Transaction ───────────────┐
│ 2026-11-14  Tesco Express    Groceries   12.40 ││ Payee     Tesco Express    │
│ 2026-11-13  TfL Travel       Transport    2.80 ││ Amount    £12.40 (debit)   │
│▶2026-11-12  Pret A Manger    —            6.15 ││ Date      2026-11-12       │
│ 2026-11-12  Salary           Income  -2,100.00 ││ Category  —                │
│ ...                                           ││ Account   Monzo Current    │
└───────────────────────────────────────────────┘│ Notes                      │
 r review  c category  n notes  u undo  q quit    │ ...                        │
                                                  └────────────────────────────┘
```

- Left pane: about 60% of the width. Payee takes up the spare space, and
  category is cut short before payee is. Amounts are right-aligned, and money
  coming in has its own colour. Reviewed rows are green. A row with a save in
  progress shows a spinner or a dimmed marker.
- Right pane: a `viewport` with every field, names in place of IDs. The main
  fields (payee, amount, date, category, account, notes, status) come first.
  The rest follow (original name, tags, source, external ID, created and
  updated times, recurring ID, Plaid metadata, custom metadata, files).
- Footer: key hints, then the status line for errors and rate-limit waits.
- When the window is too narrow for two panes (under about 90 columns), the
  detail panel goes below the list.

### Modes

- **Browse** (default):
  - `↑`/`↓`/`k`/`j` move the selection, `PgUp`/`PgDn`/`g`/`G` jump.
  - `r` marks the transaction reviewed and moves to the next unreviewed row.
  - `u` undoes the review (below).
  - `c` opens the category picker, `n` opens the notes editor.
  - `R` reloads from the API.
  - `q`/`Ctrl-C` quits.
- **Category picker:** a `list` that narrows as you type, over categories
  shown as `Group › Category`, with archived ones left out. The current
  category is preselected. `Enter` sets the category and marks the
  transaction reviewed in one `PUT`, then moves to the next unreviewed row.
  `Esc` cancels.
- **Notes editor:** a `textarea` filled with the current notes. `Enter` saves
  and `Alt-Enter` adds a new line. `Esc` cancels. Saving notes doesn't change
  the review status.

### Undo

- `u` on a row reviewed in this session restores what it was before. For a
  plain review, that is `status: unreviewed`. For a category change, it is
  `status: unreviewed` and the previous `category_id`, in one `PUT`.
- The model keeps a snapshot of each row's status and category as it was
  before its first change in the session. Undo goes back to that snapshot,
  not one step back.
- `u` on a row that wasn't changed in this session does nothing. Notes aren't
  covered by undo, because the editor already shows the old text.

### State and concurrency

- The model owns the transactions. A change runs as a `tea.Cmd` that calls
  the API and returns a result message. The row changes only once the API
  confirms it, using the object the `PUT` returns.
- While a save is in progress for a row, further changes to that row are
  ignored, so saves can't arrive out of order.
- A failed save leaves the row as it was and puts the error in the status
  line.
- The initial load, and `R`, fetch transactions, categories and accounts in
  parallel behind a spinner. `R` clears the green reviewed rows.

## Testing

- `internal/lunchmoney`: `httptest.Server` tests covering paging, the bodies
  of `PUT` requests, error parsing, and 429 retry.
- `cmd/browse`: drive the model with key messages against a fake client
  interface. Check selection movement, what review, categorise, notes and
  undo send, green rows, and moving to the next row.
- Manual runs against `mock.lunchmoney.dev/v2` with any token of 11 or more
  characters. Use a demo budget before pointing `lmao` at the real one, since
  changes are permanent.

## Milestones

1. **Skeleton:** `go mod init github.com/adnathanail/lmao`, Cobra root, token
   from the environment, API client with transaction, category and account
   listing, and tests.
2. **Read-only TUI:** two-pane layout, list, detail panel, reload, resizing.
3. **Review and undo:** `r` and `u`, green rows, moving to the next row,
   errors in the status line.
4. **Category picker:** `c`, which also marks reviewed, with undo restoring
   the category.
5. **Notes:** `n`.
6. **Docs:** README and CLAUDE.md.
7. **Packaging, later:** `flake.nix` (package, Home Manager module that sets
   `LUNCHMONEY_TOKEN` from a secret file), `.golangci.yml`, CI workflow,
   release workflow, version stamping, and Dependabot with the
   vendor-hash workflow, all modelled on clonager.
8. **OAuth, when Lunch Money ships it:** `OAuthSource`, `lmao login`, and a
   Keychain-stored token.

## Open questions

- Should `r` on a row that's already reviewed do nothing, or undo it like `u`?
  The plan: it does nothing.
- Should there be a setting to sort the list by amount or payee instead of
  date? Not planned for the first version.
