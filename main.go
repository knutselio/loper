// Command loper is a keyboard-driven terminal picker for git repos.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/config"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/repoinfo"
	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr, startUI))
}

// run loads the config named by the command-line args, hands it to start and
// returns the exit code. Errors are written to stderr.
func run(args []string, stderr io.Writer, start func(home string, s config.Settings) error) int {
	flags := flag.NewFlagSet("loper", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configFile := flags.String("config", "", "use this config `file` instead of the standard one")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "loper:", err)
		return 1
	}
	settings, err := config.Load(*configFile, home)
	if err != nil {
		fmt.Fprintln(stderr, "loper:", err)
		return 1
	}
	if err := start(home, settings); err != nil {
		fmt.Fprintln(stderr, "loper:", err)
		return 1
	}
	return 0
}

// startUI runs the picker until it quits.
func startUI(home string, settings config.Settings) error {
	m := ui.New(ui.Deps{
		Home:        home,
		ConfigFile:  settings.File,
		Paths:       settings.Paths,
		Scan:        scanner.Scan,
		Missing:     scanner.Missing,
		Branch:      repoinfo.Branch,
		Exists:      repoinfo.Exists,
		Open:        func(t target.Target, path string) error { return target.Run(t, path, 2*time.Second) },
		Targets:     settings.Targets,
		History:     history.New(home),
		Cache:       cache.New(home),
		Now:         time.Now,
		Tick:        tea.Tick,
		CloseOnOpen: settings.CloseOnOpen,
		SortByName:  settings.SortByName,
		CacheMaxAge: settings.CacheMaxAge,
		Color:       os.Getenv("NO_COLOR") == "",
		Columns:     settings.Columns,
		RowStyle:    settings.RowStyle,
	})
	_, err := tea.NewProgram(m).Run()
	return err
}
