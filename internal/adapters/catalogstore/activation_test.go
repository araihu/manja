package catalogstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/application/catalog"
)

func TestActivationPersistsAndRestoresExactRouteTable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(7)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := compiledFixture(t)
	receipt, err := coordinator.Activate(context.Background(), "/catalog", "", 7, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SnapshotID != snapshot.ID || runtime.Table().Mounts["/catalog"].Active.ID != snapshot.ID {
		t.Fatalf("activation receipt/table = %#v / %#v", receipt, runtime.Table())
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
	if table := restarted.Table(); table.Generation != 7 || table.Mounts["/catalog"].Active.ID != snapshot.ID {
		t.Fatalf("recovered route table = %#v", table)
	}
}

func TestDurableStateWritersRejectOversizedDataBeforeWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coordinator, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(1))
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()

	journal := activationJournalV1{SchemaVersion: 1, Mount: "/" + strings.Repeat("j", maxActivationJournalBytes), Generation: 1}
	if err := coordinator.writeJournal(journal); !errors.Is(err, ErrStorageBudget) {
		t.Fatalf("oversized journal error = %v, want %v", err, ErrStorageBudget)
	}
	if _, err := os.Stat(coordinator.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("oversized journal write left state: %v", err)
	}

	if err := coordinator.writeRouteTableBytes(bytes.Repeat([]byte("x"), maxDurableRouteTableBytes+1)); !errors.Is(err, ErrStorageBudget) {
		t.Fatalf("oversized route table error = %v, want %v", err, ErrStorageBudget)
	}
	if _, err := os.Stat(coordinator.routeTablePath()); !os.IsNotExist(err) {
		t.Fatalf("oversized route table write left state: %v", err)
	}

	mounts := make(map[string]catalog.MountState, maxDurableMounts+1)
	for index := 0; index <= maxDurableMounts; index++ {
		mounts["/mount-"+strings.Repeat("x", index+1)] = catalog.MountState{Active: catalog.RuntimeSnapshot{
			ID:       catalog.SnapshotID("snapshot-sha256-" + strings.Repeat("a", 64)),
			Location: "memory",
			Directory: catalog.CatalogArtifactV1{
				SchemaVersion: 1,
				CatalogID:     "catalog",
			},
			Search: catalog.SearchDirectoryV1{SchemaVersion: 1},
		}}
	}
	if _, err := encodeRouteTable(&catalog.RouteTable{Generation: 1, Mounts: mounts}); !errors.Is(err, ErrStorageBudget) {
		t.Fatalf("oversized route mount count error = %v, want %v", err, ErrStorageBudget)
	}
	if _, err := encodeRouteTableWithResourceLimits(&catalog.RouteTable{Generation: 1, Mounts: mounts}, false); err != nil {
		t.Fatalf("unbounded route table rejected experimental mount count: %v", err)
	}
}

func TestOversizedActivationFailsBeforePublishingOrServing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()

	for index := 0; index < maxDurableMounts; index++ {
		mount := "/existing-" + strings.Repeat("x", index+1)
		snapshot := catalog.RuntimeSnapshot{
			ID:       catalog.SnapshotID("snapshot-sha256-" + fmt.Sprintf("%064x", index+1)),
			Location: "memory",
			Directory: catalog.CatalogArtifactV1{
				SchemaVersion: 1,
				CatalogID:     "catalog",
			},
			Search: catalog.SearchDirectoryV1{SchemaVersion: 1},
		}
		if _, err := runtime.ActivateMount(mount, "", 1, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	before := runtime.Table()
	candidate := compiledFixtureVersion(t, "oversized")
	if _, err := coordinator.Activate(context.Background(), "/new", "", 1, candidate); !errors.Is(err, ErrStorageBudget) {
		t.Fatalf("oversized mount-count activation error = %v, want %v", err, ErrStorageBudget)
	}
	after := runtime.Table()
	if len(after.Mounts) != len(before.Mounts) || after.Mounts["/new"].Active.ID != "" {
		t.Fatalf("oversized activation changed runtime: before=%#v after=%#v", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, "snapshots", string(candidate.ID))); !os.IsNotExist(err) {
		t.Fatalf("oversized activation published candidate: %v", err)
	}
	if _, err := os.Stat(coordinator.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("oversized activation left journal: %v", err)
	}
}

func TestActivationStaleExpectedOldLeavesDurablePointerUnchanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(2)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 2, first); err != nil {
		t.Fatal(err)
	}
	pointerBefore, err := os.ReadFile(coordinator.routeTablePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 2, second); !errors.Is(err, catalog.ErrStaleSnapshot) {
		t.Fatalf("stale activation error = %v", err)
	}
	pointerAfter, err := os.ReadFile(coordinator.routeTablePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(pointerBefore) != string(pointerAfter) || runtime.Table().Mounts["/catalog"].Active.ID != first.ID {
		t.Fatal("stale activation changed durable or in-process state")
	}
}

