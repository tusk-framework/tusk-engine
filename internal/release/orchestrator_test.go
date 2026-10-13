package release

import "testing"

func TestPlanAutomaticRelease(t *testing.T) {
	const current = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newer = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const candidate = "v1.2.3"

	tests := []struct {
		name       string
		input      ReleasePlanInput
		wantAction ReleaseAction
		wantTag    string
	}{
		{
			name:       "new release for current main",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: current, NextTag: candidate},
			wantAction: ReleaseCreate,
			wantTag:    candidate,
		},
		{
			name:       "stale queued sha does not release",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: newer, NextTag: candidate},
			wantAction: ReleaseSkipStale,
		},
		{
			name:       "no semantic release remains a distinct image-only plan",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: current},
			wantAction: ReleaseNoVersion,
		},
		{
			name:       "same sha draft resumes before version calculation",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: newer, CandidateTag: candidate, CandidateTagSHA: current, CandidateRelease: ReleaseDraft},
			wantAction: ReleaseResume,
			wantTag:    candidate,
		},
		{
			name:       "same sha published release is idempotent",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: newer, CandidateTag: candidate, CandidateTagSHA: current, CandidateRelease: ReleasePublished},
			wantAction: ReleaseAlreadyPublished,
			wantTag:    candidate,
		},
		{
			name:       "candidate tag on another sha is conflict",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: current, CandidateTag: candidate, CandidateTagSHA: newer, CandidateRelease: ReleaseDraft},
			wantAction: ReleaseConflict,
		},
		{
			name:       "new tag must be rechecked against remote before create",
			input:      ReleasePlanInput{ValidatedSHA: current, RemoteMainSHA: current, NextTag: candidate, CandidateTag: candidate, CandidateTagSHA: newer},
			wantAction: ReleaseConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := PlanAutomaticRelease(test.input)
			if plan.Action != test.wantAction || plan.Tag != test.wantTag {
				t.Fatalf("PlanAutomaticRelease() = {%s, %q}, want {%s, %q}", plan.Action, plan.Tag, test.wantAction, test.wantTag)
			}
		})
	}
}

func TestPlanAutomaticReleaseFailsClosedOnInvalidInputs(t *testing.T) {
	tests := []ReleasePlanInput{
		{ValidatedSHA: "short", RemoteMainSHA: "short", NextTag: "v1.2.3"},
		{ValidatedSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RemoteMainSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", NextTag: "1.2.3"},
		{ValidatedSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RemoteMainSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CandidateTag: "v1.2.3"},
		{ValidatedSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RemoteMainSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CandidateTag: "v1.2.3", CandidateTagSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CandidateRelease: ReleaseUnknown},
	}
	for index, input := range tests {
		if plan := PlanAutomaticRelease(input); plan.Action != ReleaseConflict {
			t.Errorf("case %d action = %s, want conflict", index, plan.Action)
		}
	}
}
