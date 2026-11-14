# AGENTS.md

`loper` is a terminal picker for git repos. SPEC.md holds the product and the
design decisions. Read it before changing behaviour, and don't repeat it here.

## Checks

    go vet ./...
    go test -race ./...
    go install .     # builds loper into $GOBIN, or $(go env GOPATH)/bin

CI runs the first two on macos-latest for every push and pull request.

## Testing

Test behaviour through the seam in SPEC.md's "Testing Decisions": the driver in
`internal/ui/driver_test.go` feeds keys to the real program and the test checks
the screen text and the side effects. Never assert on internal state, message
types or function calls. Nothing is mocked: fixtures are real repos made with
`git init -b main` in a temp tree, and targets are fake scripts. Tests must never
touch the real home, config, cache or state folders, and never run Zed.

## Commits

One commit per ticket. The subject is one imperative line of at most 50
characters that starts with Add, Fix, Change, Remove or Refactor, with no type
prefix such as "feat:" and no trailing period. Make it specific without being verbose: "Add a local-only
development seeder for assignment", not "Add local seeder". Add a body only when
it carries something the subject cannot, as one small paragraph, never a list.
No AI attribution or co-author lines of any kind.
