package domain

import (
	"encoding/json"
	"testing"
)

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

func TestPinnedTrackAppliesRejectThenAcceptForSameRevision(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	rejectedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false,
	}
	rejected, changed, err := ConsiderReleaseDecision(track, rejectedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || rejected.Generation != 3 || rejected.CandidateRevisionID != "revision-next" {
		t.Fatalf("rejected decision = %#v, changed = %t", rejected, changed)
	}

	acceptedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true,
	}
	accepted, changed, err := ConsiderReleaseDecision(rejected, acceptedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || accepted.Generation != 4 || accepted.CandidateRevisionID != "revision-next" {
		t.Fatalf("accepted decision = %#v, changed = %t", accepted, changed)
	}
	if accepted.LastDecision == nil || *accepted.LastDecision != acceptedDecision {
		t.Fatalf("last decision = %#v, want %#v", accepted.LastDecision, acceptedDecision)
	}
}

func TestReleaseDecisionExactAcceptedReplayIsNoOp(t *testing.T) {
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true,
	}
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 3, CurrentRevisionID: "revision-good",
		CandidateRevisionID: "revision-next", LastDecision: &decision,
	}

	next, changed, err := ConsiderReleaseDecision(track, decision)
	if err != nil {
		t.Fatal(err)
	}
	if changed || next != track {
		t.Fatalf("exact replay changed track: next=%#v changed=%t", next, changed)
	}
}

func TestReleaseDecisionRepeatedRejectionIsNoOp(t *testing.T) {
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false,
	}
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 3, CurrentRevisionID: "revision-good",
		CandidateRevisionID: "revision-next", LastDecision: &decision,
	}

	next, changed, err := ConsiderReleaseDecision(track, decision)
	if err != nil {
		t.Fatal(err)
	}
	if changed || next != track {
		t.Fatalf("repeated rejection changed track: next=%#v changed=%t", next, changed)
	}
}

func TestConsiderReleaseRevisionUsesDecisionIdentityForPinnedReplay(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}

	rejected, err := ConsiderReleaseRevision(track, "revision-next", false)
	if err != nil {
		t.Fatal(err)
	}
	rejectedReplay, err := ConsiderReleaseRevision(rejected, "revision-next", false)
	if err != nil {
		t.Fatal(err)
	}
	if rejectedReplay != rejected {
		t.Fatalf("exact rejection replay changed track: %#v", rejectedReplay)
	}

	accepted, err := ConsiderReleaseRevision(rejected, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Generation != rejected.Generation+1 {
		t.Fatalf("reject to accept generation = %d, want %d", accepted.Generation, rejected.Generation+1)
	}
	if accepted.LastDecision == nil || !accepted.LastDecision.Accepted || accepted.LastDecision.Verdict != VerdictPass {
		t.Fatalf("reject to accept last decision = %#v, want accepted pass", accepted.LastDecision)
	}

	acceptedReplay, err := ConsiderReleaseRevision(accepted, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if acceptedReplay != accepted {
		t.Fatalf("exact acceptance replay changed track: %#v", acceptedReplay)
	}
}

func TestPinnedPromotionRequiresMatchingAcceptedDecisionEvidence(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	rejected, err := ConsiderReleaseRevision(track, "revision-next", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PromoteReleaseRevision(rejected, "revision-next"); err == nil {
		t.Fatal("rejected decision promoted its candidate")
	}

	mismatchedDecision := ReleaseDecision{
		RevisionID: "revision-other", ReviewID: "review-other",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictPass, Accepted: true,
	}
	mismatched := rejected
	mismatched.LastDecision = &mismatchedDecision
	if _, err := PromoteReleaseRevision(mismatched, "revision-next"); err == nil {
		t.Fatal("accepted decision for another revision promoted the candidate")
	}
}

func TestPinnedAcceptedPromotionSurvivesRestartAndExactReplay(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	accepted, err := ConsiderReleaseRevision(track, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	var restarted ReleaseTrack
	if err := json.Unmarshal(persisted, &restarted); err != nil {
		t.Fatal(err)
	}

	promoted, err := PromoteReleaseRevision(restarted, "revision-next")
	if err != nil {
		t.Fatalf("promote restarted accepted decision: %v", err)
	}
	if promoted.CurrentRevisionID != "revision-next" || promoted.CandidateRevisionID != "" {
		t.Fatalf("promoted track = %#v", promoted)
	}
	replayed, err := PromoteReleaseRevision(promoted, "revision-next")
	if err != nil {
		t.Fatalf("replay accepted promotion: %v", err)
	}
	if replayed != promoted {
		t.Fatalf("exact promotion replay changed track: %#v", replayed)
	}
}
