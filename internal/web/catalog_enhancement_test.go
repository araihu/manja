package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

type catalogEnhancementDescriptorReceipt struct {
	SchemaVersion      uint32 `json:"schemaVersion"`
	PublicationKey     string `json:"publicationKey"`
	PublicationBase    string `json:"publicationBase"`
	SnapshotID         string `json:"snapshotId"`
	RevisionID         string `json:"revisionId"`
	ProjectionFormat   string `json:"projectionFormat"`
	ProjectionDigest   string `json:"projectionDigest"`
	ProjectionDataBase string `json:"projectionDataBase"`
}

func TestCatalogEnhancementDescriptorRequiresPublicAnonymousImmutableSnapshot(t *testing.T) {
	for _, mount := range []string{"/", "/kubernetes"} {
		t.Run(mount, func(t *testing.T) {
			baseHandler, snapshot := catalogHandlerFixture(t, mount)
			base := baseHandler.(*CatalogHandler)
			snapshot = catalogEnhancementSnapshot(t, snapshot)
			runtime := catalog.NewRuntime(1)
			if _, err := runtime.ActivateMount(mount, "", 1, snapshot); err != nil {
				t.Fatal(err)
			}
			handler := NewCatalogHandlerWithOrganizationAndEnhancement(
				runtime,
				base.children,
				base.presentation,
				base.organization,
				CatalogEnhancementPolicy{Publications: map[string]CatalogPublicEligibility{
					mount: {CatalogID: snapshot.Directory.CatalogID, PublicationKey: "public-kubernetes", Public: true, Anonymous: true},
				}},
			)
			requestPath := mount
			if mount != "/" {
				requestPath += "/"
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s = %d body=%q", requestPath, response.Code, response.Body.String())
			}
			descriptor := decodeCatalogEnhancementDescriptor(t, response.Body.String())
			wantBase := mount
			if wantBase != "/" {
				wantBase += "/"
			}
			wantProjectionBase := wantBase + "snapshots/" + string(snapshot.ID) + "/projection-data/"
			wantDigest := strings.TrimPrefix(string(snapshot.ID), "snapshot-sha256-")
			if descriptor.SchemaVersion != 1 || descriptor.PublicationKey != "public-kubernetes" || descriptor.PublicationBase != wantBase || descriptor.SnapshotID != string(snapshot.ID) || descriptor.RevisionID != snapshot.Manifest.Identity.RevisionID || descriptor.ProjectionFormat != "projection-v2" || descriptor.ProjectionDigest != wantDigest || descriptor.ProjectionDataBase != wantProjectionBase {
				t.Fatalf("descriptor = %#v", descriptor)
			}
			if !strings.Contains(response.Body.String(), `data-manja-catalog-shell="true"`) {
				t.Fatal("eligible response lost the SSR catalog shell")
			}
		})
	}
}

