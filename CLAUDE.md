# lmao

A Go terminal UI for reviewing unreviewed Lunch Money transactions through
the v2 API. User-facing docs are in README.md and the root command's long
help (`cmd/root.go`); keep both in sync when keys or environment variables
change. `PLAN.md` has the milestones still to do (packaging, OAuth).

## Commands

```sh
go test ./...
go vet ./...
gofmt -l .                                   # should print nothing
go build -o lmao .
LMAO_TEST_MOCK=1 go test -run TestMockServer ./internal/lunchmoney/   # hits mock.lunchmoney.dev
LUNCHMONEY_API_URL=https://mock.lunchmoney.dev/v2 LUNCHMONEY_TOKEN=any-mock-token ./lmao
```

The API reference is an OpenAPI spec published to npm as
`@lunch-money/v2-api-spec` (`lunch-money-api-v2.yaml`). The docs site's
reference pages render client-side, so read the spec instead.

## Layout

- `main.go` → `cmd.Execute()`
- `cmd/root.go` — the Cobra root command; a bare `lmao` runs the view
- `cmd/browse.go` — the Bubble Tea model: state, keys, loading and saving
- `cmd/picker.go` — the category picker and the notes editor (a Bubbles
  `textarea`)
- `cmd/view.go` — rendering: layout, list, details, header and footer
- `cmd/style.go` — Lip Gloss styles, money and time formatting
- `internal/lunchmoney/` — the API client: requests, errors, rate-limit
  retry (`client.go`), transactions, categories, accounts and tags
- `internal/auth/` — `TokenSource`, and `EnvSource` reading `LUNCHMONEY_TOKEN`

## Design decisions

- **The view only changes in `Update`.** Loads and saves run as commands
  and report back with `loadedMsg`/`savedMsg`. A row changes only once the
  API confirms the change, and takes the transaction the `PUT` returns.
- **One change in flight per row** (`row.saving`): further changes to that
  row are refused until it finishes, so they can't arrive out of order.
- **Undo goes back to a snapshot, not one step**: `row.before` is the
  status and category before the first change made in this session, set
  when that change succeeds. `u` restores it in one `PUT` (clearing the
  category with `category_id: null` if it had none), then clears it. Notes
  aren't covered by undo.
- **Setting a category also marks the transaction reviewed**, in the same
  `PUT`. `r`, and `c`'s Enter, move to the next unreviewed row below.
- **Reviewed rows stay until a reload**, in green with a ✓; only
  unreviewed transactions are loaded, so a reload drops them.
- **`Change` sends only the fields being changed** (its `MarshalJSON`):
  the API returns 400 for unknown fields, and null clears a field.
- **The `PUT` response has no metadata** (the list asks for it with
  `include_metadata`), so `saved` keeps the row's existing metadata.
- **Rate limits** (100 requests a minute): a 429 is retried once after its
  `Retry-After` (capped at 2 minutes), and `Client.OnRateLimit` tells the
  view, which `runBrowse` wires to `p.Send`.
- **Tokens come from a `TokenSource` asked on every request**, so an OAuth
  source that refreshes tokens can replace `EnvSource` without changes
  elsewhere.
- **Amounts are decimal strings**, positive for money going out. They're
  formatted with `math/big`, never `float64`. The list shows money coming
  in with a `+` and in cyan, which keeps green for reviewed rows.
- **The list is drawn by hand** (as in clonager's `browse`), not with a
  Bubbles table, so rows can be coloured as a whole or by cell. The
  category column is sized from every category's name, so it doesn't change
  width as categories are set.
- Colours are basic ANSI (0–7), so they follow the terminal theme. The
  textarea's own styles use fixed colours, so `newNotesEditor` replaces
  them, and turns off cursor blinking.

## Tests

- `internal/lunchmoney`: `httptest` servers check request bodies, paging,
  error parsing and the 429 retry. `TestMockServer` runs against the real
  mock server, only with `LMAO_TEST_MOCK=1`.
- `cmd`: the model is driven with key messages against `fakeAPI`, running
  the commands it returns (`run`/`collect`). Each test waits for one
  spinner tick (100ms).