func TestActivationAdmissionRunsAfterStagingAndBeforeRoutePublication(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(2)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 2, first); err != nil {
		t.Fatal(err)
	}
	pointerBefore, err := os.ReadFile(coordinator.routeTablePath())
	if err != nil {
		t.Fatal(err)
	}

	staged := false
	coordinator.hooks.afterPreflight = func() error {
		staged = true
		return nil
	}
	errAdmission := errors.New("process budget admission rejected")
	_, err = coordinator.ActivateAdmitted(context.Background(), "/catalog", first.ID, 2, second, nil, func() error {
		if !staged {
			t.Fatal("activation admission ran before immutable staging and preflight")
		}
		return errAdmission
	})
	if !errors.Is(err, errAdmission) {
		t.Fatalf("activation error = %v, want %v", err, errAdmission)
	}
	if runtime.Table().Mounts["/catalog"].Active.ID != first.ID {
		t.Fatalf("runtime route changed after rejected admission: %#v", runtime.Table())
	}
	pointerAfter, err := os.ReadFile(coordinator.routeTablePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(pointerAfter) != string(pointerBefore) {
		t.Fatalf("durable route changed after rejected admission:\nbefore=%s\nafter=%s", pointerBefore, pointerAfter)
	}
	if _, err := os.Stat(coordinator.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected activation journal = %v, want absent", err)
	}
	if _, err := coordinator.store.Preflight(context.Background(), second.ID); err != nil {
		t.Fatalf("staged immutable candidate is not available for bounded reclamation: %v", err)
	}
}

func TestActivationCancellationAfterPreflightLeavesLKGUnchanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(2)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 2, first); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coordinator.hooks.afterPreflight = func() error {
		cancel()
		return nil
	}
	if _, err := coordinator.ActivateAdmitted(ctx, "/catalog", first.ID, 2, second, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled activation error = %v, want %v", err, context.Canceled)
	}
	if active := runtime.Table().Mounts["/catalog"].Active.ID; active != first.ID {
		t.Fatalf("cancelled activation changed LKG to %q, want %q", active, first.ID)
	}
	if _, err := os.Stat(coordinator.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("cancelled activation journal = %v, want absent", err)
	}
}

func TestUnchangedActivationStillRunsPostStagingAdmission(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(2)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	snapshot := compiledFixtureVersion(t, "same")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 2, snapshot); err != nil {
		t.Fatal(err)
	}

	staged := false
	coordinator.hooks.afterPreflight = func() error {
		staged = true
		return nil
	}
	errAdmission := errors.New("process budget admission rejected")
	_, err = coordinator.ActivateAdmitted(context.Background(), "/catalog", snapshot.ID, 2, snapshot, nil, func() error {
		if !staged {
			t.Fatal("unchanged activation admission ran before preflight")
		}
		return errAdmission
	})
	if !errors.Is(err, errAdmission) {
		t.Fatalf("unchanged activation error = %v, want %v", err, errAdmission)
	}
	if _, err := os.Stat(coordinator.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("unchanged rejected activation journal = %v, want absent", err)
	}
}