func TestCatalogEnhancementDescriptorFailsClosed(t *testing.T) {
	baseHandler, valid := catalogHandlerFixture(t, "/kubernetes")
	base := baseHandler.(*CatalogHandler)
	valid = catalogEnhancementSnapshot(t, valid)

	tests := []struct {
		name   string
		policy CatalogEnhancementPolicy
		mutate func(*catalog.RuntimeSnapshot)
	}{
		{name: "no authority"},
		{name: "private", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(_ *catalog.RuntimeSnapshot) {}},
		{name: "not anonymous", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(_ *catalog.RuntimeSnapshot) {}},
		{name: "kill switch", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(_ *catalog.RuntimeSnapshot) {}},
		{name: "catalog mismatch", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(_ *catalog.RuntimeSnapshot) {}},
		{name: "manifest snapshot mismatch", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) {
			snapshot.Manifest.SnapshotID = "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("f", 64))
		}},
		{name: "identity digest mismatch", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) { snapshot.Manifest.Identity.RevisionID = "revision-changed" }},
		{name: "transport inventory mismatch", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) {
			snapshot.Manifest.Children = append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Children...)
			for index := range snapshot.Manifest.Children {
				if snapshot.Manifest.Children[index].Path == "catalog.json" {
					snapshot.Manifest.Children[index].SHA256 = strings.Repeat("f", 64)
					return
				}
			}
			t.Fatal("catalog.json identity missing from fixture")
		}},
		{name: "missing revision", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) { snapshot.Manifest.Identity.RevisionID = "" }},
		{name: "wrong projection format", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) {
			snapshot.Manifest.Identity.Versions.ProjectionFormat = "projection-v1"
		}},
		{name: "invalid source digest", policy: eligibleCatalogEnhancementPolicy(valid), mutate: func(snapshot *catalog.RuntimeSnapshot) {
			snapshot.Manifest.Identity.SourceManifestSHA256 = strings.Repeat("A", 64)
		}},
	}
	tests[1].policy.Publications["/kubernetes"] = CatalogPublicEligibility{CatalogID: valid.Directory.CatalogID, PublicationKey: "public-kubernetes", Anonymous: true}
	tests[2].policy.Publications["/kubernetes"] = CatalogPublicEligibility{CatalogID: valid.Directory.CatalogID, PublicationKey: "public-kubernetes", Public: true}
	tests[3].policy.Disabled = true
	tests[4].policy.Publications["/kubernetes"] = CatalogPublicEligibility{CatalogID: "other", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := valid
			if test.mutate != nil {
				test.mutate(&snapshot)
			}
			runtime := catalog.NewRuntime(1)
			if _, err := runtime.ActivateMount("/kubernetes", "", 1, snapshot); err != nil {
				t.Fatal(err)
			}
			handler := NewCatalogHandlerWithOrganizationAndEnhancement(runtime, base.children, base.presentation, base.organization, test.policy)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), `id="manja-local-docs-descriptor"`) {
				t.Fatalf("ineligible response emitted descriptor: %s", response.Body.String())
			}
			projectionPath := "/kubernetes/snapshots/" + string(snapshot.ID) + "/projection-data/" + snapshot.Directory.Documents[1].Operations[0].DetailChild
			projection := httptest.NewRecorder()
			handler.ServeHTTP(projection, httptest.NewRequest(http.MethodGet, projectionPath, nil))
			if projection.Code != http.StatusOK {
				t.Fatalf("OC-04A transport under ineligible policy = %d body=%q", projection.Code, projection.Body.String())
			}
		})
	}
}

func eligibleCatalogEnhancementPolicy(snapshot catalog.RuntimeSnapshot) CatalogEnhancementPolicy {
	return CatalogEnhancementPolicy{Publications: map[string]CatalogPublicEligibility{
		"/kubernetes": {CatalogID: snapshot.Directory.CatalogID, PublicationKey: "public-kubernetes", Public: true, Anonymous: true},
	}}
}

func catalogEnhancementSnapshot(t *testing.T, snapshot catalog.RuntimeSnapshot) catalog.RuntimeSnapshot {
	t.Helper()
	identity := catalog.SnapshotIdentityV1{
		SchemaVersion:        1,
		CatalogID:            snapshot.Directory.CatalogID,
		CatalogTitle:         snapshot.Directory.Title,
		RevisionID:           "revision-immutable-1",
		SourceManifestSHA256: strings.Repeat("a", 64),
		Versions:             catalog.CompilerVersions{ProjectionFormat: "projection-v2"},
		Children:             append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Children...),
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	snapshot.ID = catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	snapshot.Manifest = catalog.ManifestV1{SchemaVersion: 1, SnapshotID: snapshot.ID, Identity: identity, Children: append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Children...)}
	return snapshot
}

func decodeCatalogEnhancementDescriptor(t *testing.T, body string) catalogEnhancementDescriptorReceipt {
	t.Helper()
	const start = `<script id="manja-local-docs-descriptor" type="application/json">`
	startIndex := strings.Index(body, start)
	if startIndex < 0 {
		t.Fatalf("descriptor script missing from HTML")
	}
	startIndex += len(start)
	endIndex := strings.Index(body[startIndex:], "</script>")
	if endIndex < 0 {
		t.Fatalf("descriptor script is not closed")
	}
	var descriptor catalogEnhancementDescriptorReceipt
	if err := json.Unmarshal([]byte(body[startIndex:startIndex+endIndex]), &descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	return descriptor
}
