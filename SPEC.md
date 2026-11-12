# loper

A keyboard-driven terminal picker for the git repos on my machine. The name reads as "lazy opener", and in Dutch a _loper_ is a master key.

## Problem Statement

I work on macOS, increasingly in the terminal, and I move between a handful of git repos. Today I use a bash script, `zedi`, to do that: it finds every repo under `~/code`, caches the list for four hours, lets me pick one with `fzf`, and opens it in Zed. It has three limits:

- **One destination.** It can only open a repo in Zed. I also want to open a repo in a new terminal panel, and later in other places I define myself.
- **No context.** The list is bare paths. I cannot see which branch a repo is on, or which repos I opened most recently.
- **One root.** It only looks under `~/code`, so repos elsewhere, such as my dotfiles, are never in the list.

Tools that list multiple repos exist (gitpane, git-scope, dirtygit), but they are dashboards or git workspaces: panels, polling, status views and git actions, built to be lived in. I tried gitpane's idea and it is not what I want. I want a fast picker with a few small extras that gets out of the way.

## Solution

`loper` is a terminal UI in the style of lazygit: one full-width list of my repos, driven entirely from the keyboard.

- **It starts instantly.** The list comes from a cache and is corrected in the background, so it is always current without ever making me wait.
- **Each row tells me enough to choose.** A row shows the repo's name, where it lives, the branch it is on, and when I last opened it with `loper`. The repos I opened most recently are at the top.
- **I find a repo by typing.** `/` starts a fuzzy search over name, folder and branch.
- **I choose where it opens.** Each "open target" is a command I define in a config file, such as Zed, a new terminal tab or lazygit. `enter` shows the targets, and each target can have its own key that opens the selected repo in one keystroke.
- **It is a launcher and nothing more.** It never runs git operations and never fetches.

## User Stories

### Finding repos

1. As a developer, I want `loper` to list every git repo under `~/code` without any configuration, so that it works the first time I run it.
2. As a developer, I want to add another folder to scan with one line of config, so that repos outside `~/code` appear in the list.
3. As a developer, I want to add a single repo such as my dotfiles the same way I add a scan folder, so that I have only one setting to learn.
4. As a developer, I want the scan to stop descending once it finds a repo, so that nested checkouts in dependency folders never clutter the list and the scan stays fast.
5. As a developer, I want hidden folders and symlinks skipped during the scan, so that caches and tool-managed clones do not appear.
6. As a developer, I want a hidden folder that I list explicitly to be included, so that I can still add a repo that lives in a dot-folder.
7. As a developer, I want a warning when a configured path does not exist, so that I notice a typo while the rest of the list still loads.
8. As a developer, I want only primary repos listed, not linked worktrees or submodules, so that the first version's list is simple and predictable.

### Speed and freshness

9. As a developer, I want the list to appear the moment I launch the tool, so that picking a repo never starts with a wait.
10. As a developer, I want the list refreshed in the background on every launch, so that a repo I cloned a minute ago is there without my doing anything.
11. As a developer, I want a spinner while a refresh runs, so that I know the list may still change.
12. As a developer, I want the list to update in place when the refresh finishes, so that I never have to restart to see new repos.
13. As a developer, I want a key that forces a refresh, so that I can pick up a repo I cloned while the tool was open.
14. As a developer, I want to be able to set a maximum cache age, so that I can skip the folder scan on most launches if my roots ever grow large.
15. As a developer, I want each repo's branch re-read on every launch regardless of cache age, so that the branch column is never wrong.
16. As a developer, I want repos whose folder has been deleted to drop out of the list on launch, so that I can never pick a dead entry.
17. As a developer, I want a clear "scanning" state on the very first launch, so that an empty list is not mistaken for "no repos".
18. As a developer, I want a message naming the scanned paths and the config file when no repos are found, so that I know what to fix.
19. As a developer, I want the selection to move to the nearest row when the selected repo disappears in a refresh, so that I never lose my place.

### Reading the list

