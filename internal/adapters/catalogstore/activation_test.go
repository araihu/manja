package catalogstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
