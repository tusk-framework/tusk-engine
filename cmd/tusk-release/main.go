package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/release"
)

var stableTagPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var stableReleaseTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitSHA1Pattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
var commitSHA256Pattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

type gitRunner func(args ...string) ([]byte, error)

func main() {
	if err := run(os.Args[1:], os.Stdout, execGit); err != nil {
		fmt.Fprintln(os.Stderr, "tusk-release:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, git gitRunner) error {
	if len(args) > 0 && args[0] == "validate-message" {
		flags := flag.NewFlagSet("validate-message", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		message := flags.String("message", "", "Conventional Commit message to validate")
		if err := flags.Parse(args[1:]); err != nil {
			return fmt.Errorf("invalid arguments: %w", err)
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected positional arguments")
		}
		if *message == "" {
			return errors.New("missing required --message")
		}
		return release.ValidateConventionalCommit(*message)
	}
	if len(args) > 0 && args[0] == "automatic-plan" {
		return runAutomaticPlan(args[1:], stdout)
	}
	if len(args) > 0 && args[0] == "previous-tag" {
		return runPreviousTag(args[1:], stdout, git)
	}

	flags := flag.NewFlagSet("tusk-release", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	previousTag := flags.String("previous-tag", "", "previous stable release tag")
	targetCommit := flags.String("target-commit", "", "target commit SHA")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := validateTag(*previousTag); err != nil {
		return err
	}
	if err := validateCommit(*targetCommit); err != nil {
		return err
	}

	output, err := git("log", "--no-merges", "--format=%B%x00", *previousTag+".."+*targetCommit)
	if err != nil {
		return fmt.Errorf("read commit history: %w", err)
	}
	version, shouldRelease, err := release.CalculateNextVersion(*previousTag, splitCommitMessages(string(output)))
	if err != nil {
		return err
	}
	outputLine := fmt.Sprintf("version=%s\nshould_release=%t\n", version, shouldRelease)
	_, err = io.WriteString(stdout, outputLine)
	return err
}

func runPreviousTag(args []string, stdout io.Writer, git gitRunner) error {
	flags := flag.NewFlagSet("previous-tag", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	candidateTag := flags.String("candidate-tag", "", "stable release tag whose first-parent predecessor is needed")
	expectedSHA := flags.String("expected-sha", "", "expected commit resolved by the candidate tag")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if !stableReleaseTagPattern.MatchString(*candidateTag) {
		return errors.New("candidate tag must be a stable vMAJOR.MINOR.PATCH tag")
	}
	if err := validateCommit(*expectedSHA); err != nil {
		return fmt.Errorf("invalid expected SHA: %w", err)
	}
	tagType, err := git("cat-file", "-t", "refs/tags/"+*candidateTag)
	if err != nil {
		return fmt.Errorf("inspect candidate tag: %w", err)
	}
	if strings.TrimSpace(string(tagType)) != "tag" {
		return errors.New("candidate stable release tag must be annotated")
	}
	candidateCommit, err := git("rev-parse", "--verify", *candidateTag+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve candidate tag: %w", err)
	}
	candidateSHA := strings.TrimSpace(string(candidateCommit))
	if candidateSHA != *expectedSHA {
		return errors.New("candidate tag does not resolve to the expected source SHA")
	}
	history, err := git("rev-list", "--first-parent", strings.TrimSpace(string(candidateCommit)))
	if err != nil {
		return fmt.Errorf("read candidate first-parent history: %w", err)
	}
	commits := strings.Fields(string(history))
	for _, commit := range commits[1:] {
		output, err := git("tag", "--points-at", commit)
		if err != nil {
			return fmt.Errorf("read tags at commit %s: %w", commit, err)
		}
		var stableTags []string
		for _, tag := range strings.Fields(string(output)) {
			if stableReleaseTagPattern.MatchString(tag) {
				tagType, err := git("cat-file", "-t", "refs/tags/"+tag)
				if err != nil {
					return fmt.Errorf("inspect previous stable tag %s: %w", tag, err)
				}
				if strings.TrimSpace(string(tagType)) != "tag" {
					return fmt.Errorf("previous stable release tag %s must be annotated", tag)
				}
				tagCommit, err := git("rev-parse", "--verify", "refs/tags/"+tag+"^{commit}")
				if err != nil {
					return fmt.Errorf("resolve previous stable tag %s: %w", tag, err)
				}
				if strings.TrimSpace(string(tagCommit)) != commit {
					return fmt.Errorf("previous stable tag %s does not resolve to the first-parent commit being examined", tag)
				}
				stableTags = append(stableTags, tag)
			}
		}
		if len(stableTags) > 1 {
			return fmt.Errorf("multiple stable release tags point at first-parent commit %s", commit)
		}
		if len(stableTags) == 1 {
			_, err := fmt.Fprintf(stdout, "tag=%s\n", stableTags[0])
			return err
		}
	}
	_, err = io.WriteString(stdout, "tag=\n")
	return err
}

func runAutomaticPlan(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("automatic-plan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	validatedSHA := flags.String("validated-sha", "", "validated source commit SHA")
	remoteMainSHA := flags.String("remote-main-sha", "", "current remote main commit SHA")
	nextTag := flags.String("next-tag", "", "calculated next stable tag, if a release is due")
	candidateTag := flags.String("candidate-tag", "", "existing tag to reconcile")
	candidateTagSHA := flags.String("candidate-tag-sha", "", "commit resolved by the existing candidate tag")
	candidateRelease := flags.String("candidate-release", "unknown", "existing release state: unknown, absent, draft, or published")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	state := release.ReleaseUnknown
	switch *candidateRelease {
	case "unknown":
	case "absent":
		state = release.ReleaseAbsent
	case "draft":
		state = release.ReleaseDraft
	case "published":
		state = release.ReleasePublished
	default:
		return errors.New("invalid candidate release state")
	}
	plan := release.PlanAutomaticRelease(release.ReleasePlanInput{
		ValidatedSHA:     *validatedSHA,
		RemoteMainSHA:    *remoteMainSHA,
		NextTag:          *nextTag,
		CandidateTag:     *candidateTag,
		CandidateTagSHA:  *candidateTagSHA,
		CandidateRelease: state,
	})
	if plan.Action == release.ReleaseConflict {
		return errors.New("release tag state conflicts with the validated commit")
	}
	_, err := fmt.Fprintf(stdout, "action=%s\ntag=%s\n", plan.Action, plan.Tag)
	return err
}

func validateTag(tag string) error {
	if !stableTagPattern.MatchString(tag) {
		return fmt.Errorf("invalid previous tag %q: expected stable vMAJOR.MINOR.PATCH", tag)
	}
	return nil
}

func validateCommit(commit string) error {
	if !commitSHA1Pattern.MatchString(commit) && !commitSHA256Pattern.MatchString(commit) {
		return errors.New("invalid target commit: expected a full 40- or 64-character hexadecimal SHA")
	}
	return nil
}

func splitCommitMessages(output string) []string {
	if output == "" {
		return nil
	}
	messages := strings.Split(output, "\x00")
	if messages[len(messages)-1] == "" {
		messages = messages[:len(messages)-1]
	}
	return messages
}

func execGit(args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	return command.Output()
}
