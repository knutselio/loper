// Command loper is a keyboard-driven terminal picker for git repos.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
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

// drainArg is the hidden mode in which loper drains a target's output; see
// target.Run.
const drainArg = "--drain-target-output"

func main() {
	// Before anything else: the drain has no terminal, and must not read or
	// create the config.
	if len(os.Args) == 2 && os.Args[1] == drainArg {
		// A target dies of SIGPIPE on its next write once the drain is gone,
		// so the drain ends only with the last writer, or by SIGKILL, and
		// not when killall loper or a closing terminal asks loper to.
		signal.Ignore(syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		if err := target.Drain(); err != nil {
			fmt.Fprintf(os.Stderr, "loper: %s: %v\n", drainArg, err)
			os.Exit(2)
		}
		return
	}
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
	flags.BoolVar(showVersion, "v", false, "")
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
		// Only the standard location without XDG_CONFIG_HOME is in the home
		// folder. The targets do not depend on it, so without it they still load.
		if *configFile == "" && xdg.Dir("XDG_CONFIG_HOME", "") == "" {
			if home, err = os.UserHomeDir(); err != nil {
				fmt.Fprintln(stderr, "loper:", err)
				return 1
			}
		}
		// A config that does not load is shown in the help, not an error.
		settings, err := config.Load(*configFile, home)
		file := config.Path(*configFile, home)
		if _, statErr := os.Stat(file); *configFile == "" && errors.Is(statErr, fs.ErrNotExist) {
			settings, file = config.FirstRun(home), file+" (does not exist yet)"
		}
		help(stdout, file, settings.Targets, err)
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
	settings, err := config.Open(*configFile, home)
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
  -v, --version    Print the version
`

// help prints the usage, the config file that would be read, loper's own keys
// and the configured targets, or why the config file does not load.
func help(w io.Writer, configFile string, targets []target.Target, loadErr error) {
	fmt.Fprintf(w, "loper picks one of your git repos and opens it.\n\n%s\nConfig file: %s\n", usage, configFile)
	sections := ui.KeyList()
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.Bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.Keys))
		}
	}
	row := func(keys, text string) {
		fmt.Fprintf(w, "  %s%s  %s\n", keys, strings.Repeat(" ", keyWidth-ansi.StringWidth(keys)), text)
	}
	for _, s := range sections {
		title := s.Title
		if title == "" {
			title = "Keys"
		}
		fmt.Fprintf(w, "\n%s:\n", title)
		for _, b := range s.Bindings {
			row(b.Keys, b.Action)
		}
	}
	fmt.Fprint(w, "\nTargets:\n")
	switch {
	case loadErr != nil:
		fmt.Fprintf(w, "  %v\n", loadErr)
	case len(targets) == 0:
		fmt.Fprintln(w, "  none; add one in the picker with a, or in the config file")
	}
	for _, t := range targets {
		row(t.Key, t.Name)
	}
}

// version is the module version that go install and go build stamp into the
// binary, such as v0.0.0-20261111120000-3c250a4e1f2b. The toolchain stamps
// (devel) when it has none, as in a test binary.
func version() string {
	info, _ := debug.ReadBuildInfo() // fails only in a binary built without modules, which this one cannot be
	return info.Main.Version
}

// startUI runs the picker until it quits, then hands the terminal over to the
// in-place target it ended on, if any.
func startUI(home string, settings config.Settings) error {
	cdFile := takeCdFile()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	drain := []string{self, drainArg}
	m := ui.New(ui.Deps{
		Home:        home,
		ConfigFile:  settings.File,
		Paths:       settings.Paths,
		Scan:        scanner.Scan,
		Missing:     scanner.Missing,
		Branch:      repoinfo.Branch,
		Exists:      repoinfo.Exists,
		Open:        func(t target.Target, path string) error { return target.Run(t, path, 2*time.Second, drain) },
		Targets:     settings.Targets,
		History:     history.New(home),
		Cache:       cache.New(home),
		Now:         time.Now,
		Tick:        tea.Tick,
		Notice:      settings.Notice,
		NoticeFor:   3 * time.Second,
		EnterPause:  300 * time.Millisecond,
		CloseOnOpen: settings.CloseOnOpen,
		SortByName:  settings.SortByName,
		CacheMaxAge: settings.CacheMaxAge,
		Styles:      true,
		Color:       os.Getenv("NO_COLOR") == "",
		Columns:     settings.Columns,
		RowStyle:    settings.RowStyle,
		CheckTarget: config.CheckTarget,
		CheckFile:   config.CheckFile,
		WriteTarget: config.WriteTarget,
	})
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	// The program has ended and the terminal is restored, so an in-place
	// target can take the tool's place.
	if h, ok := final.(ui.Model).HandOver(); ok {
		return handOver(h, cdFile)
	}
	return nil
}

// takeCdFile returns the shell wrapper's file in LOPER_CD_FILE and removes the
// variable, so no target or hand-over gets it: a loper started from one would
// otherwise write to the outer wrapper's file.
func takeCdFile() string {
	file := os.Getenv("LOPER_CD_FILE")
	os.Unsetenv("LOPER_CD_FILE")
	return file
}

// handOver is the hand-over for h, but for a cd command run with the shell
// wrapper, whose file cdFile gets the folder for the wrapper to change to.
func handOver(h target.HandOver, cdFile string) error {
	if h.Cd && cdFile != "" {
		return os.WriteFile(cdFile, []byte(h.Dir), 0o600)
	}
	return replace(h)
}

// replace replaces the tool's process with h's command, run in its folder
// with the tool's environment and terminal. It returns only when it fails.
func replace(h target.HandOver) error {
	if err := os.Chdir(h.Dir); err != nil {
		return err
	}
	os.Setenv("PWD", h.Dir) // as os/exec sets it for a normal target, so a shell's pwd is the folder as given
	return syscall.Exec(h.Program, h.Args, os.Environ())
}
