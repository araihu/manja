package domain

import (
	"testing"
	"time"
)

func TestDomainRejectsControlCharactersInSecurityIdentitiesAndPaths(t *testing.T) {
	control := "safe\x00collision"
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	t.Run("release authorization fields", func(t *testing.T) {
		base := ReleaseAuthorization{
			ContractID: "payments", TrackID: "stable", ReviewID: "review",
			SyncRecordID: "sync", BaselineRevisionID: "baseline", CandidateRevisionID: "candidate",
			SourceID: "source", BoundRef: "refs/heads/main", PublicPath: "/payments/stable",
			PolicyDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		mutations := map[string]func(*ReleaseAuthorization){
			"contract":  func(value *ReleaseAuthorization) { value.ContractID = control },
			"track":     func(value *ReleaseAuthorization) { value.TrackID = control },
			"review":    func(value *ReleaseAuthorization) { value.ReviewID = control },
			"sync":      func(value *ReleaseAuthorization) { value.SyncRecordID = control },
			"baseline":  func(value *ReleaseAuthorization) { value.BaselineRevisionID = control },
			"candidate": func(value *ReleaseAuthorization) { value.CandidateRevisionID = control },
			"source":    func(value *ReleaseAuthorization) { value.SourceID = control },
			"ref":       func(value *ReleaseAuthorization) { value.BoundRef = control },
			"path":      func(value *ReleaseAuthorization) { value.PublicPath = "/payments/\x00stable" },
		}
		for name, mutate := range mutations {
			t.Run(name, func(t *testing.T) {
				value := base
				mutate(&value)
				if err := ValidateReleaseAuthorization(value); err == nil {
					t.Fatal("control-bearing authorization was accepted")
				}
			})
		}
	})

	t.Run("release track and decision", func(t *testing.T) {
		decision := ReleaseDecision{
			RevisionID: control, ReviewID: "review",
			ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Verdict:      VerdictFail, EvaluatedAt: evaluatedAt,
		}
		if err := validateReleaseDecision(decision); err == nil {
			t.Fatal("control-bearing decision revision was accepted")
		}
		decision.RevisionID = "revision"
		decision.ReviewID = control
		if err := validateReleaseDecision(decision); err == nil {
			t.Fatal("control-bearing decision review was accepted")
		}
		if err := ValidateReleaseTrack(ReleaseTrack{
			ID: "stable", ContractID: "payments", BoundRef: control, Mode: ReleaseModePinned,
		}); err == nil {
			t.Fatal("control-bearing release ref was accepted")
		}
	})

	t.Run("snapshot identities", func(t *testing.T) {
		snapshot := NewContractSnapshot(control, "revision", []byte("spec"), SpecIndex{})
		if err := ValidateContractSnapshot(snapshot); err == nil {
			t.Fatal("control-bearing snapshot contract was accepted")
		}
		snapshot = NewContractSnapshot("payments", control, []byte("spec"), SpecIndex{})
		if err := ValidateContractSnapshot(snapshot); err == nil {
			t.Fatal("control-bearing snapshot revision was accepted")
		}
		if err := validateSnapshotRef("candidate", SnapshotRef{
			RevisionID:     control,
			SpecDigest:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ContractDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}); err == nil {
			t.Fatal("control-bearing review snapshot reference was accepted")
		}
	})

	t.Run("policy and engine identities", func(t *testing.T) {
		if _, err := MergePolicy(PolicyLayer{Name: control, Source: PolicySourceRepository}); err == nil {
			t.Fatal("control-bearing policy layer name was accepted")
		}
		baseline := NewContractSnapshot("payments", "baseline", []byte("baseline"), SpecIndex{})
		candidate := NewContractSnapshot("payments", "candidate", []byte("candidate"), SpecIndex{})
		policy, err := MergePolicy(PolicyLayer{Name: "repository", Source: PolicySourceRepository})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := EvaluateReview(ReviewRequest{
			ContractID: "payments", Target: baseline, Candidate: candidate,
			Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: control,
		}); err == nil {
			t.Fatal("control-bearing review engine identity was accepted")
		}
	})
}
