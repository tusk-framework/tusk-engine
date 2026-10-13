package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		previous   string
		target     string
		messages   string
		gitErr     error
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "release",
			previous:   "v1.2.3",
			target:     strings.Repeat("a", 40),
			messages:   "fix: repair a leak\n\nbody line\x00",
			wantOutput: "version=1.2.4\nshould_release=true\n",
		},
		{
			name:       "no release",
			previous:   "v1.2.3",
			target:     strings.Repeat("b", 40),
			messages:   "docs: update guide\x00",
			wantOutput: "version=1.2.3\nshould_release=false\n",
		},
		{
			name:     "malformed commit input has no partial output",
			previous: "v1.2.3",
			target:   strings.Repeat("c", 40),
			messages: "feat add widget\x00",
			wantErr:  true,
		},
		{
			name:    "git failure has no partial output",
			gitErr:  errors.New("git failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			git := func(args ...string) ([]byte, error) {
				if tt.gitErr != nil {
					return nil, tt.gitErr
				}
				if len(args) != 4 || args[0] != "log" || args[1] != "--no-merges" || args[2] != "--format=%B%x00" || args[3] != tt.previous+".."+tt.target {
					t.Fatalf("unexpected git arguments: %q", args)
				}
				return []byte(tt.messages), nil
			}
			err := run([]string{"--previous-tag", tt.previous, "--target-commit", tt.target}, &stdout, git)
			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got := stdout.String(); got != tt.wantOutput {
				t.Errorf("stdout = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

func TestRunValidateMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "feat", message: "feat: add widget"},
		{name: "fix", message: "fix(api): handle nil input"},
		{name: "perf", message: "perf: improve startup"},
		{name: "arbitrary type", message: "docs: update guide"},
		{name: "breaking bang", message: "refactor!: replace public API"},
		{name: "breaking footer", message: "docs: remove old guide\n\nBREAKING CHANGE: old guide URL is no longer supported"},
		{name: "malformed title", message: "feat add widget", wantErr: true},
		{name: "invalid utf8", message: "fix: repair\xff", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run([]string{"validate-message", "--message", tt.message}, &stdout, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr %t", err, tt.wantErr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestRunAutomaticPlan(t *testing.T) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const next = "v1.2.4"
	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantErr    bool
	}{
		{name: "create", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", sha, "--next-tag", next}, wantOutput: "action=create\\ntag=v1.2.4\\n"},
		{name: "resume draft", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", sha, "--candidate-tag", next, "--candidate-tag-sha", sha, "--candidate-release", "draft"}, wantOutput: "action=resume\\ntag=v1.2.4\\n"},
		{name: "resume missing release", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", strings.Repeat("b", 40), "--candidate-tag", next, "--candidate-tag-sha", sha, "--candidate-release", "absent"}, wantOutput: "action=resume\\ntag=v1.2.4\\n"},
		{name: "published is idempotent", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", strings.Repeat("b", 40), "--candidate-tag", next, "--candidate-tag-sha", sha, "--candidate-release", "published"}, wantOutput: "action=already-published\\ntag=v1.2.4\\n"},
		{name: "skip stale", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--next-tag", next}, wantOutput: "action=skip-stale\\ntag=\\n"},
		{name: "no release", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", sha}, wantOutput: "action=no-version\\ntag=\\n"},
		{name: "reject conflicting tag", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", sha, "--next-tag", next, "--candidate-tag", next, "--candidate-tag-sha", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--candidate-release", "draft"}, wantOutput: "", wantErr: true},
		{name: "reject unknown release state", args: []string{"automatic-plan", "--validated-sha", sha, "--remote-main-sha", sha, "--candidate-tag", next, "--candidate-tag-sha", sha, "--candidate-release", "pending"}, wantOutput: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run(test.args, &stdout, nil)
			if (err != nil) != test.wantErr {
				t.Fatalf("run() error = %v, wantErr %t", err, test.wantErr)
			}
			if got := strings.ReplaceAll(stdout.String(), "\n", "\\n"); got != test.wantOutput {
				t.Errorf("stdout = %q, want %q", got, test.wantOutput)
			}
		})
	}
}

func TestRunRejectsInvalidInputsBeforeGit(t *testing.T) {
	tests := [][]string{
		{"--previous-tag", "-x", "--target-commit", strings.Repeat("a", 40)},
		{"--previous-tag", "v01.2.3", "--target-commit", strings.Repeat("a", 40)},
		{"--previous-tag", "v1.2.3..v9.9.9", "--target-commit", strings.Repeat("a", 40)},
		{"--previous-tag", "v1.2.3", "--target-commit", "HEAD"},
		{"--previous-tag", "v1.2.3", "--target-commit", strings.Repeat("a", 39)},
		{"--previous-tag", "v1.2.3", "--target-commit", strings.Repeat("g", 40)},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout bytes.Buffer
			gitCalled := false
			git := func(...string) ([]byte, error) {
				gitCalled = true
				return nil, nil
			}
			if err := run(args, &stdout, git); err == nil {
				t.Fatal("run() unexpectedly succeeded")
			}
			if gitCalled {
				t.Error("git was called for invalid input")
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestGitLogHistoryRange(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	runGit("init", "-q")
	runGit("checkout", "-q", "-b", "main")
	runGit("commit", "--allow-empty", "-qm", "chore: baseline")
	runGit("tag", "v1.2.3")
	runGit("commit", "--allow-empty", "-m", "fix: inner fix")
	inner := strings.TrimSpace(string(runGit("rev-parse", "HEAD")))
	runGit("checkout", "-q", "-b", "feature/example", "v1.2.3")
	runGit("commit", "--allow-empty", "-m", "feat: add branch feature")
	branchCommit := strings.TrimSpace(string(runGit("rev-parse", "HEAD")))
	runGit("checkout", "-q", "main")
	runGit("merge", "--no-ff", "-m", "Merge pull request #42 from feature/example", branchCommit)
	runGit("commit", "--allow-empty", "-m", "feat: add widget")
	target := strings.TrimSpace(string(runGit("rev-parse", "HEAD")))

	var stdout bytes.Buffer
	err := run([]string{"--previous-tag", "v1.2.3", "--target-commit", target}, &stdout, func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		return cmd.Output()
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "version=1.3.0\nshould_release=true\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	commits := strings.Fields(string(runGit("log", "--no-merges", "--format=%H", "v1.2.3.."+target)))
	if len(commits) != 3 || !contains(commits, inner) || !contains(commits, branchCommit) {
		t.Fatalf("non-merge commits = %v, want inner %s, branch %s, and post-merge feat", commits, inner, branchCommit)
	}
}

func TestPreviousTagSelectsNearestFirstParentStableTag(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	runGit("init", "-q")
	runGit("checkout", "-q", "-b", "main")
	runGit("commit", "--allow-empty", "-qm", "chore: baseline")
	runGit("tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	runGit("checkout", "-q", "-b", "feature/unreleased")
	runGit("commit", "--allow-empty", "-qm", "feat: unfinished branch")
	runGit("tag", "v9.0.0")
	runGit("checkout", "-q", "main")
	runGit("commit", "--allow-empty", "-qm", "fix: main patch")
	runGit("tag", "-a", "v1.1.0", "-m", "Release v1.1.0")
	runGit("merge", "--no-ff", "-m", "Merge feature branch", "feature/unreleased")
	runGit("commit", "--allow-empty", "-qm", "feat: next main change")
	runGit("tag", "-a", "v1.2.0", "-m", "Release v1.2.0")
	target := strings.TrimSpace(string(runGit("rev-parse", "HEAD")))

	git := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		return cmd.Output()
	}
	var stdout bytes.Buffer
	err := run([]string{"previous-tag", "--candidate-tag", "v1.2.0", "--expected-sha", target}, &stdout, git)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "tag=v1.1.0\n"; got != want {
		t.Fatalf("previous stable tag = %q, want %q", got, want)
	}

	stdout.Reset()
	if err := run([]string{"previous-tag", "--candidate-tag", "v1.2.0", "--expected-sha", strings.Repeat("0", 40)}, &stdout, git); err == nil {
		t.Fatal("previous-tag accepted a candidate ref resolving to a different source SHA")
	}
	runGit("tag", "v1.2.1", target)
	stdout.Reset()
	if err := run([]string{"previous-tag", "--candidate-tag", "v1.2.1", "--expected-sha", target}, &stdout, git); err == nil {
		t.Fatal("previous-tag accepted a lightweight candidate tag")
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
