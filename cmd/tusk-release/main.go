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