func TestConcurrentDifferentMountActivationsBothSurviveDurably(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(4)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for _, test := range []struct {
		mount    string
		snapshot catalog.CompiledSnapshot
	}{{"/alpha", compiledFixtureVersion(t, "alpha")}, {"/beta", compiledFixtureVersion(t, "beta")}} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := coordinator.Activate(context.Background(), test.mount, "", 4, test.snapshot)
			errorsSeen <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(runtime.Table().Mounts) != 2 {
		t.Fatalf("runtime routes = %#v", runtime.Table())
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
	if len(restarted.Table().Mounts) != 2 {
		t.Fatalf("durable routes = %#v", restarted.Table())
	}
}

func TestStartupRecoversJournalOnlyWhenExpectedStateMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(9)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := compiledFixture(t)
	if _, err := coordinator.store.Publish(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.writeJournal(activationJournalV1{SchemaVersion: 1, Mount: "/catalog", Candidate: snapshot.ID, Generation: 9}); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := catalog.NewRuntime(9)
	reopened, err := OpenActivationCoordinator(context.Background(), root, restarted)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if restarted.Table().Mounts["/catalog"].Active.ID != snapshot.ID {
		t.Fatalf("journal recovery table = %#v", restarted.Table())
	}
	if _, err := os.Stat(reopened.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("recovered journal still exists: %v", err)
	}
}

func TestSecondProcessLockFailsWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	first, err := OpenActivationCoordinator(context.Background(), root, catalog.NewRuntime(1))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := OpenActivationCoordinator(ctx, root, catalog.NewRuntime(1)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock error = %v", err)
	}
	if matches, err := filepath.Glob(filepath.Join(root, "state", "*")); err != nil || len(matches) != 0 {
		t.Fatalf("failed lock mutated state: %v, %v", matches, err)
	}
}

func TestStartupCorruptionFallsBackOnceAndQuarantinesActive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(5)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first := compiledFixtureVersion(t, "healthy")
	second := compiledFixtureVersion(t, "corrupt")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 5, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 5, second); err != nil {
		t.Fatal(err)
	}
	corruptCompiledChild(t, coordinator.store, second)
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := catalog.NewRuntime(0)
	reopened, err := OpenActivationCoordinator(context.Background(), root, restarted)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state := restarted.Table().Mounts["/catalog"]
	if state.Active.ID != first.ID || state.Previous != nil {
		t.Fatalf("corruption fallback = %#v", state)
	}
	if _, err := os.Stat(filepath.Join(root, "quarantine", string(second.ID))); err != nil {
		t.Fatalf("corrupt snapshot was not quarantined: %v", err)
	}
}

func TestRuntimeCorruptionTransitionDisablesMountWithoutHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	snapshot := compiledFixture(t)
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.HandleCorruption(context.Background(), "/catalog", snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if _, exists := runtime.Table().Mounts["/catalog"]; exists {
		t.Fatalf("corrupt mount remains active: %#v", runtime.Table())
	}
	if _, err := os.Stat(filepath.Join(root, "quarantine", string(snapshot.ID))); err != nil {
		t.Fatalf("corrupt snapshot was not quarantined: %v", err)
	}
}

func TestGarbageCollectionWaitsForAdmissionQuiescence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(3)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	third := compiledFixtureVersion(t, "third")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 3, first); err != nil {
		t.Fatal(err)
	}
	admission, err := runtime.Admit("/catalog")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 3, second); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", second.ID, 3, third); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.GarbageCollect(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstPath, _ := coordinator.store.snapshotPath(first.ID)
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("admitted snapshot was collected: %v", err)
	}
	admission.Release()
	if err := coordinator.GarbageCollect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("unreferenced snapshot remains: %v", err)
	}
}

func TestActivationReclaimsUnreferencedStorageBeforeAdmissionRetry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()

	unreferenced := filepath.Join(root, "snapshots", "unreferenced", "sparse.bin")
	if err := os.MkdirAll(filepath.Dir(unreferenced), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(unreferenced)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(int64(MaxStoredBytes)); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot := compiledFixture(t)
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, snapshot); err != nil {
		t.Fatalf("activation with reclaimable storage: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(unreferenced)); !os.IsNotExist(err) {
		t.Fatalf("unreferenced storage remains after admission: %v", err)
	}
	if _, err := coordinator.store.Preflight(context.Background(), snapshot.ID); err != nil {
		t.Fatalf("admitted snapshot unreadable after reclamation: %v", err)
	}
}

