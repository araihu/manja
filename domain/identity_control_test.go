package domain

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestDomainRejectsInvalidUTF8InSecurityIdentitiesAndPaths(t *testing.T) {
	invalid := string([]byte("payments-\xff"))
	if utf8.ValidString(invalid) {
		t.Fatal("test identity unexpectedly contains valid UTF-8")
	}
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	base := ReleaseAuthorization{
		ContractID: "payments", TrackID: "stable", ReviewID: "review",
		SyncRecordID: "sync", BaselineRevisionID: "baseline", CandidateRevisionID: "candidate",
		SourceID: "source", BoundRef: "refs/heads/main", PublicPath: "/payments/stable",
		PolicyDigest: strings.Repeat("a", 64),
	}
	for name, mutate := range map[string]func(*ReleaseAuthorization){
		"contract":  func(value *ReleaseAuthorization) { value.ContractID = invalid },
		"track":     func(value *ReleaseAuthorization) { value.TrackID = invalid },
		"review":    func(value *ReleaseAuthorization) { value.ReviewID = invalid },
		"sync":      func(value *ReleaseAuthorization) { value.SyncRecordID = invalid },
		"baseline":  func(value *ReleaseAuthorization) { value.BaselineRevisionID = invalid },
		"candidate": func(value *ReleaseAuthorization) { value.CandidateRevisionID = invalid },
		"source":    func(value *ReleaseAuthorization) { value.SourceID = invalid },
		"ref":       func(value *ReleaseAuthorization) { value.BoundRef = invalid },
		"path":      func(value *ReleaseAuthorization) { value.PublicPath = "/" + invalid },
	} {
		t.Run("authorization/"+name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := ValidateReleaseAuthorization(value); err == nil {
				t.Fatal("invalid UTF-8 authorization identity was accepted")
			}
		})
	}

	t.Run("track", func(t *testing.T) {
		if err := ValidateReleaseTrack(ReleaseTrack{
			ID: invalid, ContractID: "payments", Mode: ReleaseModePinned,
		}); err == nil {
			t.Fatal("invalid UTF-8 release track identity was accepted")
		}
	})
	t.Run("snapshot", func(t *testing.T) {
		snapshot := NewContractSnapshot(invalid, "revision", []byte("spec"), SpecIndex{})
		if err := ValidateContractSnapshot(snapshot); err == nil {
			t.Fatal("invalid UTF-8 snapshot identity was accepted")
		}
	})
	t.Run("policy", func(t *testing.T) {
		if _, err := MergePolicy(PolicyLayer{Name: invalid, Source: PolicySourceRepository}); err == nil {
			t.Fatal("invalid UTF-8 policy identity was accepted")
		}
	})
	t.Run("review engine", func(t *testing.T) {
		baseline := NewContractSnapshot("payments", "baseline", []byte("baseline"), SpecIndex{})
		candidate := NewContractSnapshot("payments", "candidate", []byte("candidate"), SpecIndex{})
		policy, err := MergePolicy(PolicyLayer{Name: "repository", Source: PolicySourceRepository})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := EvaluateReview(ReviewRequest{
			ContractID: "payments", Target: baseline, Candidate: candidate,
			Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: invalid,
		}); err == nil {
			t.Fatal("invalid UTF-8 review engine identity was accepted")
		}
	})
}
