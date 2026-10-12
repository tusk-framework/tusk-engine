package release

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/leodido/go-conventionalcommits"
	"github.com/leodido/go-conventionalcommits/parser"
)

var stableVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var releaseRelevantHeader = regexp.MustCompile(`(?i)^(feat|fix|perf)(?:\([^\r\n)]*\))?!?(?:\s|:|$)`)
var breakingMetadata = regexp.MustCompile(`(?im)^\s*BREAKING CHANGE\s*:`)
var validBreakingMetadata = regexp.MustCompile(`(?im)^\s*BREAKING CHANGE:\s*\S`)

const (
	noBump = iota
	patchBump
	minorBump
	majorBump
)

// CalculateNextVersion calculates the next stable version from Conventional Commit messages.
// The returned version never includes the tag's leading "v".
func CalculateNextVersion(current string, messages []string) (string, bool, error) {
	version := strings.TrimPrefix(current, "v")
	parts := stableVersionPattern.FindStringSubmatch(current)
	if parts == nil {
		return "", false, fmt.Errorf("invalid current version %q: expected stable vMAJOR.MINOR.PATCH", current)
	}

	major, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", false, fmt.Errorf("invalid current version %q: major component overflows", current)
	}
	minor, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return "", false, fmt.Errorf("invalid current version %q: minor component overflows", current)
	}
	patchVersion, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return "", false, fmt.Errorf("invalid current version %q: patch component overflows", current)
	}
	version = fmt.Sprintf("%d.%d.%d", major, minor, patchVersion)

	var bump int
	for _, message := range messages {
		if breakingMetadata.MatchString(message) && !validBreakingMetadata.MatchString(message) {
			return "", false, fmt.Errorf("invalid Conventional Commit message %q: BREAKING CHANGE footer must have a value", message)
		}
		parsed, err := parseMessage(message)
		if err != nil {
			if isReleaseRelevant(message) || breakingMetadata.MatchString(message) || hasReleaseRelevantHeader(message) {
				return "", false, fmt.Errorf("invalid Conventional Commit message %q: %w", message, err)
			}
			continue
		}

		commit, ok := parsed.(*conventionalcommits.ConventionalCommit)
		if !ok {
			return "", false, fmt.Errorf("unexpected Conventional Commit parser result for %q", message)
		}
		if commit.IsBreakingChange() {
			bump = majorBump
		} else if bump < minorBump && commit.Type == "feat" {
			bump = minorBump
		} else if bump < patchBump && (commit.Type == "fix" || commit.Type == "perf") {
			bump = patchBump
		}
	}

	if bump == noBump {
		return version, false, nil
	}
	if bump == majorBump {
		if major == math.MaxUint64 {
			return "", false, fmt.Errorf("cannot increment current version %q: major component overflows", current)
		}
		major++
		minor, patchVersion = 0, 0
	} else if bump == minorBump {
		if minor == math.MaxUint64 {
			return "", false, fmt.Errorf("cannot increment current version %q: minor component overflows", current)
		}
		minor++
		patchVersion = 0
	} else {
		if patchVersion == math.MaxUint64 {
			return "", false, fmt.Errorf("cannot increment current version %q: patch component overflows", current)
		}
		patchVersion++
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patchVersion), true, nil
}

func parseMessage(message string) (conventionalcommits.Message, error) {
	machine := parser.NewMachine(parser.WithTypes(conventionalcommits.TypesFreeForm))
	strictMachine, ok := machine.(parser.Machine)
	if ok {
		strictMachine.WithStrictUTF8()
	}
	return machine.Parse([]byte(message))
}

func isReleaseRelevant(message string) bool {
	header := strings.SplitN(message, "\n", 2)[0]
	return releaseRelevantHeader.MatchString(header)
}

func hasReleaseRelevantHeader(message string) bool {
	header := strings.SplitN(message, "\n", 2)[0]
	header = strings.TrimSpace(header)
	for _, prefix := range []string{"feat", "fix", "perf"} {
		if strings.HasPrefix(strings.ToLower(header), prefix) {
			return true
		}
	}
	return false
}