20. As a developer, I want each row to show the repo's name in its own column, so that I can scan names down a straight edge.
21. As a developer, I want the repo's folder shown dimmed beside the name, so that I can tell two clones of the same repo apart.
22. As a developer, I want each row to show the checked-out branch, so that I know what state I left the repo in.
23. As a developer, I want each row to show when I last opened the repo with `loper`, so that the default order explains itself.
24. As a developer, I want a `-` in that column for repos I have never opened with `loper`, so that the column stays quiet.
25. As a developer, I want to choose which columns are shown and in what order, so that the list carries only what I use.
26. As a developer, I want to switch the row style to "path from its root" or "name only", so that I can trade grouping against compactness.
27. As a developer, I want a repo with a detached HEAD to show its short commit hash, so that the branch column is never blank.
28. As a developer, I want a repo with no commits yet to show the branch it will start on, so that a fresh repo looks normal in the list.
29. As a developer, I want the tool to honour `NO_COLOR`, so that it behaves like my other terminal tools when I turn colour off.
30. As a developer, I want columns to drop in a fixed order and long values to be cut with an ellipsis in a narrow terminal, so that the list stays readable in a small split.
31. As a developer, I want the tool to use my terminal's own colours, so that it matches whatever theme I run.

### Sorting and searching

32. As a developer, I want the list sorted by most recently opened by default, so that the repo I want is usually at the top.
33. As a developer, I want repos I have never opened to follow in alphabetical order, so that the order is sensible on first use.
34. As a developer, I want to switch to alphabetical order with one key, so that I can browse by name when I prefer.
35. As a developer, I want to set the starting sort in my config, so that the tool opens the way I like it.
36. As a developer, I want a sort switched with the key to last only for that session, so that my configured order is back on the next launch.
37. As a developer, I want to press `/` and type to fuzzy-search, so that I reach a repo in a few keystrokes as I do today with `fzf`.
38. As a developer, I want the search to match the name, the folder and the branch, so that I can find a repo by what I was working on.
39. As a developer, I want the best match selected automatically, so that `/`, a few letters and `enter` acts on it without moving.
40. As a developer, I want matched letters highlighted, so that I can see why a row matched.
41. As a developer, I want `esc` to clear the search and restore my previous selection, so that a mistaken search costs nothing.
42. As a developer, I want "No matches" shown when a search finds nothing, so that an empty list is explained.

### Opening a repo

43. As a developer, I want `enter` to show a menu of open targets for the selected repo, so that I can see where it can go without memorising anything.
44. As a developer, I want each target to have its own key, so that I can open the selected repo in Zed or a terminal with a single keystroke.
45. As a developer, I want target keys to work both in the list and inside the menu, so that the fast path and the discoverable path are the same keys.
46. As a developer, I want the tool to ship with no target and to tell me how to add one when I press `enter` without any, so that it is not tied to one editor.
47. As a developer, I want to add a target by writing a command with a `{path}` placeholder, so that any program that accepts a folder can become a target.
48. As a developer, I want a `{name}` placeholder as well, so that a new terminal tab can be labelled with the repo's name.
49. As a developer, I want folder names with spaces to work without any quoting on my part, so that a target never breaks on an unusual path.
50. As a developer, I want the tool to stay open after opening a repo, so that I can keep it running in a terminal block as my launcher.
51. As a developer, I want a setting that makes it quit after a successful open, so that I can also use it as a one-shot picker.
52. As a developer, I want a short confirmation after a successful open, so that I know the command ran.
53. As a developer, I want a failed open shown in a dialog with the command and its output, so that I can see what went wrong and try another target.
54. As a developer, I want the tool never to quit on a failed open, so that an error is never lost when the window closes.
55. As a developer, I want a target command that keeps running to be left alone and counted as opened, so that the tool never freezes and never kills a program I started.
56. As a developer, I want targets that depend on beta software to live in my own config, not in the tool, so that I can fix them with a one-line edit when that software changes.

### Keyboard and layout

57. As a developer, I want vim-style and arrow-key navigation, so that the tool feels like lazygit.
58. As a developer, I want keys to jump to the top and bottom and to move half a page, so that a long list is quick to cross.
59. As a developer, I want the footer to list my targets and the main keys, so that I never have to look anything up.
60. As a developer, I want a help overlay listing every key, so that the less common ones are one keystroke away.
61. As a developer, I want a single scrolling list with no side panels, so that nothing competes with picking a repo.

### Configuration

62. As a developer, I want one TOML config file in the standard config location, so that I can keep it with my other dotfiles.
63. As a developer, I want every setting to be optional, so that I only write down what differs from the defaults.
64. As a developer, I want a config mistake reported at startup with the line it is on, so that a typo is never silently ignored.
65. As a developer, I want a target key that clashes with one of the tool's own keys reported as an error, so that a key never does two things.
66. As a developer, I want to run the tool against another config file with a flag, so that I can try changes without touching my real config.
67. As a developer, I want a help flag that prints the usage, the config file location, the keys and my configured targets, so that I can check them without starting the picker.
68. As a developer, I want to add a target from inside the tool by typing its name, key and command, so that a new target does not mean opening the config file.
69. As a developer, I want a target I add that way checked by the same rules as the config file, with the reason shown when it is refused, so that the tool never writes a config it cannot load.
70. As a developer, I want to see the exact lines that will be added and the file they go to before anything is written, and the rest of my config file left as it was, comments included, so that my hand-written file stays mine.
71. As a developer, I want a target I add to work at once, so that I do not have to restart the tool to use it.
72. As a developer, I want to edit or remove a target from the target menu, so that changing a target does not mean opening the config file either.
73. As a developer, I want to see the lines an edit or a removal will change before anything is written, and every other line of my config file left as it was, so that a change to one target never costs me the rest of the file.

