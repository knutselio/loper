# loper

[![test](https://github.com/knutselio/loper/actions/workflows/test.yml/badge.svg)](https://github.com/knutselio/loper/actions/workflows/test.yml)

A keyboard-driven terminal picker that lists the git repos on your machine and opens the one you select with a command you define, such as your editor or a new terminal tab.

It is a launcher and nothing more: it never runs a git operation and makes no network requests.

## A look at it

```text
 loper · 5 repos                                              sort: last opened
 ──────────────────────────────────────────────────────────────────────────────
   NAME        FOLDER       BRANCH              OPENED
 > billing     ~/code/work  fix/rounding-error  2h ago
   tidy-yaml   ~/code/oss   feature/anchors     yesterday
   dotfiles    ~            main                3d ago
   ray-tracer  ~/code/play  main                -
   storefront  ~/code/work  main                -
 ──────────────────────────────────────────────────────────────────────────────
 enter open | / search | s sort | r refresh | ? help | q quit | z Zed | a add
```

## Installation

loper needs Go 1.27.3 or later and is built for macOS.

```sh
go install github.com/knutselio/loper@latest
```

Or from a clone:

```sh
git clone https://github.com/knutselio/loper.git
cd loper
go install .
```

Either way the binary goes to `$GOBIN`, or `$(go env GOPATH)/bin`, which must be on your `PATH`.

## Getting started

Run `loper`. Without a config file it lists every git repo under `~/code`.

There is no target to open a repo in until you add one. Press `a` and fill in a name, a key and a command, such as `Zed`, `z` and `zed {path}`, with `enter` after each. loper shows what it will add to the config file, and one more `enter` saves it. Then select a repo and press `z`, or press `enter` and pick the target from the menu.

The target is written to `~/.config/loper/config.toml` (or `$XDG_CONFIG_HOME/loper/config.toml`), which you can also edit by hand. A small useful config:

```toml
paths = ["~/code", "~/dotfiles"]

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"
```

`loper --help` prints the config file in use and the keys. `loper --config <file>` uses another file for one run.

## Features

- Fuzzy search over name, folder and branch, with the matched letters highlighted.
- Sort by last opened or by name.
- Open the selected repo in a target with one key, or choose it from a menu.
- Add a target from inside the picker, without opening the config file.
- Instant start from a cache, with a refresh in the background.
- A branch column and a column for when you last opened each repo.
- Choose the columns and how the name and folder are drawn.
- A layout that drops columns and shortens values in a narrow terminal.
- Your terminal's own palette colours, and no colour when `NO_COLOR` is set.

## Keys

| Key                  | Action                                      |
| -------------------- | ------------------------------------------- |
| `j` / `k`, `↓` / `↑` | Move the selection                          |
| `g` / `G`            | Jump to the top / bottom                    |
| `ctrl-d` / `ctrl-u`  | Move half a page down / up                  |
| `enter`              | Open the target menu for the selected repo  |
| a target's key       | Open the selected repo in that target       |
| `/`                  | Start the search                            |
| `s`                  | Switch the sort                             |
| `r`                  | Force a full refresh                        |
| `a`                  | Add a target to the config file             |
| `?`                  | Show the help overlay                       |
| `q`, `ctrl-c`        | Quit                                        |
| `esc`                | Close a menu or dialog, or clear the search |

The keys inside the target menu and while searching are in the help overlay and in `loper --help`.

## Configuration

Every setting is optional. These are the defaults, with one target added:

```toml
paths = ["~/code"]                 # folders to scan, or repos to list directly
row_style = "name-folder"          # or "root-path", "name"
columns = ["name", "folder", "branch", "last_opened"]  # "name" is required
sort = "last_opened"               # or "name"
cache_max_age = "0"                # "0" scans on every launch; or an interval such as "4h"
close_on_open = false              # true quits after a successful open

[[targets]]                        # none by default; add as many as you like
name = "Zed"
key = "z"                          # optional, one character, not one of loper's own keys
command = "zed {path}"
```

A target's command must contain `{path}`, which becomes the repo's absolute path, and may contain `{name}`, which becomes the repo's name. The command is not run through a shell, so a path with spaces needs no quoting. A mistake in the file stops loper at startup with the line it is on.

[SPEC.md](SPEC.md) has the details: the scan rules, the cache, the row styles and how targets are run.

## Licence

MIT, see [LICENSE](LICENSE).
