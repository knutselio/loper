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
 enter open | / search | s sort | r refresh | ? help | q quit | o cd | a add
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

Run `loper`. It lists every git repo under `~/code`. On the first run it creates its config file with one target, `cd`: select a repo and press `o` to land in its folder. [Change folder](#change-folder) has the shell function that makes it your own shell's folder.

To open a repo somewhere else, add a target. Press `a` and fill in a name, a key and a command, such as `Zed`, `z` and `zed {path}`, with `enter` after each. The fourth field, `After open`, says whether loper closes after opening a repo there. It is a choice, drawn as `‹ default (stay open) ›`: `←` and `→` change it, and `enter` leaves it on `default`, which follows `close_on_open`. The last choice, `take over this window`, makes loper end and run the command in its place, in the repo's folder, for a program such as lazygit or an editor. loper shows what it will add to the config file, and one more `enter` saves it. Then select a repo and press `z`, or press `enter` and pick the target from the menu.

The target is written to `~/.config/loper/config.toml` (or `$XDG_CONFIG_HOME/loper/config.toml`), which you can also edit by hand. A small useful config:

```toml
paths = ["~/code", "~/dotfiles"]

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"
```

`loper --help` prints the config file in use, the keys and your targets. `loper --config <file>` uses another file for one run.

## Change folder

The first run writes this target, which you can remap, edit or remove like any other:

```toml
[[targets]]
name = "cd"
key = "o"
command = "cd {path}"
in_place = true
```

A command whose program is `cd` ends loper and lands you in the folder after it. A program cannot change the folder of the shell that started it, so on its own loper starts a new shell there, `$SHELL` or `/bin/sh`, and `exit` takes you back. With this function in your `~/.zshrc` or `~/.bashrc`, loper hands the folder to your own shell instead, which changes to it:

```sh
loper() {
  local file dir code
  file=$(mktemp) || return
  LOPER_CD_FILE=$file command loper "$@"
  code=$?
  dir=$(command cat -- "$file")
  command rm -f -- "$file"
  [ -n "$dir" ] && cd -- "$dir"
  return $code
}
```

Quitting, or any other target, leaves the shell where it was. The function keeps loper's exit code.

## Features

- Fuzzy search over name, folder and branch, with the matched letters highlighted.
- Sort by last opened or by name.
- Open the selected repo in a target with one key, or choose it from a menu.
- Targets that take over loper's own terminal window, such as lazygit or an editor, run in the repo's folder.
- A `cd` target, written on the first run, that lands you in the repo's folder.
- Add, edit and remove targets from inside the picker, without opening the config file.
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
| `enter`, `space`     | Open the target menu for the selected repo  |
| a target's key       | Open the selected repo in that target       |
| `/`                  | Start the search                            |
| `s`                  | Switch the sort                             |
| `r`                  | Force a full refresh                        |
| `a`                  | Add a target to the config file             |
| `e` / `d`            | Edit / remove a target, in the target menu  |
| `?`                  | Show the help overlay                       |
| `q`, `ctrl-c`        | Quit                                        |
| `esc`                | Close a menu or dialog, or clear the search |

The keys inside the target menu and while searching are in the help overlay and in `loper --help`.

## Configuration

Every setting is optional. These are the defaults, with two targets added:

```toml
paths = ["~/code"]                 # folders to scan, or repos to list directly
row_style = "name-folder"          # or "root-path", "name"
columns = ["name", "folder", "branch", "last_opened"]  # "name" is required
sort = "last_opened"               # or "name"
cache_max_age = "0"                # "0" scans on every launch; or an interval such as "4h"
close_on_open = false              # true quits after a successful open

[[targets]]                        # the first run writes a cd target; add as many as you like
name = "Zed"
key = "z"                          # optional, one character, not one of loper's own keys
command = "zed {path}"
close = true                       # optional; left out, close_on_open decides

[[targets]]
name = "Lazygit"
key = "l"
command = "lazygit -p {path}"
in_place = true                    # optional; loper ends and the command runs in its window
```

A target's command must contain `{path}`, which becomes the repo's absolute path, and may contain `{name}`, which becomes the repo's name. The command is not run through a shell, so a path with spaces needs no quoting. A `cd` command needs `in_place = true` and one folder. A mistake in the file stops loper at startup with the line it is on.

[SPEC.md](SPEC.md) has the details: the scan rules, the cache, the row styles and how targets are run.

## Licence

MIT, see [LICENSE](LICENSE).
