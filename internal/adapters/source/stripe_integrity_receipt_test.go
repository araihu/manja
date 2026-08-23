package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var approvedStripeIntegrityReceipt = gitSourceProvenanceReceipt{
	SchemaVersion:   2,
	CatalogID:       "stripe",
	CloneRepository: "https://github.com/stripe/openapi.git",
	ProvenanceURL:   "https://github.com/stripe/openapi/tree/d70de345383dd818a0ce831f4e20d375c5a90cec",
	ObjectFormat:    gitObjectFormatSHA1,
	SourceRoot:      ".",
	CommitObjectID:  "d70de345383dd818a0ce831f4e20d375c5a90cec",
	TreeObjectID:    "a7e155600c10dcfab91a94070b0e954419255862",
	Artifacts: []gitArtifactEvidence{
		{
			Path:        "openapi/spec3.json",
			Mode:        "100644",
			Size:        3840021,
			GitObjectID: "058edc82a247c71f05b94dfa6b9cef0a794a1358",
			SHA256:      "8b608cba7129d121f12358a7092574e176833fe8cb4c9fcead178c71c545f870",
		},
	},
}

func TestCommittedStripeIntegrityReceiptMatchesApprovedEvidence(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", "..", "renderer", "testdata", "kubernetes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := requireApprovedStripeIntegrityReceipt(root, "stripe-openapi.integrity.json"); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedStripeIntegrityReceiptRejectsControlledDrift(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(*gitSourceProvenanceReceipt)
	}{
		{name: "catalog ID", mutate: func(got *gitSourceProvenanceReceipt) { got.CatalogID = "stripe-fork" }},
		{name: "clone repository", mutate: func(got *gitSourceProvenanceReceipt) { got.CloneRepository = "https://github.com/stripe/openapi" }},
		{name: "provenance URL", mutate: func(got *gitSourceProvenanceReceipt) { got.ProvenanceURL += "?changed=1" }},
		{name: "object format", mutate: func(got *gitSourceProvenanceReceipt) { got.ObjectFormat = gitObjectFormatSHA256 }},
		{name: "source root", mutate: func(got *gitSourceProvenanceReceipt) { got.SourceRoot = "openapi" }},
		{name: "commit object ID", mutate: func(got *gitSourceProvenanceReceipt) { got.CommitObjectID = strings.Repeat("a", 40) }},
		{name: "tree object ID", mutate: func(got *gitSourceProvenanceReceipt) { got.TreeObjectID = strings.Repeat("b", 40) }},
		{name: "artifact path", mutate: func(got *gitSourceProvenanceReceipt) { got.Artifacts[0].Path = "openapi/spec4.json" }},
		{name: "artifact mode", mutate: func(got *gitSourceProvenanceReceipt) { got.Artifacts[0].Mode = "100755" }},
		{name: "artifact size", mutate: func(got *gitSourceProvenanceReceipt) { got.Artifacts[0].Size++ }},
		{name: "artifact Git object ID", mutate: func(got *gitSourceProvenanceReceipt) { got.Artifacts[0].GitObjectID = strings.Repeat("c", 40) }},
		{name: "artifact SHA-256", mutate: func(got *gitSourceProvenanceReceipt) { got.Artifacts[0].SHA256 = strings.Repeat("d", 64) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			receipt := cloneStripeIntegrityReceipt(approvedStripeIntegrityReceipt)
			mutation.mutate(&receipt)
			root := t.TempDir()
			contents, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "receipt.json"), contents, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := requireApprovedStripeIntegrityReceipt(root, "receipt.json"); err == nil {
				t.Fatal("controlled Stripe integrity receipt drift was accepted")
			}
		})
	}

	t.Run("license evidence is outside runtime receipt", func(t *testing.T) {
		root := t.TempDir()
		contents, err := json.Marshal(approvedStripeIntegrityReceipt)
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents[:len(contents)-1], []byte(`,"license":{"spdx":"MIT"}}`)...)
		if err := os.WriteFile(filepath.Join(root, "receipt.json"), contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := requireApprovedStripeIntegrityReceipt(root, "receipt.json"); err == nil {
			t.Fatal("runtime receipt accepted legal evidence outside its strict schema")
		}
	})
}

func requireApprovedStripeIntegrityReceipt(root, filename string) error {
	receipt, err := loadGitSourceProvenanceReceipt(root, filename, true)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(receipt, approvedStripeIntegrityReceipt) {
		return fmt.Errorf("Stripe integrity receipt = %#v, want %#v", receipt, approvedStripeIntegrityReceipt)
	}
	return nil
}

func cloneStripeIntegrityReceipt(receipt gitSourceProvenanceReceipt) gitSourceProvenanceReceipt {
	cloned := receipt
	cloned.Artifacts = append([]gitArtifactEvidence(nil), receipt.Artifacts...)
	return cloned
}
