package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
)

func TestFileStoreCurrentMarkerRequiresCurrentOperationalState(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh install remains empty", func(t *testing.T) {
		root := t.TempDir()
		_, err := NewFileStore(root).ContractRevision(ctx, "payments", "revision")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("fresh install read error = %v, want not-exist", err)
		}
		if _, err := os.Stat(filepath.Join(root, "operational", "schema.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fresh read created schema marker: %v", err)
		}
	})

	for _, test := range []struct {
		name       string
		withLegacy bool
	}{
		{name: "missing state"},
		{name: "missing state with flat legacy data", withLegacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewFileStore(root)
			if err := store.SaveRevision(ctx, core.ContractRevision{
				ID: "current", ContractID: "payments",
			}); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(root, "operational", "state.json")
			markerPath := filepath.Join(root, "operational", "schema.json")
			markerBefore, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(statePath); err != nil {
				t.Fatal(err)
			}
			var legacyPath string
			var legacyBefore []byte
			if test.withLegacy {
				legacyPath = filepath.Join(root, "revisions", "legacy-resurrected.json")
				legacyBefore, err = json.Marshal(core.ContractRevision{ID: "legacy-resurrected"})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(legacyPath, legacyBefore, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := NewFileStore(root).loadOperationalState(ctx); err == nil {
				t.Fatal("current marker plus missing state was not treated as an integrity error")
			}
			if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed restart recreated operational state: %v", err)
			}
			markerAfter, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(markerAfter, markerBefore) {
				t.Fatal("failed restart changed the durable schema marker")
			}
			if test.withLegacy {
				legacyAfter, err := os.ReadFile(legacyPath)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(legacyAfter, legacyBefore) {
					t.Fatal("failed restart rewrote flat legacy evidence")
				}
			}
		})
	}
}

func TestFileStoreRestartRejectsOutstandingDecisionAgainstWrongCurrentBaseline(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		name := "rejected"
		if accepted {
			name = "accepted pinned"
		}
		t.Run(name, func(t *testing.T) {
			testFileStoreRestartRejectsOutstandingDecisionAgainstWrongCurrentBaseline(t, accepted)
		})
	}
}

func testFileStoreRestartRejectsOutstandingDecisionAgainstWrongCurrentBaseline(
	t *testing.T,
	accepted bool,
) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, accepted)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRevision(ctx, core.ContractRevision{
		ID: "revision-other", ContractID: fixture.baseline.ContractID,
	}); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "operational", "state.json")
	state := readOperationalStateJSON(t, root)
	track := state["releaseTracks"].(map[string]any)[releaseTrackKey(fixture.next.ContractID, fixture.next.ID)].(map[string]any)
	track["currentRevisionId"] = "revision-other"
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(statePath, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileStore(root).ReleaseTrack(
		ctx,
		fixture.next.ContractID,
		fixture.next.ID,
	); err == nil {
		t.Fatal("restart accepted an outstanding decision against a non-current baseline")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, tampered) {
		t.Fatal("rejected restart rewrote tampered state")
	}
}

func TestFileStoreRestartRejectsZeroGenerationDecisionAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, false)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "operational", "state.json")
	state := readOperationalStateJSON(t, root)
	trackKey := releaseTrackKey(fixture.next.ContractID, fixture.next.ID)
	state["releaseTracks"].(map[string]any)[trackKey].(map[string]any)["generation"] = float64(0)
	state["releaseTrackAuthorities"].(map[string]any)[trackKey].(map[string]any)["generation"] = float64(0)

	zeroEventID := storedReleaseEvidenceID(fixture.authorization, false, 0)
	auditEvents := state["auditEvents"].(map[string]any)
	for key, raw := range auditEvents {
		event := raw.(map[string]any)
		delete(auditEvents, key)
		event["id"] = zeroEventID
		auditEvents[zeroEventID] = event
	}
	zeroOutboxID := "outbox-" + zeroEventID[len("audit-"):]
	outbox := state["outbox"].(map[string]any)
	for key, raw := range outbox {
		message := raw.(map[string]any)
		delete(outbox, key)
		message["id"] = zeroOutboxID
		outbox[zeroOutboxID] = message
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(statePath, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileStore(root).ReleaseTrack(
		ctx,
		fixture.next.ContractID,
		fixture.next.ID,
	); err == nil {
		t.Fatal("restart accepted zero-generation decision authority")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, tampered) {
		t.Fatal("rejected restart rewrote zero-generation authority")
	}
}
