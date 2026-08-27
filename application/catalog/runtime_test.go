package catalog

import (
	"errors"
	"sync"
	"testing"

	"github.com/araihu/manja/application/projection"
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

func TestRuntimeDeepCopiesNestedSnapshotMetadataForTableAndAdmission(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	candidate := runtimeSnapshotFixture("a")
	candidate.Directory.Documents = []DocumentDirectoryV1{{
		Key: "document",
		Overview: projection.Overview{Servers: []projection.Server{{
			ID: "server",
			Variables: []projection.ServerVariable{{
				ID:   "variable",
				Enum: []projection.TextRecord{{ID: "enum", Value: "one"}},
			}},
		}}},
		SecuritySchemes: []SecuritySchemeDirectoryV1{{Name: "scheme", Description: "original"}},
	}}
	if _, err := runtime.ActivateMount("/catalog", "", 1, candidate); err != nil {
		t.Fatal(err)
	}

	candidate.Directory.Documents[0].SecuritySchemes[0].Name = "candidate-mutated"
	candidate.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value = "candidate-mutated"
	stored := runtime.Table().Mounts["/catalog"].Active
	if stored.Directory.Documents[0].SecuritySchemes[0].Name != "scheme" || stored.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value != "one" {
		t.Fatalf("candidate mutation changed active snapshot: %#v", stored.Directory.Documents[0])
	}

	admission, err := runtime.Admit("/catalog")
	if err != nil {
		t.Fatal(err)
	}
	admission.Snapshot.Directory.Documents[0].SecuritySchemes[0].Description = "admission-mutated"
	admission.Snapshot.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value = "admission-mutated"
	admission.Release()
	stored = runtime.Table().Mounts["/catalog"].Active
	if stored.Directory.Documents[0].SecuritySchemes[0].Description != "original" || stored.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value != "one" {
		t.Fatalf("admission mutation changed active snapshot: %#v", stored.Directory.Documents[0])
	}

	table := runtime.Table()
	table.Mounts["/catalog"].Active.Directory.Documents[0].SecuritySchemes[0].Name = "table-mutated"
	table.Mounts["/catalog"].Active.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value = "table-mutated"
	stored = runtime.Table().Mounts["/catalog"].Active
	if stored.Directory.Documents[0].SecuritySchemes[0].Name != "scheme" || stored.Directory.Documents[0].Overview.Servers[0].Variables[0].Enum[0].Value != "one" {
		t.Fatalf("table mutation changed active snapshot: %#v", stored.Directory.Documents[0])
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

func TestRuntimeSameSnapshotActivationPreservesDistinctPrevious(t *testing.T) {
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
	if _, err := runtime.ActivateMount("/catalog", second.ID, 1, second); err != nil {
		t.Fatal(err)
	}

	state := runtime.Table().Mounts["/catalog"]
	if state.Active.ID != second.ID {
		t.Fatalf("active snapshot = %q, want %q", state.Active.ID, second.ID)
	}
	if state.Previous == nil || state.Previous.ID != first.ID {
		t.Fatalf("previous snapshot = %#v, want %q", state.Previous, first.ID)
	}
	if _, err := runtime.FallbackMountDurably("/catalog", second.ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	state = runtime.Table().Mounts["/catalog"]
	if state.Active.ID != first.ID || state.Previous != nil {
		t.Fatalf("fallback state = %#v, want active %q with no previous", state, first.ID)
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

func TestRuntimeWithdrawalPersistsTombstoneAndBlocksImplicitReactivation(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	second := runtimeSnapshotFixture("b")
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	tombstone := MountTombstone{
		State:      TombstoneWithdrawn,
		CatalogID:  first.Directory.CatalogID,
		SnapshotID: first.ID,
	}
	if _, err := runtime.WithdrawMountDurably("/catalog", first.ID, 1, tombstone, nil); err != nil {
		t.Fatal(err)
	}
	table := runtime.Table()
	if _, exists := table.Mounts["/catalog"]; exists {
		t.Fatalf("withdrawn mount remains active: %#v", table)
	}
	if got := table.Tombstones["/catalog"]; got != tombstone {
		t.Fatalf("tombstone = %#v, want %#v", got, tombstone)
	}
	if _, err := runtime.ActivateMount("/catalog", "", 1, second); !errors.Is(err, ErrMountWithdrawn) {
		t.Fatalf("implicit reactivation error = %v, want %v", err, ErrMountWithdrawn)
	}
	if _, err := runtime.Admit("/catalog"); !errors.Is(err, ErrMountWithdrawn) {
		t.Fatalf("withdrawn admission error = %v, want %v", err, ErrMountWithdrawn)
	}
}

func TestRuntimeReauthorizationAtomicallyReplacesTombstone(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	second := runtimeSnapshotFixture("b")
	second.Directory.CatalogID = first.Directory.CatalogID
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.WithdrawMountDurably("/catalog", first.ID, 1, MountTombstone{
		State: TombstoneDeleted, CatalogID: first.Directory.CatalogID, SnapshotID: first.ID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReauthorizeMountDurably("/catalog", 1, second, nil); err != nil {
		t.Fatal(err)
	}
	table := runtime.Table()
	state, exists := table.Mounts["/catalog"]
	if !exists || state.Active.ID != second.ID || state.Previous != nil {
		t.Fatalf("reauthorized route = %#v", table)
	}
	if _, exists := table.Tombstones["/catalog"]; exists {
		t.Fatalf("reauthorized tombstone remains: %#v", table)
	}
}

func TestRuntimeReauthorizationRejectsTombstonedSnapshotIdentity(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.WithdrawMountDurably("/catalog", first.ID, 1, MountTombstone{
		State: TombstoneWithdrawn, CatalogID: first.Directory.CatalogID, SnapshotID: first.ID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReauthorizeMountDurably("/catalog", 1, first, nil); !errors.Is(err, ErrInvalidTombstone) {
		t.Fatalf("tombstoned snapshot reauthorization error = %v, want %v", err, ErrInvalidTombstone)
	}
}

func TestRuntimeWithdrawalDoesNotPublishWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(1)
	first := runtimeSnapshotFixture("a")
	if _, err := runtime.ActivateMount("/catalog", "", 1, first); err != nil {
		t.Fatal(err)
	}
	want := errors.New("tombstone write failed")
	_, err := runtime.WithdrawMountDurably("/catalog", first.ID, 1, MountTombstone{
		State: TombstoneWithdrawn, CatalogID: first.Directory.CatalogID, SnapshotID: first.ID,
	}, func(*RouteTable) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("withdrawal error = %v, want %v", err, want)
	}
	table := runtime.Table()
	if table.Mounts["/catalog"].Active.ID != first.ID || len(table.Tombstones) != 0 {
		t.Fatalf("failed withdrawal changed runtime: %#v", table)
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
