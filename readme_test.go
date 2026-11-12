package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readmeTOML returns the toml blocks in the README, in order.
func readmeTOML(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	var blocks []string
	rest := string(b)
	for {
		var block string
		var ok bool
		if _, rest, ok = strings.Cut(rest, "```toml\n"); !ok {
			break
		}
		if block, rest, ok = strings.Cut(rest, "```"); !ok {
			t.Fatalf("README.md has a toml block that is not closed:\n%s", block)
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		t.Fatal("README.md has no toml blocks")
	}
	return blocks
}

func TestEveryConfigExampleInTheReadmeLoads(t *testing.T) {
	for i, block := range readmeTOML(t) {
		home := tempHome(t)
		file := filepath.Join(home, "config.toml")
		writeConfig(t, file, block)

		code, stderr, _ := picker(t, "--config", file)

		if code != 0 || stderr != "" {
			t.Errorf("toml block %d: exit code %d, stderr %q; want 0 and nothing:\n%s", i+1, code, stderr, block)
		}
	}
}
