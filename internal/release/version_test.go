package release

import (
	"strings"
	"testing"
)

func TestCalculateNextVersion(t *testing.T) {
	tests := []struct {
		name           string
		current        string
		messages       []string
		wantVersion    string
		wantRelease    bool
		wantErrorMatch string
	}{
		{
			name:        "patch",
			current:     "v1.2.3",
			messages:    []string{"fix: handle nil input"},
			wantVersion: "1.2.4",
			wantRelease: true,
		},
		{
			name:        "minor",
			current:     "v1.2.3",
			messages:    []string{"feat(api): add widget"},
			wantVersion: "1.3.0",
			wantRelease: true,
		},
		{
			name:        "breaking footer",
			current:     "v1.2.3",
			messages:    []string{"docs: remove old guide\n\nBREAKING CHANGE: old guide URL is no longer supported"},
			wantVersion: "2.0.0",
			wantRelease: true,
		},
		{
			name:        "breaking bang",
			current:     "v1.2.3",
			messages:    []string{"refactor!: replace the public API"},
			wantVersion: "2.0.0",
			wantRelease: true,
		},
		{
			name:        "major takes precedence over minor and patch",
			current:     "v1.2.3",
			messages:    []string{"fix: repair", "feat: add option", "chore!: change contract"},
			wantVersion: "2.0.0",
			wantRelease: true,
		},
		{
			name:        "minor takes precedence over patch",
			current:     "v1.2.3",
			messages:    []string{"fix: repair", "feat: add option"},
			wantVersion: "1.3.0",
			wantRelease: true,
		},
		{
			name:        "nonrelease only",
			current:     "v1.2.3",
			messages:    []string{"docs: update guide", "chore: tidy modules"},
			wantVersion: "1.2.3",
		},
		{
			name:        "empty range",
			current:     "v1.2.3",
			wantVersion: "1.2.3",
		},
		{
			name:           "invalid current version",
			current:        "1.2",
			messages:       []string{"fix: repair"},
			wantErrorMatch: "current version",
		},
		{
			name:           "version increment overflow",
			current:        "v1.2.18446744073709551615",
			messages:       []string{"fix: repair"},
			wantErrorMatch: "overflow",
		},
		{
			name:           "malformed release relevant header",
			current:        "v1.2.3",
			messages:       []string{"feat add widget"},
			wantErrorMatch: "message",
		},
		{
			name:           "malformed release relevant scope",
			current:        "v1.2.3",
			messages:       []string{"fix(api: repair"},
			wantErrorMatch: "message",
		},
		{
			name:           "case variant release type is malformed",
			current:        "v1.2.3",
			messages:       []string{"Feat: add API"},
			wantErrorMatch: "message",
		},
		{
			name:           "malformed case variant release scope",
			current:        "v1.2.3",
			messages:       []string{"Fix(api: repair"},
			wantErrorMatch: "message",
		},
		{
			name:           "invalid utf8 in release relevant message",
			current:        "v1.2.3",
			messages:       []string{"fix: repair\xff"},
			wantErrorMatch: "message",
		},
		{
			name:           "malformed breaking metadata",
			current:        "v1.2.3",
			messages:       []string{"docs: revise guide\n\nBREAKING CHANGE:"},
			wantErrorMatch: "message",
		},
		{
			name:           "malformed release footer",
			current:        "v1.2.3",
			messages:       []string{"fix: repair\n\nBREAKING CHANGE:"},
			wantErrorMatch: "message",
		},
		{
			name:        "normalizes v prefix",
			current:     "v1.2.3",
			messages:    []string{"docs: update guide"},
			wantVersion: "1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, shouldRelease, err := CalculateNextVersion(tt.current, tt.messages)
			if tt.wantErrorMatch != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrorMatch) {
					t.Fatalf("CalculateNextVersion() error = %v, want error containing %q", err, tt.wantErrorMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("CalculateNextVersion() error = %v", err)
			}
			if version != tt.wantVersion {
				t.Errorf("CalculateNextVersion() version = %q, want %q", version, tt.wantVersion)
			}
			if shouldRelease != tt.wantRelease {
				t.Errorf("CalculateNextVersion() shouldRelease = %t, want %t", shouldRelease, tt.wantRelease)
			}
		})
	}
}

func TestValidateConventionalCommit(t *testing.T) {
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
			err := ValidateConventionalCommit(tt.message)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateConventionalCommit() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
