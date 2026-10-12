# Task 1 report: Deterministic semantic-version calculator

## Result

Implemented `internal/release.CalculateNextVersion` using the pinned
`github.com/leodido/go-conventionalcommits` v0.13.0 parser.

The calculator accepts stable `MAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH`
input, returns an unprefixed version, and selects major over minor over patch.
`feat` increments minor; `fix` and `perf` increment patch; a valid `!` marker
or nonempty `BREAKING CHANGE:` footer increments major. Other valid commit
types and an empty message range do not trigger a release. Malformed
release-relevant messages and malformed breaking metadata return errors.
Numeric parse and increment overflow are rejected.

## Parsing contract

- Commit messages are parsed strictly with Conventional Commit free-form
  types, allowing project-specific types to remain non-releasing by default.
- A malformed message beginning with a release-relevant type is rejected,
  including malformed scope/header syntax and malformed bodies or footers.
- Breaking metadata is recognized case-insensitively as a footer label by the
  parser; an empty `BREAKING CHANGE:` value is rejected.
- Valid, non-release types do not independently trigger a release; their
  valid breaking metadata still triggers a major release.
- Current versions must be stable three-component numeric versions, optionally
  prefixed by one lowercase `v`; prerelease/build metadata and leading zeroes
  are rejected.

## RED/GREEN evidence

RED, after adding the table-driven tests and before implementing the function:

```text
Command: go test ./internal/release
Result: FAIL
internal/release/version_test.go:98:35: undefined: CalculateNextVersion
```

An early GREEN attempt exposed that the parser accepts an empty breaking
footer value. Added validation and a regression case; the final focused GREEN
run is:

```text
Command: go test ./internal/release
Result: PASS
ok github.com/tusk-framework/tusk-engine/internal/release
```

## Integration verification

After implementation and dependency cleanup:

```text
Command: go test ./internal/release
Result: PASS

Command: go test ./...
Result: PASS; all packages passed, packages without tests reported normally.

Command: go vet ./...
Result: PASS; no diagnostics.

Command: git diff --check
Result: PASS; no whitespace errors.
```

The Go module cache/build cache initially denied access under the default
sandbox. The required commands completed after sandbox approval; there is no
remaining test or toolchain blocker.

## Files changed

- `internal/release/version.go`
- `internal/release/version_test.go`
- `go.mod`
- `go.sum`

## Self-review

- Verified bump precedence, no-release behavior, empty ranges, footer and bang
  breaking changes, malformed release metadata, malformed headers, stable
  version normalization, and numeric overflow through table-driven tests.
- Confirmed the parser dependency is pinned to v0.13.0 and checksums are
  recorded.
- Compared the task changes against review base
  `fc1a87b32fd403ee7a48e1056e93a8db18bc1a94`; no plan/spec edits are included.
- No CLI or workflow files were changed.

## Commit

- Implementation: `95c8916` (`feat: calculate semantic engine release versions`)
- Report: `4286cbe` (`docs: record version calculator implementation report`)