func TestPendingActivationJournalBlocksLaterTransition(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	third := compiledFixtureVersion(t, "third")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	errCrash := errors.New("simulated process crash")
	coordinator.hooks.afterJournal = func() error { return errCrash }
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 1, second); !errors.Is(err, errCrash) {
		t.Fatalf("second activation error = %v, want %v", err, errCrash)
	}
	before := runtime.Table()
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 1, third); !errors.Is(err, ErrActivationPending) {
		t.Fatalf("third activation error = %v, want %v", err, ErrActivationPending)
	}
	after := runtime.Table()
	if after.Generation != before.Generation || after.Mounts["/catalog"].Active.ID != before.Mounts["/catalog"].Active.ID {
		t.Fatalf("pending activation changed last-known-good route: before=%#v after=%#v", before, after)
	}
	journal, err := os.ReadFile(coordinator.journalPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte(second.ID)) || bytes.Contains(journal, []byte(third.ID)) {
		t.Fatalf("pending journal changed after rejected transition: %s", journal)
	}
}

func TestWithdrawalPersistsTombstoneAndBlocksRestartReactivation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first := compiledFixture(t)
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Withdraw(context.Background(), "/catalog", first.ID, 1, catalog.TombstoneWithdrawn); err != nil {
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
	if _, exists := restarted.Table().Mounts["/catalog"]; exists {
		t.Fatalf("withdrawn route was restored as active: %#v", restarted.Table())
	}
	if tombstone, exists := restarted.Table().Tombstones["/catalog"]; !exists || tombstone.State != catalog.TombstoneWithdrawn || tombstone.SnapshotID != first.ID {
		t.Fatalf("recovered tombstone = %#v, exists=%t", tombstone, exists)
	}
	if _, err := reopened.Activate(context.Background(), "/catalog", "", 1, compiledFixtureVersion(t, "successor")); !errors.Is(err, catalog.ErrMountWithdrawn) {
		t.Fatalf("implicit reactivation error = %v, want %v", err, catalog.ErrMountWithdrawn)
	}
}

func TestReauthorizePersistsFreshActiveWithoutTombstonedPrevious(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Withdraw(context.Background(), "/catalog", first.ID, 1, catalog.TombstoneDeleted); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Reauthorize(context.Background(), "/catalog", 1, second); err != nil {
		t.Fatal(err)
	}
	state := runtime.Table().Mounts["/catalog"]
	if state.Active.ID != second.ID || state.Previous != nil {
		t.Fatalf("reauthorized state = %#v", state)
	}
	if _, exists := runtime.Table().Tombstones["/catalog"]; exists {
		t.Fatalf("reauthorized tombstone remains: %#v", runtime.Table())
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
	if got := restarted.Table().Mounts["/catalog"].Active.ID; got != second.ID {
		t.Fatalf("recovered reauthorized active = %q, want %q", got, second.ID)
	}
}

func TestActivationRejectsCatalogIdentityChange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureCatalogVersion(t, "catalog", "first")
	other := compiledFixtureCatalogVersion(t, "other", "other")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 1, other); !errors.Is(err, ErrActivationIdentity) {
		t.Fatalf("cross-catalog activation error = %v, want %v", err, ErrActivationIdentity)
	}
	if active := runtime.Table().Mounts["/catalog"].Active.ID; active != first.ID {
		t.Fatalf("cross-catalog activation changed active LKG to %q, want %q", active, first.ID)
	}
}

func TestHandleCorruptionDoesNotResurrectCorruptPrevious(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	first := compiledFixtureVersion(t, "first")
	second := compiledFixtureVersion(t, "second")
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), "/catalog", first.ID, 1, second); err != nil {
		t.Fatal(err)
	}
	corruptCompiledChild(t, coordinator.store, first)
	corruptCompiledChild(t, coordinator.store, second)
	if err := coordinator.HandleCorruption(context.Background(), "/catalog", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, exists := runtime.Table().Mounts["/catalog"]; exists {
		t.Fatalf("corruption fallback resurrected an invalid previous snapshot: %#v", runtime.Table())
	}
}

func corruptCompiledChild(t *testing.T, store *Store, snapshot catalog.CompiledSnapshot) {
	t.Helper()
	location, err := store.snapshotPath(snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range snapshot.Children {
		if child.Kind == "manifest" {
			continue
		}
		path := filepath.Join(location, filepath.FromSlash(child.Path))
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[0] ^= 1
		if err := os.WriteFile(path, data, 0o444); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("snapshot has no corruptible child")
}
