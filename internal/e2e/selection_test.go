package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTheBuiltBinaryWithSelectionMarkerDrawsTheMarkerAndNoReverseVideo(t *testing.T) {
	loper := filepath.Join(t.TempDir(), "loper")
	if out, err := exec.Command("go", "build", "-o", loper, "github.com/knutselio/loper").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	home := tempHome(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if out, err := exec.Command("git", "init", "-b", "main", filepath.Join(home, "code", "alpha")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	config := filepath.Join(home, "try.toml")
	writeConfig(t, config, "paths = [\""+filepath.Join(home, "code")+"\"]\nselection = \"marker\"\n")

	s := start(t, "> alpha", loper, "--config", config)
	s.within("the list to be drawn", s.shown)
	s.eventually("the refresh to be done", func() bool { return s.shows("1 repo loaded") })
	out := s.quit()

	if marker := regexp.MustCompile(`\x1b\[([0-9]+;)*1(;[0-9]+)*m>`); !marker.Match(out) {
		t.Errorf("want the bold marker:\n%q", out)
	}
	for _, seq := range sgr.FindAll(out, -1) {
		for _, p := range strings.Split(string(seq[2:len(seq)-1]), ";") {
			if p == "7" {
				t.Fatalf("the terminal got reverse video, %q:\n%q", seq, out)
			}
		}
	}
}