### Building and trusting it

74. As a developer, I want to install the tool by building it from my clone, so that the first version needs no release process.
75. As a developer, I want a version flag, so that a later update command has something to compare against.
76. As a maintainer, I want tests that read like the features in this document, so that I can change the internals without rewriting the tests.
77. As a maintainer, I want the tests never to touch my real config, cache or history, so that running them is always safe.

## Implementation Decisions

### Principles

- `loper` is a launcher. It reads repo state and runs the commands I configure. It never runs a git operation, never fetches, and makes no network requests.
- It is written in Go with Bubble Tea 2. It lives in its own repository under `~/code/personal`, with a GitHub remote.
- macOS is the only supported and tested platform. Nothing is deliberately Mac-specific.

### Modules

Each module hides its details behind a small interface. The UI is the only module that knows about the terminal.

- **Config.** Loads and validates the config file and applies defaults. It returns either a complete settings value or an error that names the file and line. All validation happens here, so no other module checks config values. It also adds, changes and removes a target in the file for the dialog that does so, checked by the same rules.
- **Scanner.** Given the configured paths, returns the list of repo folders found. It owns the scan rules.
- **Repo info.** Given a repo folder, returns its current branch, or the short commit hash for a detached HEAD, and whether the repo still exists. It reads the repo's HEAD reference directly, so the tool does not need the `git` program at run time.
- **Cache store.** Reads and writes the cached repo list with the data for every column, along with when it was scanned and from which configured paths.
- **History store.** Reads and writes the time each repo was last opened, keyed by the repo's absolute path.
- **Target runner.** Given a target and a repo, builds the command, runs it and returns success, or failure with the command line and its output.
- **Matcher.** Given a query and the rows, returns the matching rows in rank order with the matched positions. It uses an existing fuzzy-matching library.
- **Relative time.** Formats a past time as "2h ago", "yesterday" and so on, given the current time.
- **UI.** The Bubble Tea program: state, update and view. It receives its dependencies (settings, stores, scanner, target runner, clock) from the entry point, which is what makes it testable.
- **Entry point.** Parses the command line, loads the config, wires the modules together and starts the UI.

### Discovery

- The config has one `paths` list. An entry that is itself a repo becomes one row. Any other entry is scanned for repos beneath it.
- A repo is a folder that contains a `.git` directory. A folder with a `.git` file (a linked worktree or a submodule) is not a repo for this version.
- The scan goes to any depth and stops descending when it finds a repo.
- The scan skips folders whose name starts with a dot and does not follow symlinks. An entry listed directly in `paths` is exempt from both rules: it may be hidden and it may be a symlink, which is followed. Its rows show the path as written in the config, not the resolved one.
- A listed path that does not exist is skipped and reported as a warning in the header.
- There is no exclude list and no depth limit.

### Cache and refresh

- **Launch.** The UI shows the cached list immediately, then starts a background refresh. The header shows a spinner until it finishes, and the list updates in place.
- **Full refresh.** Runs the scanner, re-reads every repo's info and rewrites the cache.
- **Light refresh.** Re-reads the info of the cached repos and drops those that no longer exist. It does not scan.
- **Which one runs at launch.** With the default `cache_max_age` of `"0"`, every launch does a full refresh. With an interval such as `"4h"`, launch does a full refresh only when the cache is older than that, and a light refresh otherwise.
- **Changed paths.** A cache built from a different `paths` list than the current one is treated as expired.
- **Forced refresh.** `r` always does a full refresh. Pressing it while a refresh is running does nothing.
- **Selection.** If the selected repo disappears in a refresh, the selection moves to the nearest row.
- **Storage.** The cache is a JSON file in `~/.cache/loper/`. The history is a separate JSON file in `~/.local/state/loper/`, so clearing caches never loses the sort order. Both locations honour the `XDG_*` environment variables.
- **Concurrent instances.** Each instance writes its files whole and the last write wins. There is no locking.
- **History entries** for repos that are no longer found are kept, so a repo that comes back keeps its last-opened time.

