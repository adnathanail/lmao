# lmao

A terminal UI for reviewing your unreviewed [Lunch Money](https://lunchmoney.app)
transactions.

`lmao` lists the transactions waiting for review one month at a time,
starting with the current month, with the selected one's details beside
it. The header says how many are waiting in other months. From there you
can mark transactions reviewed, set their categories and edit their notes,
without opening the web app.

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
`lmao` on demo data first, make a demo budget in Lunch Money (Set up Demo)
and a token for it. Or point it at Lunch Money's mock server, which accepts
any token of 11 or more characters:

```sh
LUNCHMONEY_API_URL=https://mock.lunchmoney.dev/v2 LUNCHMONEY_TOKEN=any-mock-token lmao
```

## Installing with Nix

Try it with:

```sh
nix run github:adnathanail/lmao           # the latest commit on main
nix run github:adnathanail/lmao/v0.1.0    # a release
```

The flake's package is `lmao`, with shell completions. With Home Manager,
its module can also read the token from a file, such as a secret agenix
decrypts, each time `lmao` starts, so the token isn't in every shell's
environment:

```nix
{
  inputs.lmao.url = "github:adnathanail/lmao";

  # In your Home Manager config:
  imports = [ inputs.lmao.homeModules.default ];
  programs.lmao = {
    enable = true;
    tokenFile = "/run/agenix/lunchmoney-token"; # optional
  };
}
```

`LUNCHMONEY_TOKEN` set in the environment still takes precedence over
`tokenFile`.

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
| `o` | Open it in the Lunch Money web app |
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

## Versions and releases

`lmao --version` shows the release (e.g. `v0.2.0`) for an install of a
release, the commit hash for an install of any other commit, and `dev` for a
build from your own checkout.

To release, run the **Release** workflow (Actions → Release → Run workflow)
on `main`, choosing `patch`, `minor` or `major`. It bumps the latest release
tag by that (from `v0.1.0`: `v0.1.1`, `v0.2.0` or `v1.0.0`), makes a release
commit on top of `main` that adds a `VERSION` file, builds it with Nix
(running the tests) and checks it reports that version, then pushes the tag
and creates a GitHub release. Only the tag is pushed: `main` never has a
`VERSION` file.

This is because Nix builds can't see git tags, so the version has to be in
the source; `go install` gets it from the tag itself. Tags made by hand still
work for `go install`, but Nix builds of them show the commit hash.
