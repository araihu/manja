package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestReleaseTrackRemainsComparableForPublicCallers(t *testing.T) {
	track := ReleaseTrack{}
	if track != track {
		t.Fatal("release track did not equal itself")
	}
}

func TestCloneReleaseTrackIsolatesDecisionEvidence(t *testing.T) {
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 3, CurrentRevisionID: "revision-good",
		CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
	}

	cloned := CloneReleaseTrack(track)
	cloned.LastDecision.Accepted = true
	cloned.LastDecision.Verdict = VerdictPass

	if track.LastDecision.Accepted || track.LastDecision.Verdict != VerdictFail {
		t.Fatalf("clone mutated original decision evidence: %#v", track.LastDecision)
	}
}

func TestValidateReleaseTrackRejectsMalformedDecisionEvidence(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, EvaluatedAt: evaluatedAt,
	}
	zeroTimeDecision := decision
	zeroTimeDecision.EvaluatedAt = time.Time{}
	tests := []struct {
		name  string
		track ReleaseTrack
	}{
		{
			name: "candidate without decision evidence",
			track: ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
				CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-next"},
		},
		{
			name: "decision for a different candidate",
			track: ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
				CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-other", LastDecision: &decision},
		},
		{
			name: "rejected decision stripped of its candidate",
			track: ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
				CurrentRevisionID: "revision-good", LastDecision: &decision},
		},
		{
			name: "decision without chronology",
			track: ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
				CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-next", LastDecision: &zeroTimeDecision},
		},
		{
			name: "decision without generation",
			track: ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
				CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-next", LastDecision: &decision},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateReleaseTrack(test.track); err == nil {
				t.Fatal("malformed release track was accepted")
			}
		})
	}
}

func TestValidateReleaseTrackRejectsStrandedAcceptedFollowingDecision(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-next",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: evaluatedAt,
	}
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModeFollowing,
		Generation: 4, CurrentRevisionID: "revision-good",
		CandidateRevisionID: "revision-next", LastDecision: &decision,
	}
	if err := ValidateReleaseTrack(track); err == nil {
		t.Fatal("following track retained an accepted candidate without advancing current")
	}
}

func TestValidateReleaseTrackTransitionRejectsStrippedOrSupersededEvidence(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	currentDecision := ReleaseDecision{
		RevisionID: "revision-current", ReviewID: "review-current",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: evaluatedAt,
	}
	current := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 4, CurrentRevisionID: currentDecision.RevisionID, LastDecision: &currentDecision,
	}

	stripped := CloneReleaseTrack(current)
	stripped.LastDecision = nil
	if err := ValidateReleaseTrackTransition(current, stripped); err == nil {
		t.Fatal("stripped decision evidence was accepted as a transition")
	}
	for _, timestamp := range []time.Time{evaluatedAt.Add(-time.Minute), evaluatedAt} {
		supersededDecision := currentDecision
		supersededDecision.ReviewID = "review-superseded"
		supersededDecision.ReviewDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		supersededDecision.EvaluatedAt = timestamp
		superseded := CloneReleaseTrack(current)
		superseded.LastDecision = &supersededDecision
		if err := ValidateReleaseTrackTransition(current, superseded); err == nil {
			t.Fatalf("superseded decision at %s was accepted", timestamp)
		}
	}

	promoted := CloneReleaseTrack(current)
	if err := ValidateReleaseTrackTransition(current, promoted); err != nil {
		t.Fatalf("exact decision evidence transition: %v", err)
	}
}