### Rows and columns

- There are four columns: `name`, `folder`, `branch` and `last_opened`. The `columns` setting selects and orders them. The default is all four. `name` is required; the other three are optional.
- The cache always holds the data for every column, so changing `columns` never needs a rescan.
- `row_style` controls how name and folder are drawn:
  - `name-folder` (default): the name in one column, and the parent folder in a dimmed second column, shortened with `~`.
  - `root-path`: the `name` column shows the path from the repo's scan root, with everything before the name dimmed. A repo listed directly shows its bare name. The `folder` column is not shown, even when `columns` lists it.
  - `name`: the name only. The `folder` column is not shown, even when `columns` lists it.
- `last_opened` shows a relative time, or `-` for a repo never opened with `loper`.
- `branch` shows the branch name, the short commit hash prefixed with `@` for a detached HEAD, or the pending branch name for a repo with no commits.
- In a narrow terminal, columns are dropped in a fixed order, whatever order `columns` puts them in: last opened first, then branch, then folder. Before a column is dropped, the folder and branch columns shrink to 12 cells, with long values cut by `…`. Values that are still too long are cut with `…`; under `root-path` the dimmed path is cut at its start, so the repo's name stays whole.
- Colours come from the terminal's own palette. The tool honours `NO_COLOR`.

### Sorting

- `last_opened` (default): most recently opened first. Repos never opened follow, ordered by name and then folder.
- `name`: by name, then folder, ignoring case. Under `row_style = "root-path"` the alphabetical order is on the displayed path instead, so repos group by folder. The same rule orders never-opened repos in the `last_opened` sort.
- `s` switches between the two for the current session. The `sort` setting decides the starting order.

### Search

- `/` starts the search. The header becomes the search line.
- The query is matched fuzzily against name, folder and branch, also when their column is hidden. Under `root-path` the name is the path the row shows. A space separates terms, and a row matches when every term matches one of the three. Matching is case-insensitive unless the query contains a capital letter.
- Only matching rows are shown, best match first. A match in the name ranks above a match in the folder or branch. Ties keep the current sort order.
- The top match is selected automatically. Matched letters are highlighted.
- While searching, letters go to the query, so target keys are inactive. `↑`/`↓` and `ctrl-p`/`ctrl-n` move through the matches, and `enter` opens the target menu for the selected match.
- `esc` clears the search and restores the full list and the previous selection.

### Layout

A single full-width scrolling list between a one-line header and a one-line footer. There are no side panels and no paging.

```
 loper · 7 repos                             sort: last opened   ⠋ refreshing
 ──────────────────────────────────────────────────────────────────────────────
   NAME         FOLDER                   BRANCH                 OPENED
 > backend      ~/code/work/unlimited    fix/bug-1116           2h ago
   aycapi       ~/code/work/mono         main                   yesterday
   exa-men-v2   ~/code/work/mono         feature/aycid-login    3d ago
   dotfiles     ~                        main                   -
 ──────────────────────────────────────────────────────────────────────────────
 enter open | / search | s sort | r refresh | ? help | q quit | z Zed | …
```

- **Header:** the title, such as `loper · 7 repos` with `loper` in bold, or `loper · 1 repo` for one; then the current sort, refresh spinner and any path warnings.
- **Headings:** a dimmed line under the header's rule names the columns shown, `NAME`, `FOLDER`, `BRANCH` and `OPENED`, each aligned with its column. A column is at least as wide as its heading. The headings stay during a search and are not shown when the list has no rows or room for only one line.
- **Selection:** the selected row is marked with `>`. With colour on, it is also drawn in reverse video across the full width of the screen, gaps and padding included, and the dimmed and highlighted parts inside it never end it. The highlighted target in the menu is drawn the same way across the menu's width.
- **Accent:** with colour on, the keys in the footer, the help overlay, the target menu and its hint, and the add dialog are bold in one palette colour, cyan. Inside the reverse-video bar the accent is not used.
- **Footer:** the tool's own keys, then the targets with their keys, then `a add` last, separated by ` | `. `a add` comes after the targets so that it is the first item to go when the line is short. With colour on, each key is in the accent and the separators are dimmed. When the line does not fit, whole items are dropped from the end and it ends with `…`. After a successful open it shows a confirmation such as "Opened aycapi in Zed", and after a target is added, edited or removed one such as "Added Lazygit", "Changed Lazygit" or "Removed Lazygit". Every such notice shows in place of the keys for 3 seconds, or until the next key press, and a new notice gets its own 3 seconds. The 3 seconds are not a config setting.
- **Overlays:** the target menu, or with no targets the message in its place, the error dialog, the add dialog, which also edits and removes a target, or the message that the config file cannot take the change, and the help screen appear on top of the list. Nothing else does. The target menu has the hint `e edit  d remove` in its bottom border, as its title is in the top one, and the edit and remove dialogs open over it. When the screen is too small to show the whole add dialog, the line "Window too small for the dialog", cut to the width, shows in its place, on the middle line of the list or, with no room for a list, on the header; what was typed is kept, and the dialog shows again once the window is large enough.
- **Empty states:**
  - First launch with no cache: the spinner and "scanning…".
  - No repos found: a message naming the scanned paths and the config file location.
  - Search with no matches: "No matches".
  - No targets: the footer lists none, so `a add` follows `q quit`, and `enter` shows "No targets yet. Press a to add one." in a box in the menu's place instead of an empty menu, and the help overlay's "Targets" heading has "None yet. Press a to add one." under it.

