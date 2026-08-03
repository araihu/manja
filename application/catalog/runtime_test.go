package catalog

import (
	"errors"
	"sync"
	"testing"
)

func TestRuntimeActivationRejectsStaleSameMountAndPreservesDifferentMounts(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(7)
	alpha := runtimeSnapshotFixture("a")
	beta := runtimeSnapshotFixture("b")
	if _, err := runtime.ActivateMount("/alpha", "", 7, alpha); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ActivateMount("/alpha", "", 7, beta); !errors.Is(err, ErrStaleSnapshot) {
		t.Fatalf("stale same-mount error = %v", err)
	}
	if _, err := runtime.ActivateMount("/beta", "", 7, beta); err != nil {
		t.Fatal(err)
	}
	table := runtime.Table()
	if table.Mounts["/alpha"].Active.ID != alpha.ID || table.Mounts["/beta"].Active.ID != beta.ID {
		t.Fatalf("unrelated mount activation was lost: %#v", table)
	}
}

func TestRuntimeConcurrentDifferentMountActivationsBothSurvive(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(3)
	start := make(chan struct{})
	errorsByMount := make(chan error, 2)
	var wait sync.WaitGroup
	for _, test := range []struct {
		mount    string
		snapshot RuntimeSnapshot
	}{{mount: "/alpha", snapshot: runtimeSnapshotFixture("a")}, {mount: "/beta", snapshot: runtimeSnapshotFixture("b")}} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := runtime.ActivateMount(test.mount, "", 3, test.snapshot)
			errorsByMount <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByMount)
	for err := range errorsByMount {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(runtime.Table().Mounts) != 2 {
		t.Fatalf("concurrent route table = %#v", runtime.Table())
	}
}

func TestRuntimeConfigGenerationRejectsStalePartialResult(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(10)
	alpha := runtimeSnapshotFixture("a")
	if err := runtime.ReplaceRoutes(10, 11, map[string]RuntimeSnapshot{"/alpha": alpha}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ActivateMount("/beta", "", 10, runtimeSnapshotFixture("b")); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale generation error = %v", err)
	}
	if table := runtime.Table(); table.Generation != 11 || len(table.Mounts) != 1 || table.Mounts["/alpha"].Active.ID != alpha.ID {
		t.Fatalf("stale compiler changed route table: %#v", table)
	}
}

func TestRuntimeAdmissionPinsOldSnapshotUntilRelease(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	second := runtimeSnapshotFixture("b")
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	admission, err := runtime.Admit("/catalog")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ActivateMount("/catalog", first.ID, 1, second); err != nil {
		t.Fatal(err)
	}
	if admission.Snapshot.ID != first.ID || runtime.ReferenceCount(first.ID) != 1 {
		t.Fatalf("old admission was not pinned: %#v refs=%d", admission, runtime.ReferenceCount(first.ID))
	}
	state := runtime.Table().Mounts["/catalog"]
	if state.Active.ID != second.ID || state.Previous == nil || state.Previous.ID != first.ID {
		t.Fatalf("active/previous state = %#v", state)
	}
	previousAdmission, err := runtime.AdmitSnapshot("/catalog", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previousAdmission.Snapshot.ID != first.ID {
		t.Fatalf("qualified admission = %#v", previousAdmission)
	}
	previousAdmission.Release()
	if _, err := runtime.AdmitSnapshot("/catalog", runtimeSnapshotFixture("c").ID); !errors.Is(err, ErrMountUnavailable) {
		t.Fatalf("unretained snapshot admission error = %v", err)
	}
	admission.Release()
	admission.Release()
	if runtime.ReferenceCount(first.ID) != 0 {
		t.Fatalf("release count = %d", runtime.ReferenceCount(first.ID))
	}
}

func TestRuntimeRejectsInvalidMountAndUnavailableAdmission(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	if _, err := runtime.ActivateMount("catalog/", "", 1, runtimeSnapshotFixture("a")); !errors.Is(err, ErrInvalidMount) {
		t.Fatalf("invalid mount error = %v", err)
	}
	if _, err := runtime.Admit("/missing"); !errors.Is(err, ErrMountUnavailable) {
		t.Fatalf("missing mount error = %v", err)
	}
}

func TestRuntimeDurableActivationDoesNotPublishWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	want := errors.New("pointer write failed")
	_, err := runtime.ActivateMountDurably("/catalog", "", 1, runtimeSnapshotFixture("a"), func(table *RouteTable) error {
		if table.Mounts["/catalog"].Active.ID == "" {
			t.Fatal("persistence callback did not receive complete table")
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("activation error = %v", err)
	}
	if len(runtime.Table().Mounts) != 0 {
		t.Fatalf("failed persistence changed runtime: %#v", runtime.Table())
	}
}

func TestRuntimeFallbackPromotesOnlyPreviousAndCanDisableMount(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	second := runtimeSnapshotFixture("b")
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ActivateMount("/catalog", first.ID, 1, second); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.FallbackMountDurably("/catalog", second.ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	if state := runtime.Table().Mounts["/catalog"]; state.Active.ID != first.ID || state.Previous != nil {
		t.Fatalf("fallback state = %#v", state)
	}
	if _, err := runtime.FallbackMountDurably("/catalog", first.ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	if _, exists := runtime.Table().Mounts["/catalog"]; exists {
		t.Fatalf("mount was not disabled: %#v", runtime.Table())
	}
}

func runtimeSnapshotFixture(suffix string) RuntimeSnapshot {
	return RuntimeSnapshot{
		ID:       SnapshotID("snapshot-sha256-" + repeatRuntimeHex(suffix)),
		Location: "/snapshots/" + suffix,
		Directory: CatalogArtifactV1{
			SchemaVersion: 1, CatalogID: "catalog-" + suffix, SearchChild: "search/directory.json",
		},
		Search: SearchDirectoryV1{SchemaVersion: 1, SearchVersion: 1},
	}
}

func repeatRuntimeHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