func TestValidateReleaseTrackTransitionAllowsOnlyExactDecisionOrPromotionState(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	rejectedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, EvaluatedAt: evaluatedAt,
	}
	rejected := ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "main", Mode: ReleaseModePinned,
		Generation: 5, CurrentRevisionID: "revision-good",
		CandidateRevisionID: rejectedDecision.RevisionID, LastDecision: &rejectedDecision,
	}

	if err := ValidateReleaseTrackTransition(rejected, CloneReleaseTrack(rejected)); err != nil {
		t.Fatalf("exact persisted no-op: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*ReleaseTrack)
	}{
		{name: "generation regression", mutate: func(next *ReleaseTrack) { next.Generation = 0 }},
		{name: "generation-only change", mutate: func(next *ReleaseTrack) { next.Generation++ }},
		{name: "rejected candidate copied to current", mutate: func(next *ReleaseTrack) { next.CurrentRevisionID = next.CandidateRevisionID }},
		{name: "bound ref rewrite", mutate: func(next *ReleaseTrack) { next.BoundRef = "release" }},
		{name: "mode rewrite", mutate: func(next *ReleaseTrack) { next.Mode = ReleaseModeFollowing }},
	} {
		t.Run(test.name, func(t *testing.T) {
			next := CloneReleaseTrack(rejected)
			test.mutate(&next)
			if err := ValidateReleaseTrackTransition(rejected, next); err == nil {
				t.Fatal("illegal persisted mutation was accepted")
			}
		})
	}

	acceptedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: evaluatedAt.Add(time.Minute),
	}
	accepted, changed, err := ConsiderReleaseDecision(rejected, acceptedDecision)
	if err != nil || !changed {
		t.Fatalf("derive newer accepted decision: changed=%t err=%v", changed, err)
	}
	if err := ValidateReleaseTrackTransition(rejected, accepted); err != nil {
		t.Fatalf("newer authoritative decision: %v", err)
	}
	wrongGeneration := CloneReleaseTrack(accepted)
	wrongGeneration.Generation++
	if err := ValidateReleaseTrackTransition(rejected, wrongGeneration); err == nil {
		t.Fatal("new decision with non-unit generation change was accepted")
	}

	promoted, err := PromoteReleaseRevision(accepted, acceptedDecision.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReleaseTrackTransition(accepted, promoted); err != nil {
		t.Fatalf("pinned promotion: %v", err)
	}
	wrongPromotion := CloneReleaseTrack(promoted)
	wrongPromotion.Generation++
	if err := ValidateReleaseTrackTransition(accepted, wrongPromotion); err == nil {
		t.Fatal("promotion with non-unit generation change was accepted")
	}
}

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
	rejectedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	rejectedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false, EvaluatedAt: rejectedAt,
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
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: rejectedAt.Add(time.Minute),
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
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
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
	if changed || !reflect.DeepEqual(next, track) {
		t.Fatalf("exact replay changed track: next=%#v changed=%t", next, changed)
	}
}

