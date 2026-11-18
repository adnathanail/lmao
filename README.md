# lmao

A terminal UI for reviewing your unreviewed [Lunch Money](https://lunchmoney.app)
transactions.

`lmao` lists the transactions waiting for review in one month at a time,
starting with the current month, newest first, with the selected one's
details beside it. The header says how many are waiting in other months. From there you can mark transactions
reviewed, set their categories and edit their notes, without opening the
web app.

## Setup

1. Create an access token at <https://my.lunchmoney.app/developers>.
2. Put it in `LUNCHMONEY_TOKEN`.
3. Run `lmao`.

```sh
go install github.com/adnathanail/lmao@latest
export LUNCHMONEY_TOKEN=...
lmao
```

Changes are made to your real budget as soon as you make them. To try
`lmao` without touching it, point it at Lunch Money's mock server, which
accepts any token of 11 or more characters:

```sh
LUNCHMONEY_API_URL=https://mock.lunchmoney.dev/v2 LUNCHMONEY_TOKEN=any-mock-token lmao
```

## Building from source

You need Go 1.26 or later. From a clone of this repository:

```sh
go build -o lmao .
LUNCHMONEY_TOKEN=... ./lmao
```

`go install .` instead puts the binary in `~/go/bin` (or `$GOBIN`), so it
runs as `lmao` from anywhere if that folder is on your `PATH`.

## Keys

| Key | Does |
| --- | --- |
| `↑`/`↓`, `j`/`k` | Select a transaction (`PgUp`/`PgDn`, `g`/`G` jump) |
| `r` | Mark it reviewed, and move to the next unreviewed one. Only transactions with a category can be reviewed. |
| `c` | Set its category: type to narrow the list, `Enter` to set it. This also marks it reviewed. |
| `n` | Edit its notes: `Enter` saves, `Alt-Enter` adds a new line, `Esc` cancels |
| `u` | Undo: put its status and category back to how they were before you changed them |
| `←`/`→`, `h`/`l` | Show the previous or next month |
| `s` | Flip the order, newest first (the default) or oldest first, and select the first transaction |
| `J`/`K` | Scroll the details |
| `R` | Reload from Lunch Money |
| `q` | Quit |

Transactions you review stay in the list, in green with a ✓, until you
reload, so you can undo them with `u`. The mouse works too: click to select,
and scroll the list or the details.

Pending transactions aren't listed: Lunch Money reviews them once they post.

## Environment

| Variable | Meaning |
| --- | --- |
| `LUNCHMONEY_TOKEN` | Your access token (required) |
| `LUNCHMONEY_API_URL` | The API to use (default `https://api.lunchmoney.dev/v2`) |
