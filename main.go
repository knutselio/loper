// Command loper is a keyboard-driven terminal picker for git repos.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knutselio/loper/internal/cache"
	"github.com/knutselio/loper/internal/config"
	"github.com/knutselio/loper/internal/history"
	"github.com/knutselio/loper/internal/repoinfo"
	"github.com/knutselio/loper/internal/scanner"
	"github.com/knutselio/loper/internal/target"
	"github.com/knutselio/loper/internal/ui"
	"github.com/knutselio/loper/internal/xdg"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, startUI))
}

// run prints the help or the version to stdout, or loads the config named by
// the command-line args and hands it to start, and returns the exit code.
// Errors are written to stderr.
func run(args []string, stdout, stderr io.Writer, start func(home string, s config.Settings) error) int {
	// The flag package's own messages and usage are replaced by usage.
	flags := flag.NewFlagSet("loper", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configFile := flags.String("config", "", "")
	showVersion := flags.Bool("version", false, "")
	showHelp := flags.Bool("help", false, "")
	flags.BoolVar(showHelp, "h", false, "")
	err := flags.Parse(args)
	noFile := errors.New("--config needs a file")
	// The flag package names a flag with one dash; usage writes two.
	if name, ok := strings.CutPrefix(fmt.Sprint(err), "flag provided but not defined: -"); ok {
		err = fmt.Errorf("unknown flag --%s", name)
	}
	if strings.HasPrefix(fmt.Sprint(err), "flag needs an argument") { // only --config takes one
		err = noFile
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "config" && *configFile == "" && err == nil {
			err = noFile
		}
	})
	if err == nil && flags.NArg() > 0 {
		err = fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if err != nil {
		fmt.Fprintf(stderr, "loper: %v\n\n%s", err, usage)
		return 2
	}
	if *showHelp {
		home := ""
		// Only the standard location without XDG_CONFIG_HOME is in the home folder.
		if *configFile == "" && xdg.Dir("XDG_CONFIG_HOME", "") == "" {
			if home, err = os.UserHomeDir(); err != nil {
				fmt.Fprintln(stderr, "loper:", err)
				return 1
			}
		}
		help(stdout, config.Path(*configFile, home))
		return 0
	}
	if *showVersion {
		fmt.Fprintln(stdout, "loper", version())
		return 0
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

// usage is the synopsis and the flags, for --help and for a usage error.
const usage = `Usage: loper [--config <file>]

Options:
  --config <file>  Use this config file instead of the standard one
  -h, --help       Print this help
  --version        Print the version
`

// help prints the usage, the config file that would be read, where targets
// come from and loper's own keys.
func help(w io.Writer, configFile string) {
	fmt.Fprintf(w, "loper picks one of your git repos and opens it.\n\n%s\nConfig file: %s\n", usage, configFile)
	fmt.Fprintln(w, "Targets are defined in the config file, or added in the picker with a.")
	sections := ui.KeyList()
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.Bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.Keys))
		}
	}
	for _, s := range sections {
		title := s.Title
		if title == "" {
			title = "Keys"
		}
		fmt.Fprintf(w, "\n%s:\n", title)
		for _, b := range s.Bindings {
			fmt.Fprintf(w, "  %s%s  %s\n", b.Keys, strings.Repeat(" ", keyWidth-ansi.StringWidth(b.Keys)), b.Action)
		}
	}
}

// version is the module version that go install and go build stamp into the
// binary, such as v0.0.0-20261111120000-3c250a4e1f2b. The toolchain stamps
// (devel) when it has none, as in a test binary.
func version() string {
	info, _ := debug.ReadBuildInfo() // fails only in a binary built without modules, which this one cannot be
	return info.Main.Version
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
		CheckTarget: config.CheckTarget,
		AddTarget:   config.AddTarget,
	})
	_, err := tea.NewProgram(m).Run()
	return err
}