func TestReleaseDecisionRepeatedRejectionIsNoOp(t *testing.T) {
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
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
	if changed || !reflect.DeepEqual(next, track) {
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
	if !reflect.DeepEqual(rejectedReplay, rejected) {
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
	if !reflect.DeepEqual(acceptedReplay, accepted) {
		t.Fatalf("exact acceptance replay changed track: %#v", acceptedReplay)
	}
}

func TestConsiderReleaseRevisionFailsClosedAfterLegacyAcceptance(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	accepted, err := ConsiderReleaseRevision(track, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ConsiderReleaseRevision(accepted, "revision-next", false); err == nil {
		t.Fatal("legacy acceptance was revoked without authoritative identity or time")
	}
	exactReplay, err := ConsiderReleaseRevision(accepted, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(exactReplay, accepted) {
		t.Fatalf("exact accepted replay changed track: got=%#v want=%#v", exactReplay, accepted)
	}
	if _, err := ConsiderReleaseRevision(accepted, "revision-other", false); err == nil {
		t.Fatal("legacy acceptance was superseded by another unauthenticated candidate")
	}

	persisted, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	var restarted ReleaseTrack
	if err := json.Unmarshal(persisted, &restarted); err != nil {
		t.Fatal(err)
	}
	if _, err := ConsiderReleaseRevision(restarted, "revision-next", false); err == nil {
		t.Fatal("restarted legacy acceptance was revoked")
	}
	promoted, err := PromoteReleaseRevision(restarted, "revision-next")
	if err != nil {
		t.Fatalf("promote retained legacy acceptance: %v", err)
	}
	if _, err := ConsiderReleaseRevision(promoted, "revision-next", false); err == nil {
		t.Fatal("promoted legacy acceptance was revoked")
	}
}

func TestConsiderReleaseRevisionCannotReacceptAfterDeniedLegacyRevocation(t *testing.T) {
	track := ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned}
	accepted, err := ConsiderReleaseRevision(track, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConsiderReleaseRevision(accepted, "revision-next", false); err == nil {
		t.Fatal("legacy revocation unexpectedly succeeded")
	}
	replayed, err := ConsiderReleaseRevision(accepted, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed, accepted) {
		t.Fatalf("denied revoke then accept changed track: got=%#v want=%#v", replayed, accepted)
	}
}

func TestReleaseTransitionsRejectGenerationOverflow(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	decision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-next",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: evaluatedAt,
	}
	maxGeneration := ^uint64(0)

	t.Run("decision", func(t *testing.T) {
		track := ReleaseTrack{
			ID: "stable", ContractID: "payments", Mode: ReleaseModeFollowing,
			Generation: maxGeneration, CurrentRevisionID: "revision-good",
		}
		if _, changed, err := ConsiderReleaseDecision(track, decision); err == nil || changed {
			t.Fatalf("overflowing decision changed=%t err=%v", changed, err)
		}
	})

	t.Run("promotion", func(t *testing.T) {
		track := ReleaseTrack{
			ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
			Generation: maxGeneration, CurrentRevisionID: "revision-good",
			CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
		}
		if _, err := PromoteReleaseRevision(track, decision.RevisionID); err == nil {
			t.Fatal("overflowing promotion succeeded")
		}
	})
}

func TestReleaseSecurityIdentitiesRejectPadding(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	validDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-next",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, EvaluatedAt: evaluatedAt,
	}
	validTrack := ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
		Mode: ReleaseModePinned, Generation: 1,
		CandidateRevisionID: validDecision.RevisionID, LastDecision: &validDecision,
	}

	for name, mutate := range map[string]func(*ReleaseTrack){
		"track id": func(track *ReleaseTrack) { track.ID = " stable" },
		"contract id": func(track *ReleaseTrack) {
			track.ContractID = "payments "
		},
		"bound ref": func(track *ReleaseTrack) {
			track.BoundRef = " refs/heads/main"
		},
		"candidate revision": func(track *ReleaseTrack) {
			track.CandidateRevisionID = "revision-next "
			track.LastDecision.RevisionID = track.CandidateRevisionID
		},
		"decision revision": func(track *ReleaseTrack) {
			track.CandidateRevisionID = " revision-next"
			track.LastDecision.RevisionID = track.CandidateRevisionID
		},
		"decision review": func(track *ReleaseTrack) {
			track.LastDecision.ReviewID = "review-next "
		},
	} {
		t.Run(name, func(t *testing.T) {
			track := CloneReleaseTrack(validTrack)
			mutate(&track)
			if err := ValidateReleaseTrack(track); err == nil {
				t.Fatalf("padded release identity was accepted: %#v", track)
			}
		})
	}

	baseline := ReleaseTrack{ID: "stable", ContractID: "payments", Mode: ReleaseModePinned}
	if _, err := ConsiderReleaseRevision(baseline, " revision-next", false); err == nil {
		t.Fatal("legacy helper normalized a padded revision id")
	}

	accepted := validDecision
	accepted.Accepted = true
	accepted.Verdict = VerdictPass
	pinned := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 1, CandidateRevisionID: accepted.RevisionID, LastDecision: &accepted,
	}
	if _, err := PromoteReleaseRevision(pinned, "revision-next "); err == nil {
		t.Fatal("promotion normalized a padded revision id")
	}
}

func TestValidateReleaseAuthorizationBindsOneReviewToOneTrack(t *testing.T) {
	valid := ReleaseAuthorization{
		ContractID: "payments", TrackID: "stable",
		ReviewID: "review-next", SyncRecordID: "sync-next",
		BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
		SourceID: "payments-git", BoundRef: "refs/heads/main",
		PublicPath:   "/payments/stable",
		PolicyDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	if err := ValidateReleaseAuthorization(valid); err != nil {
		t.Fatalf("valid release authorization: %v", err)
	}

	for name, mutate := range map[string]func(*ReleaseAuthorization){
		"contract":     func(value *ReleaseAuthorization) { value.ContractID = "payments " },
		"track":        func(value *ReleaseAuthorization) { value.TrackID = " stable" },
		"review":       func(value *ReleaseAuthorization) { value.ReviewID = "review-next " },
		"sync":         func(value *ReleaseAuthorization) { value.SyncRecordID = " sync-next" },
		"baseline":     func(value *ReleaseAuthorization) { value.BaselineRevisionID = "revision-good " },
		"candidate":    func(value *ReleaseAuthorization) { value.CandidateRevisionID = " revision-next" },
		"source":       func(value *ReleaseAuthorization) { value.SourceID = "payments-git " },
		"ref":          func(value *ReleaseAuthorization) { value.BoundRef = " refs/heads/main" },
		"path":         func(value *ReleaseAuthorization) { value.PublicPath = "payments/stable" },
		"path padding": func(value *ReleaseAuthorization) { value.PublicPath = "/payments/stable " },
		"policy":       func(value *ReleaseAuthorization) { value.PolicyDigest = "not-a-digest" },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := ValidateReleaseAuthorization(value); err == nil {
				t.Fatalf("invalid authorization was accepted: %#v", value)
			}
		})
	}
}

