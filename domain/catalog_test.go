package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestValidateCatalogCandidateAcceptsCanonicalFileCatalog(t *testing.T) {
	t.Parallel()

	candidate := validCatalogCandidate()
	if err := ValidateCatalogCandidate(candidate); err != nil {
		t.Fatalf("canonical catalog candidate: %v", err)
	}
}

func TestValidateCatalogCandidateRejectsInvalidCatalogMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*CatalogCandidate)
	}{
		{name: "catalog id", mutate: func(value *CatalogCandidate) { value.ID = "Kubernetes" }},
		{name: "title utf8", mutate: func(value *CatalogCandidate) { value.Title = string([]byte{'K', 0xff}) }},
		{name: "profile", mutate: func(value *CatalogCandidate) { value.ProfileID = "" }},
		{name: "revision kind", mutate: func(value *CatalogCandidate) { value.Revision.Kind = "network" }},
		{name: "revision id", mutate: func(value *CatalogCandidate) { value.Revision.ID = "" }},
		{name: "manifest digest", mutate: func(value *CatalogCandidate) { value.Revision.ManifestDigest = strings.Repeat("A", 64) }},
		{name: "unexpected file commit", mutate: func(value *CatalogCandidate) { value.Revision.CommitSHA = strings.Repeat("a", 40) }},
		{name: "missing document", mutate: func(value *CatalogCandidate) { value.Documents = nil }},
		{name: "unknown default document", mutate: func(value *CatalogCandidate) { value.DefaultDocumentKey = "missing-v1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := validCatalogCandidate()
			test.mutate(&candidate)
			if err := ValidateCatalogCandidate(candidate); err == nil {
				t.Fatal("invalid catalog metadata was accepted")
			}
		})
	}
}

func TestValidateCatalogCandidateEnforcesGitRevisionEvidence(t *testing.T) {
	t.Parallel()

	candidate := validCatalogCandidate()
	candidate.Revision.Kind = CatalogRevisionGit
	candidate.Revision.CommitSHA = strings.Repeat("a", 40)
	if err := ValidateCatalogCandidate(candidate); err != nil {
		t.Fatalf("canonical Git revision: %v", err)
	}
	candidate.Revision.CommitSHA = strings.Repeat("b", 64)
	if err := ValidateCatalogCandidate(candidate); err != nil {
		t.Fatalf("canonical Git SHA-256 revision: %v", err)
	}

	for _, commit := range []string{"", strings.Repeat("a", 39), strings.Repeat("a", 63), strings.Repeat("A", 40), strings.Repeat("z", 40)} {
		candidate.Revision.CommitSHA = commit
		if err := ValidateCatalogCandidate(candidate); err == nil {
			t.Fatalf("invalid Git commit %q was accepted", commit)
		}
	}
}

func TestValidateCatalogCandidateRejectsInvalidDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*CatalogCandidate)
	}{
		{name: "duplicate key", mutate: func(value *CatalogCandidate) {
			value.Documents = append(value.Documents, value.Documents[0])
			value.Documents[1].SourcePath = "apis/apps/v1_openapi.json"
		}},
		{name: "duplicate source path", mutate: func(value *CatalogCandidate) {
			value.Documents = append(value.Documents, value.Documents[0])
			value.Documents[1].Key = "apps-v1"
		}},
		{name: "noncanonical key", mutate: func(value *CatalogCandidate) { value.Documents[0].Key = "core/v1" }},
		{name: "absolute source", mutate: func(value *CatalogCandidate) { value.Documents[0].SourcePath = "/api/v1_openapi.json" }},
		{name: "escaping source", mutate: func(value *CatalogCandidate) { value.Documents[0].SourcePath = "../api/v1_openapi.json" }},
		{name: "backslash source", mutate: func(value *CatalogCandidate) { value.Documents[0].SourcePath = `api\v1_openapi.json` }},
		{name: "unsupported format", mutate: func(value *CatalogCandidate) { value.Documents[0].Format = "toml" }},
		{name: "empty bytes", mutate: func(value *CatalogCandidate) { value.Documents[0].Bytes = nil }},
		{name: "oversized bytes", mutate: func(value *CatalogCandidate) { value.Documents[0].Bytes = make([]byte, maxCatalogDocumentBytes+1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validCatalogCandidate()
			test.mutate(&candidate)
			if err := ValidateCatalogCandidate(candidate); err == nil {
				t.Fatal("invalid catalog document was accepted")
			}
		})
	}
}

func TestValidateCatalogCandidateBoundsDocumentCount(t *testing.T) {
	t.Parallel()

	candidate := validCatalogCandidate()
	candidate.Documents = make([]CatalogDocument, maxCatalogDocuments)
	for i := range candidate.Documents {
		candidate.Documents[i] = CatalogDocument{
			Key:        "doc-" + base36(i),
			SourcePath: "specs/doc-" + base36(i) + ".json",
			Format:     CatalogFormatJSON,
			Bytes:      []byte("{}"),
		}
	}
	candidate.DefaultDocumentKey = candidate.Documents[0].Key
	if err := ValidateCatalogCandidate(candidate); err != nil {
		t.Fatalf("document count boundary: %v", err)
	}
	candidate.Documents = append(candidate.Documents, CatalogDocument{
		Key: "over", SourcePath: "specs/over.json", Format: CatalogFormatJSON, Bytes: []byte("{}"),
	})
	if err := ValidateCatalogCandidate(candidate); err == nil {
		t.Fatal("catalog accepted a document beyond the hard count")
	}
}

func TestValidateSpecIndexRequiresSortedUniqueFacets(t *testing.T) {
	t.Parallel()

	valid := SpecIndex{Operations: []Operation{{
		Method: "GET", Path: "/api/v1/pods",
		Facets: []Facet{{Name: "group", Value: "core"}, {Name: "version", Value: "v1"}},
	}}}
	if err := ValidateSpecIndex(valid); err != nil {
		t.Fatalf("sorted facets: %v", err)
	}

	for name, facets := range map[string][]Facet{
		"unsorted":    {{Name: "version", Value: "v1"}, {Name: "group", Value: "core"}},
		"duplicate":   {{Name: "group", Value: "core"}, {Name: "group", Value: "core"}},
		"empty name":  {{Name: "", Value: "core"}},
		"empty value": {{Name: "group", Value: ""}},
	} {
		t.Run(name, func(t *testing.T) {
			index := valid
			index.Operations = append([]Operation(nil), valid.Operations...)
			index.Operations[0].Facets = facets
			if err := ValidateSpecIndex(index); err == nil {
				t.Fatal("invalid operation facets were accepted")
			}
		})
	}
}

func validCatalogCandidate() CatalogCandidate {
	manifest := sha256.Sum256([]byte("api/v1_openapi.json\x00{}"))
	return CatalogCandidate{
		ID:                 "kubernetes",
		Title:              "Kubernetes",
		DefaultDocumentKey: "core-v1",
		ProfileID:          CompatibilityProfileStrict,
		Revision: CatalogRevision{
			Kind:           CatalogRevisionFiles,
			ID:             "file-manifest-a",
			ManifestDigest: hex.EncodeToString(manifest[:]),
		},
		Documents: []CatalogDocument{{
			Key: "core-v1", SourcePath: "api/v1_openapi.json", Format: CatalogFormatJSON, Bytes: []byte("{}"),
		}},
	}
}

func base36(value int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value == 0 {
		return "0"
	}
	var reversed [16]byte
	index := len(reversed)
	for value > 0 {
		index--
		reversed[index] = alphabet[value%len(alphabet)]
		value /= len(alphabet)
	}
	return string(reversed[index:])
}