### Keybindings

These keys are fixed in this version. Only target keys are configurable.

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
| `e` / `d`            | Edit / remove a target, in the target menu  |
| `?`                  | Show the help overlay                       |
| `q`, `ctrl-c`        | Quit                                        |
| `esc`                | Close a menu or dialog, or clear the search |

Inside the target menu, `j`/`k` and the arrow keys move, `enter` runs the highlighted target, a target's key runs that target, `e` edits the highlighted target, `d` removes it, and `esc` closes the menu; `q` and `ctrl-c` quit from inside the menu, the error dialog and the help overlay, and `esc` or `?` closes the help overlay. With no targets, in the message `enter` shows instead of the menu, `a` opens the add dialog, `esc` closes the message, `q` and `ctrl-c` quit, and every other key does nothing.

Inside the add dialog every printable key is text, `q`, the list keys and the target keys included, and only `ctrl-c` quits. `tab` / `shift-tab` and `↓` / `↑` move between the fields, `enter` moves to the next field and from the last one to the confirmation, `backspace` removes a character, a paste goes into the current field without its line breaks and control characters, and `esc` closes the dialog without writing anything. In the confirmation, `enter` writes the target and `esc` goes back to the fields with their contents kept. An `enter` within 300 milliseconds of the confirmation appearing, or while the screen is too small to show it, is ignored, so a quick double `enter` on the last field never writes a block that was not seen; `esc` is never held back. The 300 milliseconds are a parameter of the UI, not a config setting, so tests can control them. In the message that the config file cannot take a target, `esc` closes the message, `q` and `ctrl-c` quit, and every other key does nothing. `a` does nothing in a target menu that lists targets, the error dialog and the help overlay, and during a search it is text. `e` and `d` act only in a target menu that lists targets: on the list, in the no-targets message, the error dialog and the help overlay they do nothing, and during a search and in the fields they are text.

`e` opens the add dialog, titled "Edit a target", with its fields filled in from the highlighted target, and it takes the same keys. `d` shows the lines that will be removed, in a dialog titled "Remove a target", where `enter` removes them and `esc` keeps them; the 300 milliseconds apply to that `enter` as well. `esc` in the fields of an edit, or in a removal, goes back to the menu, which after an edit or a removal shows the targets as they now are, with the edited target or the one in the removed one's place highlighted. Removing the last target closes the menu.

### Open targets

