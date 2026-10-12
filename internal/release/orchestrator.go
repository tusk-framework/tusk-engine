package release

import (
	"regexp"
)

var fullSHARegexp = regexp.MustCompile(`^[a-f0-9]{40}$`)
var stableReleaseTagRegexp = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type ReleaseState uint8

const (
	ReleaseUnknown ReleaseState = iota
	ReleaseAbsent
	ReleaseDraft
	ReleasePublished
)

type ReleaseAction string

const (
	ReleaseConflict         ReleaseAction = "conflict"
	ReleaseSkipStale        ReleaseAction = "skip-stale"
	ReleaseNoVersion        ReleaseAction = "no-version"
	ReleaseCreate           ReleaseAction = "create"
	ReleaseResume           ReleaseAction = "resume"
	ReleaseAlreadyPublished ReleaseAction = "already-published"
)

type ReleasePlanInput struct {
	ValidatedSHA     string
	RemoteMainSHA    string
	NextTag          string
	CandidateTag     string
	CandidateTagSHA  string
	CandidateRelease ReleaseState
}

type ReleasePlan struct {
	Action ReleaseAction
	Tag    string
}

// PlanAutomaticRelease decides whether a validated main commit may create or
// resume a release. Candidate-tag reconciliation precedes stale-main handling
// so an immutable tag created by an earlier attempt can be safely completed.
func PlanAutomaticRelease(input ReleasePlanInput) ReleasePlan {
	if !fullSHARegexp.MatchString(input.ValidatedSHA) || !fullSHARegexp.MatchString(input.RemoteMainSHA) {
		return ReleasePlan{Action: ReleaseConflict}
	}

	if input.CandidateTag != "" {
		if !stableReleaseTagRegexp.MatchString(input.CandidateTag) || !fullSHARegexp.MatchString(input.CandidateTagSHA) || input.CandidateRelease == ReleaseUnknown {
			return ReleasePlan{Action: ReleaseConflict}
		}
		if input.CandidateTagSHA != input.ValidatedSHA {
			return ReleasePlan{Action: ReleaseConflict}
		}
		switch input.CandidateRelease {
		case ReleasePublished:
			return ReleasePlan{Action: ReleaseAlreadyPublished, Tag: input.CandidateTag}
		case ReleaseDraft, ReleaseAbsent:
			return ReleasePlan{Action: ReleaseResume, Tag: input.CandidateTag}
		default:
			return ReleasePlan{Action: ReleaseConflict}
		}
	}
	if input.CandidateTagSHA != "" {
		return ReleasePlan{Action: ReleaseConflict}
	}

	if input.RemoteMainSHA != input.ValidatedSHA {
		return ReleasePlan{Action: ReleaseSkipStale}
	}
	if input.NextTag == "" {
		return ReleasePlan{Action: ReleaseNoVersion}
	}
	if !stableReleaseTagRegexp.MatchString(input.NextTag) {
		return ReleasePlan{Action: ReleaseConflict}
	}
	return ReleasePlan{Action: ReleaseCreate, Tag: input.NextTag}
}
