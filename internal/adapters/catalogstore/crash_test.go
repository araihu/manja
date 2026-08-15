package catalogstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

func TestActivationCrashRecoveryAtDurableBoundaries(t *testing.T) {
	t.Parallel()

	errCrash := errors.New("simulated process crash")
	for _, test := range []struct {
		name          string
		suffix        string
		hooks         activationHooks
		wantActivated bool
	}{
		{name: "after immutable preflight", suffix: "preflight", hooks: activationHooks{afterPreflight: func() error { return errCrash }}},
		{name: "after journal fsync", suffix: "journal", hooks: activationHooks{afterJournal: func() error { return errCrash }}, wantActivated: true},
		{name: "after active pointer replace", suffix: "pointer", hooks: activationHooks{afterPointer: func() error { return errCrash }}, wantActivated: true},
		{name: "after in-process swap", suffix: "runtime", hooks: activationHooks{afterRuntime: func() error { return errCrash }}, wantActivated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime := catalog.NewRuntime(6)
			coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
			if err != nil {
				t.Fatal(err)
			}
			coordinator.hooks = test.hooks
			snapshot := compiledFixtureVersion(t, test.suffix)
			if _, err := coordinator.Activate(context.Background(), "/catalog", "", 6, snapshot); !errors.Is(err, errCrash) {
				t.Fatalf("crash error = %v", err)
			}
			if err := coordinator.Close(); err != nil {
				t.Fatal(err)
			}

			restarted := catalog.NewRuntime(6)
			reopened, err := OpenActivationCoordinator(context.Background(), root, restarted)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			state, active := restarted.Table().Mounts["/catalog"]
			if active != test.wantActivated {
				t.Fatalf("recovered active=%t, want %t: %#v", active, test.wantActivated, restarted.Table())
			}
			if active && state.Active.ID != snapshot.ID {
				t.Fatalf("recovered snapshot = %q, want %q", state.Active.ID, snapshot.ID)
			}
		})
	}
}

func TestRecoveryArchivesStaleJournalWithoutOverwritingNewerRoute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(8)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	active := compiledFixtureVersion(t, "active")
	stale := compiledFixtureVersion(t, "stale")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 8, active); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.store.Publish(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.writeJournal(activationJournalV1{SchemaVersion: 1, Mount: "/catalog", Candidate: stale.ID, Generation: 8}); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := catalog.NewRuntime(0)
	reopened, err := OpenActivationCoordinator(context.Background(), root, restarted)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if restarted.Table().Mounts["/catalog"].Active.ID != active.ID {
		t.Fatalf("stale journal overwrote active route: %#v", restarted.Table())
	}
}

func TestRecoveryRejectsNonCanonicalRouteTable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schemaVersion":1,"generation":9,"generation":9,"mounts":{}}`)
	if err := os.WriteFile(filepath.Join(state, "routes.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(0))
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("duplicate route-table keys error = %v, want %v", err, ErrCorruptSnapshot)
	}
}

func TestRecoveryRejectsOversizedActivationJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coordinator, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(9))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := compiledFixture(t)
	if _, err := coordinator.store.Publish(context.Background(), snapshot); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"schemaVersion":1,"mount":"/catalog","candidate":%q,"generation":9}`, snapshot.ID)
	data := append(bytes.Repeat([]byte(" "), 128<<10), []byte(valid)...)
	if err := os.WriteFile(filepath.Join(state, "activation-journal.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(9))
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("oversized activation journal error = %v, want %v", err, ErrCorruptSnapshot)
	}
}

func TestRecoveryRejectsNonCanonicalActivationJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coordinator, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(9))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := compiledFixture(t)
	if _, err := coordinator.store.Publish(context.Background(), snapshot); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(fmt.Sprintf(`{"schemaVersion":1,"mount":"/catalog","candidate":%q,"candidate":%q,"generation":9}`, snapshot.ID, snapshot.ID))
	if err := os.WriteFile(filepath.Join(state, "activation-journal.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(9))
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("duplicate journal keys error = %v, want %v", err, ErrCorruptSnapshot)
	}
}

func TestRecoveryRejectsActivationJournalAcrossCatalogIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first := compiledFixtureCatalogVersion(t, "catalog", "first")
	other := compiledFixtureCatalogVersion(t, "other", "other")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.store.Publish(context.Background(), other); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.writeJournal(activationJournalV1{SchemaVersion: 1, Mount: "/catalog", CatalogID: "other", Candidate: other.ID, ExpectedOld: first.ID, Generation: 1}); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(0))
	if !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("cross-catalog journal recovery error = %v, want %v", err, ErrCorruptSnapshot)
	}
}

func TestRecoveryRejectsRouteCatalogIdentityMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coordinator, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, compiledFixtureCatalogVersion(t, "catalog", "first")); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	routesPath := filepath.Join(root, "state", "routes.json")
	routes, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(routes, []byte(`"catalogId":"catalog"`)) {
		t.Fatalf("route table omitted catalog identity: %s", routes)
	}
	routes = bytes.Replace(routes, []byte(`"catalogId":"catalog"`), []byte(`"catalogId":"other"`), 1)
	if err := os.WriteFile(routesPath, routes, 0o600); err != nil {
		t.Fatal(err)
	}
	recoveredRuntime := catalog.NewRuntime(0)
	recovered, err := OpenActivationCoordinator(context.Background(), root, recoveredRuntime)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recoveredRuntime.HasMount("/catalog") {
		t.Fatalf("route catalog identity mismatch left mount active: %#v", recoveredRuntime.Table())
	}
	if _, err := os.Stat(filepath.Join(root, "quarantine")); err != nil {
		t.Fatalf("mismatched route snapshot was not quarantined: %v", err)
	}
}

func TestRecoveryDoesNotGuessLegacyFallbackCatalogIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coordinator, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(1))
	if err != nil {
		t.Fatal(err)
	}
	active := compiledFixtureCatalogVersion(t, "catalog", "active")
	legacyPrevious := compiledFixtureCatalogVersion(t, "other", "previous")
	for _, snapshot := range []catalog.CompiledSnapshot{active, legacyPrevious} {
		if _, err := coordinator.store.Publish(context.Background(), snapshot); err != nil {
			_ = coordinator.Close()
			t.Fatal(err)
		}
	}
	legacy := durableRouteTableV1{
		SchemaVersion: 1,
		Generation:    1,
		Mounts: map[string]durableMountV1{
			"/catalog": {Active: active.ID, Previous: legacyPrevious.ID},
		},
	}
	routes, err := json.Marshal(legacy)
	if err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.writeRouteTableBytes(routes); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	corruptCompiledChild(t, coordinator.store, active)
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}

	recoveredRuntime := catalog.NewRuntime(0)
	recovered, err := OpenActivationCoordinator(context.Background(), root, recoveredRuntime)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recoveredRuntime.HasMount("/catalog") {
		t.Fatalf("legacy route table guessed a fallback catalog: %#v", recoveredRuntime.Table())
	}
}