- The command of a target is entirely up to the user. The tool knows nothing about Zed, Rex or any other program: it substitutes the placeholders and runs what the user wrote.
- A target has a `name`, a `command` and an optional single-character `key`. The name is required, a name of only spaces counts as empty, and it may not contain control characters such as a line break or an escape, which would break the rows or reach the terminal. A target without a key is reachable only through the menu.
- The command must contain `{path}`, which is replaced by the repo's absolute path. `{name}` is replaced by the repo's name. There are no other placeholders.
- The command is not run through a shell. The tool splits it into words using shell-style quoting (single quotes, double quotes and backslash), then substitutes each placeholder as part of a single argument, so a path with spaces needs no quoting. An unbalanced quote is a config error. A user who needs shell features writes `sh -c '…'` as the command.
- The command runs in the background with the repo as its working folder and the tool's own environment. The tool waits for it to exit, for at most 2 seconds. The wait is a parameter of the target runner, not a config setting, so tests can shorten it.
- **Success** is exit code 0 within the wait, or a command that is still running when the wait ends. A command that is still running is left running, detached from the tool, and is never stopped. The footer confirms it, the repo's last-opened time is recorded, and the tool stays open. With `close_on_open = true` the tool quits instead, unless the add dialog is open.
- **Failure** is a non-zero exit code within the wait, or a command that cannot be started. The error dialog shows the command line that ran and its output. Nothing is recorded and the tool never quits, whatever `close_on_open` says.
- No target is built in. Targets come only from the config file, written by hand or with the add dialog; with none, the tool works as a list and a search, and the no-targets message under "Empty states" says how to add one. Zed appears only as an example, in this document and in the add dialog.
- **Adding a target from the tool.** `a` opens a dialog and at once checks, in the background, that the config file can take a target, as the write would. If it cannot, because it lists its targets inline, is a link to a file that does not exist or no longer loads, a message with the reason replaces the fields, along with anything typed in them meanwhile, and `esc` closes it. The dialog has three fields: the name, the key and the command. Each empty field shows, dimmed, the example of a Zed target: `Zed`, `z` and `zed {path}`. The example is a hint and never a value: a field left empty is empty, and nothing of the example is written. With colour off the empty fields show nothing, since undimmed the example would read as typed text. When it is filled in, the target is checked by the config's own rules: a name is required, and the spaces around it are left out, the command must contain `{path}` and have balanced quotes, and the key is optional, one character, not one of the tool's own keys and not the key of another target in the file. A refused target shows the reason inside the dialog, which stays open. Otherwise the dialog shows the `[[targets]]` block it will append and the file it goes to, and `enter` writes it. The new target works at once, without a restart: it is in the footer, the target menu and the help, and its key opens the selected repo. The write follows the rules under "Config".
- **Editing and removing a target from the tool.** `e` in the target menu opens the add dialog with the highlighted target filled in, and `d` asks to remove it. Both check at once, in the background, that the file can be changed, as the write would: as well as the add dialog's reasons, the target may no longer be in the file, or two of its blocks may load to the same target. An edit is checked by the same rules as a target added, except that the target's own key is not a duplicate. The confirmation shows the target's lines as they are in the file and the lines that replace them, and says that comments inside the block are not kept; a removal shows the lines that go. Lines longer than the block the tool writes, four lines, are shown as their first two lines, a line such as `… 11 more lines` and their last line, so the confirmation fits whenever one for a plain block would. An edit that changes nothing, with the name, key and command as they were, closes the dialog at `enter` on the last field without a confirmation, a write or a notice, and the file stays as it was. The change works at once, as an added target does. The write follows the rules under "Config".
- A target that takes over the terminal itself, such as a full-screen program run in place, is not supported. Such programs are opened through a target that starts them elsewhere, as in the lazygit example below.

### Config

- The file is TOML at `~/.config/loper/config.toml`, honouring `XDG_CONFIG_HOME`. `--config <file>` overrides the location for one run.
- Every setting is optional. With no file, the tool runs on the defaults.
- The tool changes the config file in three ways only: the add dialog appends one `[[targets]]` block to it, an edit replaces the lines of one `[[targets]]` block, and a removal removes them.
  - The file is the one in use: the `--config` file when given, otherwise the standard location. A missing file or folder is created, the folder private to the user.
  - The file is read again when the target is written, and the block is appended to its text, so the rest of the file, comments and order included, is untouched. The name, key and command are encoded by the TOML library.
  - The text with the block appended is loaded with the config's own loader before it is written. If it does not load, for example because the file writes its targets as an inline `targets = [...]` list or was edited into an invalid state meanwhile, nothing is written and the dialog says why. The file is checked this way when `a`, `e` or `d` is pressed, again when the fields are filled in and once more when the target is written, since it can change in between. A reason about the new block itself, such as a key already used, is given without a line number, since its line is not yet in the file.
  - For an edit or a removal the target is found in the file read again by its name, key and command as they were loaded, never by its position. If it is no longer there, or more than one block matches, or its lines are no longer those the confirmation showed, nothing is written and the dialog says why. Its lines run from its `[[targets]]` line to the line of its last value, as the TOML parser reads them, so a multi-line string that holds a `[[targets]]` line is never taken for a block. Only those lines change; comments directly above the block and below its last value stay. A new block takes the line ending of the old one, and no line break at its end when the old one ended the file without one. A removed block takes the blank lines after it along or, when only blank lines follow it, those before it, so no run of blank lines is left behind.
  - Before an edit or a removal is written, the new text is read back, and the write is refused unless every setting and every other target is as it was, in the same order, and the changed target is exactly the edited one, or gone.
  - The file is written whole, through a temporary file renamed over it, so a failed write leaves it as it was and the dialog shows the error. The file keeps its mode; a new file is private to the user. A file its owner cannot write is refused for an add, an edit and a removal alike, with the reason, and nothing is written, since the rename would replace it all the same. A config file that is a symlink stays one: the file it points to is rewritten. A link to a file that does not exist is refused, and nothing is written.
