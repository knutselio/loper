// Package repoinfo reads what a repo has checked out, straight from its
// HEAD file, so the tool never runs git.
package repoinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// Branch returns the branch the repo at path has checked out, or the short
// commit hash prefixed with @ for a detached HEAD. It returns "" if HEAD
// cannot be read or holds neither.
func Branch(path string) string {
	b, err := os.ReadFile(filepath.Join(path, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))
	if branch, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return branch
	}
	if !isHash(head) {
		return ""
	}
	return "@" + head[:7]
}

// isHash reports whether s is a full SHA-1 or SHA-256 commit hash.
func isHash(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}