func TestStaleAcceptedDecisionReplayCannotOverrideNewerRejection(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	acceptedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	acceptedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: acceptedAt,
	}
	accepted, changed, err := ConsiderReleaseDecision(track, acceptedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("initial acceptance was a no-op")
	}
	rejectedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictFail, Accepted: false, EvaluatedAt: acceptedAt.Add(time.Minute),
	}
	rejected, changed, err := ConsiderReleaseDecision(accepted, rejectedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("newer rejection was a no-op")
	}

	replayed, changed, err := ConsiderReleaseDecision(rejected, acceptedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !reflect.DeepEqual(replayed, rejected) {
		t.Fatalf("stale accepted replay changed track: replayed=%#v rejected=%#v changed=%t", replayed, rejected, changed)
	}
	if _, err := PromoteReleaseRevision(replayed, "revision-next"); err == nil {
		t.Fatal("stale accepted replay restored promotion authorization")
	}
}

func TestReleaseDecisionRejectsUnseenOlderDecision(t *testing.T) {
	older := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	currentDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-newer",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false, EvaluatedAt: newer,
	}
	track, changed, err := ConsiderReleaseDecision(ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}, currentDecision)
	if err != nil || !changed {
		t.Fatalf("apply newer decision: changed=%t err=%v", changed, err)
	}
	staleUnseen := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-older-unseen",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: older,
	}

	replayed, changed, err := ConsiderReleaseDecision(track, staleUnseen)
	if err != nil {
		t.Fatalf("ignore unseen older decision: %v", err)
	}
	if changed || !reflect.DeepEqual(replayed, track) {
		t.Fatalf("unseen older decision changed track: replayed=%#v track=%#v changed=%t", replayed, track, changed)
	}
}

func TestReleaseDecisionRejectsEqualTimeDifferentIdentity(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	currentDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-first",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false, EvaluatedAt: evaluatedAt,
	}
	track, changed, err := ConsiderReleaseDecision(ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}, currentDecision)
	if err != nil || !changed {
		t.Fatalf("apply first decision: changed=%t err=%v", changed, err)
	}
	conflicting := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-conflicting",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: evaluatedAt,
	}

	if _, _, err := ConsiderReleaseDecision(track, conflicting); err == nil {
		t.Fatal("different decision at the same evaluation time was accepted")
	}
}

func TestReleaseDecisionAllowsNewerAcceptanceAfterRejection(t *testing.T) {
	rejectedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	rejectedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      VerdictFail, Accepted: false, EvaluatedAt: rejectedAt,
	}
	rejected, changed, err := ConsiderReleaseDecision(ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}, rejectedDecision)
	if err != nil || !changed {
		t.Fatalf("apply rejection: changed=%t err=%v", changed, err)
	}
	acceptedDecision := ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      VerdictPass, Accepted: true, EvaluatedAt: rejectedAt.Add(time.Minute),
	}

	accepted, changed, err := ConsiderReleaseDecision(rejected, acceptedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || accepted.LastDecision == nil || !reflect.DeepEqual(*accepted.LastDecision, acceptedDecision) {
		t.Fatalf("newer acceptance not applied: accepted=%#v changed=%t", accepted, changed)
	}
}

func TestReleaseDecisionStateRemainsConstantAcrossLongSequence(t *testing.T) {
	track := ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	startedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	firstSize := 0
	for index := 0; index < 512; index++ {
		decision := ReleaseDecision{
			RevisionID:   "revision-next",
			ReviewID:     fmt.Sprintf("review-%04d", index),
			ReviewDigest: fmt.Sprintf("%064x", index+1),
			Verdict:      VerdictFail,
			Accepted:     false,
			EvaluatedAt:  startedAt.Add(time.Duration(index) * time.Second),
		}
		next, changed, err := ConsiderReleaseDecision(track, decision)
		if err != nil {
			t.Fatalf("decision %d: %v", index, err)
		}
		if !changed {
			t.Fatalf("decision %d was a no-op", index)
		}
		track = next
		encoded, err := json.Marshal(track)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			firstSize = len(encoded)
		}
		if bytes.Contains(encoded, []byte("decisionHistory")) {
			t.Fatalf("decision %d retained append-only replay history", index)
		}
		if len(encoded) > firstSize+16 {
			t.Fatalf("decision state grew from %d to %d bytes at decision %d", firstSize, len(encoded), index)
		}
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
	if !reflect.DeepEqual(replayed, promoted) {
		t.Fatalf("exact promotion replay changed track: %#v", replayed)
	}
}