- These are errors that stop the tool at startup with a message naming the file and line:
  - an unknown setting or an invalid value;
  - a target without a name, or with a name that is only spaces or contains a control character;
  - a target without `{path}`;
  - a target command with an unbalanced quote;
  - a `columns` list without `name`, or with a column listed twice;
  - a target key longer than one character;
  - two targets with the same key;
  - a target key that is one of the tool's own keys.

The defaults, written out:

```toml
paths = ["~/code"]
row_style = "name-folder"          # or "root-path", "name"
columns = ["name", "folder", "branch", "last_opened"]
sort = "last_opened"               # or "name"
cache_max_age = "0"                # "0" = full refresh on every launch; or e.g. "4h"
close_on_open = false
# no targets: they come only from the config file or the add dialog
```

My personal config, as the worked example:

```toml
paths = ["~/code", "~/dotfiles"]

[[targets]]
name = "Zed"
key = "z"
command = "zed {path}"

[[targets]]
name = "Terminal tab"
key = "t"
command = "rex window new -c {path}"

[[targets]]
name = "Terminal split"
key = "x"
command = "rex split -c {path}"

[[targets]]
name = "Lazygit"
key = "l"
command = "rex split -c {path} -- lazygit"
```

### Command line

| Command                 | What it does                                                                     |
| ----------------------- | -------------------------------------------------------------------------------- |
| `loper`                 | Starts the picker                                                                |
| `loper -v`, `--version` | Prints the version                                                               |
| `loper --help`          | Prints usage, the config file location, the key list and the configured targets  |
| `loper --config <file>` | Uses another config file for this run                                            |

There are no subcommands in this version.

`--help` ends with a "Targets" section that lists each target in the config file in use, honouring `--config` and `XDG_CONFIG_HOME`: its key, blank when it has none, and its name, in the order they are written. With no file or no targets it says `none; add one in the picker with a, or in the config file`. A config file that does not load, including a `--config` file that does not exist, does not stop `--help`: the loader's message takes the place of the list, everything else is printed and the exit code is 0.

### Install

The tool is installed by building it from the clone with `go install`. The resulting binary must be on the `PATH`.

## Testing Decisions

### What makes a good test here

A good test describes something from the User Stories in terms of what I do and what I see: keys go in, and the test checks the screen text and the side effects. It does not check internal state, message types or function calls, so it survives any reorganisation of the code.

### The main seam

Almost all tests go through one seam: the UI program with real modules behind it.

- **Driver.** A small test harness builds the real program state, feeds it one key at a time, runs any resulting background work on the spot, feeds the result back, and exposes the rendered screen as text. It does not use Bubble Tea's experimental `teatest` helper, which works on a raw byte stream and waits by polling.
- **Nothing is mocked.** Each test creates a temporary folder tree with real repos made by `git init`, writes a real config file, and uses a fake target: a small script that records its arguments, or fails on purpose.
- **Fixed environment.** Each test sets the screen size, turns colour off, and points the home, config, cache and state locations at temporary folders. My real files are never read or written.
- **Controlled clock.** The test supplies the current time, so relative times are stable.
- **Background refresh.** The test triggers it explicitly and can check both the cached list and the refreshed list.
- **Assertions.** Targeted checks: "the selected row is `aycapi`", "the screen contains the error text", "the fake target was called with this path", "this repo now sorts first". There is at most one full-screen snapshot, to guard the layout.

The temporary trees cover the awkward cases: a repo nested inside another, a folder name with spaces, a hidden folder, a symlink, a symlink listed in `paths`, a linked worktree, a detached HEAD, a repo deleted after it was cached, and a configured path that does not exist.

### What is tested at this seam

Discovery rules, cache and refresh behaviour, row rendering in each style, column selection, narrow-terminal behaviour, both sorts, search, the target menu, direct target keys, success and failure of a target, `close_on_open`, a target that keeps running past the wait (shortened for the test), adding, editing and removing a target and what that writes to the config file, and empty states.

### Other tests

- **End to end.** One or two tests run the compiled binary inside a pseudo-terminal, with `--config` and with the home, cache and state locations pointed at temporary folders. They send keys to pick a repo and check that the fake target ran and the history was written. They prove the entry point wires the modules together, and they assert on side effects, not on screen output.
- **Startup.** Config errors stop the tool before the UI exists, so they are tested through the entry point: run it with a bad config file and check the exit code and the message. The `--version` and `--help` flags are tested the same way.
- **Direct unit tests.** Only the matcher's ranking and the relative-time formatting are tested directly, because their many edge cases are tedious to reach through keystrokes.

