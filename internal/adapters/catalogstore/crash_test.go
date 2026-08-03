package catalogstore

import (
	"context"
	"errors"
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
