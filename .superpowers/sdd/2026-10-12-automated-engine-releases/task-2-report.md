# Task 2: Version calculation CLI and history-range contract

## Status

Implemented the `cmd/tusk-release` CLI, focused unit and local-Git integration
tests, and the contributor guidance for squash and regular merge release inputs.

## Behavior

The CLI requires `--previous-tag` and `--target-commit`. It accepts a stable
`MAJOR.MINOR.PATCH` tag with an optional `v` prefix and a full 40- or 64-digit
hexadecimal commit SHA. It runs only the read-only command:

```text
git log --no-merges --format=%B%x00 <previous-tag>..<target-commit>
```

The NUL record separator preserves full `%B` messages while separating adjacent
commits, including messages with multiline bodies. Git's `--no-merges` omits
synthetic merge wrappers while retaining non-merge commits reachable through a
regular GitHub merge. The CLI passes each complete message to
`release.CalculateNextVersion` and writes only these two lines on success:

```text
version=<value>
should_release=true|false
```

Invalid arguments, tag/SHA values, Git failures, and malformed release-relevant
messages return an error before stdout is written. Errors are printed to stderr
by `main`.

## TDD and verification evidence

1. Added CLI tests before implementation. `go test ./cmd/tusk-release` failed at
   compile time with `undefined: run`, confirming the implementation was absent.
2. Added CLI behavior and history-range integration coverage, then implemented
   the command and documentation.
3. The local Git integration fixture creates a tag, a Conventional Commit on
   the mainline, a feature branch containing a Conventional Commit, a two-parent
   synthetic pull-request merge wrapper, and a later mainline commit. It
   confirms both included non-merge messages are considered while the synthetic
   merge wrapper is omitted.
4. `go test ./cmd/tusk-release ./internal/release` passed.
5. `git diff --check` passed.

The first attempt to use Go's default build cache was denied by the environment.
The passing focused tests were rerun with `GOCACHE` located in the writable
temporary directory; no cache artifacts remain in the worktree.

## Review notes

- The brief describes `--format=%B <range>`. The implementation uses
  `--format=%B%x00 <range>` so complete commit messages can be reliably
  separated. This adds only a NUL record delimiter; `%B` remains the message
  format, and the range and `--no-merges` contract are unchanged.
- Tag syntax is validated, while tag existence and ancestry are delegated to
  `git log`; failure is reported without producing output.
- No network commands or Git mutations are performed by the CLI.
- Review preserves message contents verbatim: parsing removes only the trailing
  NUL record delimiter, not whitespace from commit bodies.
- Workspace guidance requires synchronizing local `main` from `origin/main`
  before new work. That sync was not performed because the Task 2 brief
  explicitly prohibits network access; implementation proceeded in the
  already-existing feature worktree.

## Commit

`feat: expose deterministic release calculation to CI` —
`9918b81eb47340954f022fcb9bb3236c8ec602d1`.