Zed and Rex are never run by the tests. The test suite needs the `git` program to build its fixtures.

## Out of Scope

The first version deliberately leaves these out:

- Git operations of any kind, fetching, and anything dashboard-like.
- Opening repos or pull requests on GitHub.
- Linked worktrees and submodules as rows.
- Status columns: an uncommitted-changes marker, ahead/behind counts and the number of branches.
- A release pipeline and an `update` command.
- A Homebrew tap.
- An exclude list and a scan depth limit.
- Remapping the tool's own keys.
- Themes and colour settings.
- A fully configurable row layout beyond `row_style` and `columns`.
- Non-interactive subcommands, such as printing the repo list for scripts.
- Targets that take over the terminal in place.
- Mouse support.
- Locking between concurrent instances.
- Platforms other than macOS.
- Any change to my dotfiles repo, including removing `zedi`.

## Further Notes

### Planned follow-ups

- **Status columns.** The uncommitted-changes marker, ahead/behind counts and branch count are the intended next columns. Measured on my 6 repos, branch count costs about 5ms per repo, while the uncommitted-changes check took 485ms for all six on a cold run, so those columns must load in the background after the list appears. Ahead/behind would reflect the last fetch, since the tool never fetches. The cache already stores data per column, so they slot in as new column names.
- **Releases and updates.** A GitHub Actions pipeline running GoReleaser on each version tag, publishing macOS binaries for Apple Silicon and Intel. An `update` subcommand compares the running version with the latest GitHub release, downloads the matching binary, verifies its checksum and replaces the installed one; `update --check` only reports. The tool never checks for updates on its own at launch.
- **GitHub targets.** Opening a repo's GitHub page or one of its pull requests, which is where a more dashboard-like view might later grow from.
- **Worktree rows**, an **exclude list**, **key remapping**, **themes** and a **Homebrew tap**, in no particular order.

### Background

- **Why no blocking scan.** A full scan of `~/code` (about 145,000 files and folders, 6 repos) took 0.13 seconds with `fd`, before the stop-at-repo rule. The design assumes up to roughly 100 repos.
- **How the existing tools differ.** gitpane is a three-panel live dashboard and git workspace whose open action assumes tmux. git-scope is a paged, read-only status table with one editor command. dirtygit is a five-pane cleanup tool that hides clean repos. None of them is a picker first.
- **Testing precedent.** lazygit tests at the same seam, with real repos, keys in and "contains" assertions on views. The comparable Bubble Tea tools drive the update step directly instead of using `teatest`.

### Details filled in while writing

These were not discussed in the interview and are open to change:

- **Target keys in my config.** The split uses `x`, because `s` is the sort key, and lazygit uses `l`.
- **Changed paths.** A cache built from a different `paths` list counts as expired.
- **History.** Entries for repos that are no longer found are kept.
- **Config rules.** A target key is one character, duplicate target keys are an error, and `columns` must include `name` and list each column once.
- **Row styles and columns.** The `root-path` and `name` styles hide the `folder` column even when `columns` lists it.
- **Alphabetical order under `root-path`.** It sorts on the displayed path, so repos group by folder.
- **Narrow terminals.** The order in which columns are dropped is fixed and ignores the order in `columns`.
- **Wait.** The 2 seconds is not a config setting.
- **Edit and remove.** Comments directly above a removed block stay, so they may come to sit above the next block. The blank lines a removal takes along are not shown in its confirmation, which shows the block's own lines. The menu's hint is in its bottom border, so it costs the menu no row.

### Open questions, deferred

- **Rex tab.** `rex window new -c <path>` creates a new Rex window, and the syntax of all three Rex commands in my config was checked against the Rex CLI help on 11 November 2026. Whether a window is what the Rex app shows as a tab has not been confirmed by running it. Rex is in beta and its CLI may change, which is why the Rex targets live in my config and not in the tool.
- **GitHub repository visibility.** The repository is private for now and becomes public at v0.1.
- **Dotfiles migration.** Removing `zedi`, keeping the `loper` config in the dotfiles repo, putting the Go binary folder on the `PATH`, and adding install steps to the reinstall guide are mine to do later.
- **The name.** `loper` is the working name. It was free as a command, as a Homebrew formula and among GitHub projects of note when checked on 11 November 2026.
