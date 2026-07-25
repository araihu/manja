package domain

import "testing"

func TestFollowingTrackAdvancesOnlyAcceptedRevision(t *testing.T) {
	track := ReleaseTrack{
		ID:                "v1",
		ContractID:        "payments",
		Mode:              ReleaseModeFollowing,
		Generation:        7,
		CurrentRevisionID: "revision-good",
	}

	rejected, err := ConsiderReleaseRevision(track, "revision-broken", false)
	if err != nil {
		t.Fatalf("consider rejected revision: %v", err)
	}
	if rejected.CurrentRevisionID != "revision-good" {
		t.Fatalf("rejected revision replaced last known good with %q", rejected.CurrentRevisionID)
	}
	if rejected.CandidateRevisionID != "revision-broken" {
		t.Fatalf("rejected candidate = %q, want revision-broken", rejected.CandidateRevisionID)
	}

	accepted, err := ConsiderReleaseRevision(track, "revision-next", true)
	if err != nil {
		t.Fatalf("consider accepted revision: %v", err)
	}
	if accepted.CurrentRevisionID != "revision-next" {
		t.Fatalf("accepted revision = %q, want revision-next", accepted.CurrentRevisionID)
	}
	if accepted.CandidateRevisionID != "" {
		t.Fatalf("accepted following track retained candidate %q", accepted.CandidateRevisionID)
	}
	if accepted.Generation != 8 {
		t.Fatalf("accepted generation = %d, want 8", accepted.Generation)
	}
}

func TestPinnedTrackRequiresExplicitPromotion(t *testing.T) {
	track := ReleaseTrack{
		ID:                "stable",
		ContractID:        "payments",
		Mode:              ReleaseModePinned,
		Generation:        2,
		CurrentRevisionID: "revision-good",
	}

	candidate, err := ConsiderReleaseRevision(track, "revision-next", true)
	if err != nil {
		t.Fatalf("consider pinned candidate: %v", err)
	}
	if candidate.CurrentRevisionID != "revision-good" || candidate.CandidateRevisionID != "revision-next" {
		t.Fatalf("pinned candidate changed public state: %#v", candidate)
	}

	promoted, err := PromoteReleaseRevision(candidate, "revision-next")
	if err != nil {
		t.Fatalf("promote candidate: %v", err)
	}
	if promoted.CurrentRevisionID != "revision-next" || promoted.CandidateRevisionID != "" {
		t.Fatalf("promotion did not advance candidate: %#v", promoted)
	}
}
