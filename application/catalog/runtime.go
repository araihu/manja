package catalog

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var (
	ErrInvalidMount     = errors.New("catalog: invalid mount")
	ErrInvalidSnapshot  = errors.New("catalog: invalid runtime snapshot")
	ErrStaleSnapshot    = errors.New("catalog: stale snapshot")
	ErrStaleGeneration  = errors.New("catalog: stale configuration generation")
	ErrMountUnavailable = errors.New("catalog: mount unavailable")
	ErrMountWithdrawn   = errors.New("catalog: mount withdrawn")
	ErrInvalidTombstone = errors.New("catalog: invalid mount tombstone")
	ErrRuntimeBusy      = errors.New("catalog: runtime has active admissions")
)

type TombstoneState string

const (
	TombstoneWithdrawn TombstoneState = "withdrawn"
	TombstoneDeleted   TombstoneState = "deleted"
)

// MountTombstone is durable authority that prevents a withdrawn route from
// being implicitly reactivated. SnapshotID records the last public identity;
// it is not a serving pointer and may be garbage-collected after withdrawal.
type MountTombstone struct {
	State      TombstoneState
	CatalogID  string
	SnapshotID SnapshotID
}

// RuntimeSnapshot is the verified, immutable snapshot admitted for rendering.
// Location names the content-addressed directory that owns Directory and Search.
type RuntimeSnapshot struct {
	ID        SnapshotID
	Location  string
	Directory CatalogArtifactV1
	Search    SearchDirectoryV1
	Manifest  ManifestV1
}

type MountState struct {
	Active   RuntimeSnapshot
	Previous *RuntimeSnapshot
}

type RouteTable struct {
	Generation uint64
	Mounts     map[string]MountState
	Tombstones map[string]MountTombstone
}

// MountActivation is the bounded result of one durable route transition. It
// lets persistence coordinators avoid cloning the complete route table again
// after the durable pointer and in-process table have been published.
type MountActivation struct {
	Generation uint64
	ActiveID   SnapshotID
	PreviousID SnapshotID
	Changed    bool
}

// Runtime publishes immutable route-table replacements. The mutex serializes
// writers and request reference counts; readers load the current table once.
type Runtime struct {
	writes sync.Mutex
	table  atomic.Pointer[RouteTable]
	refs   map[SnapshotID]uint64
}

func NewRuntime(generation uint64) *Runtime {
	runtime := &Runtime{refs: make(map[SnapshotID]uint64)}
	runtime.table.Store(&RouteTable{Generation: generation, Mounts: make(map[string]MountState), Tombstones: make(map[string]MountTombstone)})
	return runtime
}

// ActivateMount performs an expected-old compare-and-swap for one mount while
// preserving every other mount in the current configuration generation.
func (runtime *Runtime) ActivateMount(mount string, expectedOld SnapshotID, generation uint64, candidate RuntimeSnapshot) (*RouteTable, error) {
	return runtime.ActivateMountDurably(mount, expectedOld, generation, candidate, nil)
}

func (runtime *Runtime) CheckMount(mount string, expectedOld SnapshotID, generation uint64) error {
	if err := validateRuntimeMount(mount); err != nil {
		return err
	}
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != generation {
		return fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, generation, current.Generation)
	}
	if _, withdrawn := current.Tombstones[mount]; withdrawn {
		return fmt.Errorf("%w: mount %q", ErrMountWithdrawn, mount)
	}
	state, exists := current.Mounts[mount]
	if exists && state.Active.ID != expectedOld {
		return fmt.Errorf("%w: mount %q expected %q, current %q", ErrStaleSnapshot, mount, expectedOld, state.Active.ID)
	}
	if !exists && expectedOld != "" {
		return fmt.Errorf("%w: mount %q has no active snapshot", ErrStaleSnapshot, mount)
	}
	return nil
}

// ActivateMountDurably calls persist with the complete next table while the
// writer lock is held. The in-process pointer changes only after persistence
// succeeds, so a durable coordinator has one non-failing publication step left.
func (runtime *Runtime) ActivateMountDurably(
	mount string,
	expectedOld SnapshotID,
	generation uint64,
	candidate RuntimeSnapshot,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	_, table, err := runtime.activateMountDurably(mount, expectedOld, generation, candidate, persist, true)
	return table, err
}

// ActivateMountDurablyBounded has the same durable ordering as
// ActivateMountDurably, but returns only transition identity. All full-table
// cloning happens before persist returns, which lets a caller place a final
// resource admission inside persist without a later full-table allocation.
func (runtime *Runtime) ActivateMountDurablyBounded(
	mount string,
	expectedOld SnapshotID,
	generation uint64,
	candidate RuntimeSnapshot,
	persist func(*RouteTable) error,
) (MountActivation, error) {
	activation, _, err := runtime.activateMountDurably(mount, expectedOld, generation, candidate, persist, false)
	return activation, err
}

func (runtime *Runtime) activateMountDurably(
	mount string,
	expectedOld SnapshotID,
	generation uint64,
	candidate RuntimeSnapshot,
	persist func(*RouteTable) error,
	cloneResult bool,
) (MountActivation, *RouteTable, error) {
	if err := validateRuntimeMount(mount); err != nil {
		return MountActivation{}, nil, err
	}
	if err := validateRuntimeSnapshot(candidate); err != nil {
		return MountActivation{}, nil, err
	}

	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != generation {
		return MountActivation{}, nil, fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, generation, current.Generation)
	}
	if _, withdrawn := current.Tombstones[mount]; withdrawn {
		return MountActivation{}, nil, fmt.Errorf("%w: mount %q", ErrMountWithdrawn, mount)
	}
	state, exists := current.Mounts[mount]
	if exists {
		if state.Active.ID != expectedOld {
			return MountActivation{}, nil, fmt.Errorf("%w: mount %q expected %q, current %q", ErrStaleSnapshot, mount, expectedOld, state.Active.ID)
		}
		if candidate.ID == state.Active.ID {
			previousID := SnapshotID("")
			if state.Previous != nil {
				previousID = state.Previous.ID
			}
			activation := MountActivation{Generation: current.Generation, ActiveID: state.Active.ID, PreviousID: previousID}
			if cloneResult {
				return activation, cloneRouteTable(current), nil
			}
			return activation, nil, nil
		}
	} else if expectedOld != "" {
		return MountActivation{}, nil, fmt.Errorf("%w: mount %q has no active snapshot", ErrStaleSnapshot, mount)
	}

	next := cloneRouteTable(current)
	nextState := MountState{Active: cloneRuntimeSnapshot(candidate)}
	if exists {
		previous := cloneRuntimeSnapshot(state.Active)
		nextState.Previous = &previous
	}
	next.Mounts[mount] = nextState
	if persist != nil {
		if err := persist(cloneRouteTable(next)); err != nil {
			return MountActivation{}, nil, err
		}
	}
	runtime.table.Store(next)
	previousID := SnapshotID("")
	if nextState.Previous != nil {
		previousID = nextState.Previous.ID
	}
	activation := MountActivation{Generation: next.Generation, ActiveID: nextState.Active.ID, PreviousID: previousID, Changed: true}
	if cloneResult {
		return activation, cloneRouteTable(next), nil
	}
	return activation, nil, nil
}

// ReplaceRoutes installs the complete result of a configuration compilation.
// A stale compiler cannot partially replace a newer route table.
func (runtime *Runtime) ReplaceRoutes(expectedGeneration, nextGeneration uint64, routes map[string]RuntimeSnapshot) error {
	if nextGeneration <= expectedGeneration {
		return fmt.Errorf("%w: next generation %d must exceed %d", ErrStaleGeneration, nextGeneration, expectedGeneration)
	}
	validated := make(map[string]MountState, len(routes))
	for mount, snapshot := range routes {
		if err := validateRuntimeMount(mount); err != nil {
			return err
		}
		if err := validateRuntimeSnapshot(snapshot); err != nil {
			return err
		}
		validated[mount] = MountState{Active: cloneRuntimeSnapshot(snapshot)}
	}

	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != expectedGeneration {
		return fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, expectedGeneration, current.Generation)
	}
	for mount := range validated {
		if _, withdrawn := current.Tombstones[mount]; withdrawn {
			return fmt.Errorf("%w: mount %q requires explicit reauthorization", ErrMountWithdrawn, mount)
		}
	}
	runtime.table.Store(&RouteTable{Generation: nextGeneration, Mounts: validated, Tombstones: cloneTombstones(current.Tombstones)})
	return nil
}

// WithdrawMountDurably removes a serving route and records durable tombstone
// authority in one pointer transition. A failed persist callback leaves both
// the active route and tombstone state unchanged.
func (runtime *Runtime) WithdrawMountDurably(
	mount string,
	expectedActive SnapshotID,
	generation uint64,
	tombstone MountTombstone,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	if err := validateRuntimeMount(mount); err != nil {
		return nil, err
	}
	if err := validateTombstoneState(tombstone.State); err != nil {
		return nil, err
	}

	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != generation {
		return nil, fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, generation, current.Generation)
	}
	state, exists := current.Mounts[mount]
	if !exists {
		if existing, tombstoned := current.Tombstones[mount]; tombstoned && existing == tombstone {
			return cloneRouteTable(current), nil
		}
		if _, tombstoned := current.Tombstones[mount]; tombstoned {
			return nil, fmt.Errorf("%w: mount %q", ErrMountWithdrawn, mount)
		}
		return nil, fmt.Errorf("%w: mount %q has no active snapshot", ErrStaleSnapshot, mount)
	}
	if state.Active.ID != expectedActive {
		return nil, fmt.Errorf("%w: mount %q expected %q, current %q", ErrStaleSnapshot, mount, expectedActive, state.Active.ID)
	}
	if tombstone.SnapshotID == "" {
		tombstone.SnapshotID = state.Active.ID
	}
	if tombstone.CatalogID == "" {
		tombstone.CatalogID = state.Active.Directory.CatalogID
	}
	if err := validateRuntimeTombstone(tombstone); err != nil {
		return nil, err
	}
	if tombstone.SnapshotID != state.Active.ID || tombstone.CatalogID != state.Active.Directory.CatalogID {
		return nil, fmt.Errorf("%w: mount %q identity does not match active snapshot", ErrInvalidTombstone, mount)
	}
	next := cloneRouteTable(current)
	delete(next.Mounts, mount)
	next.Tombstones[mount] = tombstone
	if persist != nil {
		if err := persist(cloneRouteTable(next)); err != nil {
			return nil, err
		}
	}
	runtime.table.Store(next)
	return cloneRouteTable(next), nil
}

// ReauthorizeMountDurably is the only transition that may recreate a
// tombstoned route. It installs a fresh active snapshot and clears tombstone
// authority atomically; it never promotes tombstoned bytes as previous state.
func (runtime *Runtime) ReauthorizeMountDurably(
	mount string,
	generation uint64,
	candidate RuntimeSnapshot,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	if err := validateRuntimeMount(mount); err != nil {
		return nil, err
	}
	if err := validateRuntimeSnapshot(candidate); err != nil {
		return nil, err
	}

	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != generation {
		return nil, fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, generation, current.Generation)
	}
	tombstone, tombstoned := current.Tombstones[mount]
	if !tombstoned {
		return nil, fmt.Errorf("%w: mount %q is not tombstoned", ErrMountUnavailable, mount)
	}
	if _, active := current.Mounts[mount]; active {
		return nil, fmt.Errorf("%w: mount %q still has an active snapshot", ErrStaleSnapshot, mount)
	}
	if tombstone.CatalogID != candidate.Directory.CatalogID {
		return nil, fmt.Errorf("%w: mount %q catalog changed from %q to %q", ErrInvalidTombstone, mount, tombstone.CatalogID, candidate.Directory.CatalogID)
	}
	if tombstone.SnapshotID == candidate.ID {
		return nil, fmt.Errorf("%w: mount %q reuses tombstoned snapshot %q", ErrInvalidTombstone, mount, candidate.ID)
	}
	next := cloneRouteTable(current)
	delete(next.Tombstones, mount)
	next.Mounts[mount] = MountState{Active: cloneRuntimeSnapshot(candidate)}
	if persist != nil {
		if err := persist(cloneRouteTable(next)); err != nil {
			return nil, err
		}
	}
	runtime.table.Store(next)
	return cloneRouteTable(next), nil
}

// FallbackMountDurably removes a corrupt active snapshot and promotes at most
// one verified previous snapshot. It never walks an unbounded history.
func (runtime *Runtime) FallbackMountDurably(
	mount string,
	corrupt SnapshotID,
	generation uint64,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	return runtime.fallbackMountDurably(mount, corrupt, generation, true, persist)
}

// DisableMountDurably removes a corrupt mount without promoting its previous
// generation. Callers use this when the previous snapshot also fails
// immutable preflight; serving an unverified fallback would resurrect bad
// state under a different identity.
func (runtime *Runtime) DisableMountDurably(
	mount string,
	corrupt SnapshotID,
	generation uint64,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	return runtime.fallbackMountDurably(mount, corrupt, generation, false, persist)
}

func (runtime *Runtime) fallbackMountDurably(
	mount string,
	corrupt SnapshotID,
	generation uint64,
	usePrevious bool,
	persist func(*RouteTable) error,
) (*RouteTable, error) {
	if err := validateRuntimeMount(mount); err != nil {
		return nil, err
	}
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	current := runtime.table.Load()
	if current.Generation != generation {
		return nil, fmt.Errorf("%w: expected %d, current %d", ErrStaleGeneration, generation, current.Generation)
	}
	state, exists := current.Mounts[mount]
	if !exists || state.Active.ID != corrupt {
		return nil, fmt.Errorf("%w: mount %q no longer serves %q", ErrStaleSnapshot, mount, corrupt)
	}
	next := cloneRouteTable(current)
	if !usePrevious || state.Previous == nil {
		delete(next.Mounts, mount)
	} else {
		next.Mounts[mount] = MountState{Active: cloneRuntimeSnapshot(*state.Previous)}
	}
	if persist != nil {
		if err := persist(cloneRouteTable(next)); err != nil {
			return nil, err
		}
	}
	runtime.table.Store(next)
	return cloneRouteTable(next), nil
}

func (runtime *Runtime) Table() *RouteTable {
	return cloneRouteTable(runtime.table.Load())
}

func (runtime *Runtime) MountNames() []string {
	current := runtime.table.Load()
	result := make([]string, 0, len(current.Mounts))
	for mount := range current.Mounts {
		result = append(result, mount)
	}
	return result
}

func (runtime *Runtime) HasMount(mount string) bool {
	_, exists := runtime.table.Load().Mounts[mount]
	return exists
}

// RestoreTable installs durable startup state before request admission begins.
func (runtime *Runtime) RestoreTable(table *RouteTable) error {
	if table == nil {
		return fmt.Errorf("%w: route table is nil", ErrInvalidSnapshot)
	}
	for mount, state := range table.Mounts {
		if err := validateRuntimeMount(mount); err != nil {
			return err
		}
		if err := validateRuntimeSnapshot(state.Active); err != nil {
			return err
		}
		if state.Previous != nil {
			if err := validateRuntimeSnapshot(*state.Previous); err != nil {
				return err
			}
		}
		if _, withdrawn := table.Tombstones[mount]; withdrawn {
			return fmt.Errorf("%w: mount %q has active snapshot and tombstone", ErrInvalidTombstone, mount)
		}
	}
	for mount, tombstone := range table.Tombstones {
		if err := validateRuntimeMount(mount); err != nil {
			return err
		}
		if err := validateRuntimeTombstone(tombstone); err != nil {
			return err
		}
	}
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	if len(runtime.refs) != 0 {
		return ErrRuntimeBusy
	}
	runtime.table.Store(cloneRouteTable(table))
	return nil
}

type Admission struct {
	Mount    string
	Snapshot RuntimeSnapshot
	release  func()
	once     sync.Once
}

func (runtime *Runtime) Admit(mount string) (*Admission, error) {
	return runtime.AdmitSnapshot(mount, "")
}

// AdmitSnapshot admits the active snapshot when id is empty, or an explicitly
// retained active/previous snapshot for immutable snapshot-qualified routes.
func (runtime *Runtime) AdmitSnapshot(mount string, id SnapshotID) (*Admission, error) {
	if err := validateRuntimeMount(mount); err != nil {
		return nil, err
	}
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	state, ok := runtime.table.Load().Mounts[mount]
	if !ok {
		if _, withdrawn := runtime.table.Load().Tombstones[mount]; withdrawn {
			return nil, fmt.Errorf("%w: mount %q", ErrMountWithdrawn, mount)
		}
		return nil, fmt.Errorf("%w: %q", ErrMountUnavailable, mount)
	}
	selected := state.Active
	if id != "" && id != state.Active.ID {
		if state.Previous == nil || state.Previous.ID != id {
			return nil, fmt.Errorf("%w: snapshot %q at %q", ErrMountUnavailable, id, mount)
		}
		selected = *state.Previous
	}
	selected = cloneRuntimeSnapshot(selected)
	runtime.refs[selected.ID]++
	// Runtime owns an immutable deep copy from activation. Admissions receive
	// another copy so callers cannot mutate active or previous route state.
	admission := &Admission{Mount: mount, Snapshot: selected}
	admission.release = func() {
		runtime.writes.Lock()
		defer runtime.writes.Unlock()
		if runtime.refs[selected.ID] <= 1 {
			delete(runtime.refs, selected.ID)
			return
		}
		runtime.refs[selected.ID]--
	}
	return admission, nil
}

func (admission *Admission) Release() {
	if admission == nil || admission.release == nil {
		return
	}
	admission.once.Do(admission.release)
}

func (runtime *Runtime) ReferenceCount(snapshot SnapshotID) uint64 {
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	return runtime.refs[snapshot]
}

func validateRuntimeMount(value string) error {
	if value == "/" {
		return nil
	}
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.ContainsAny(value, `\?#`) || path.Clean(value) != value {
		return fmt.Errorf("%w: %q", ErrInvalidMount, value)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: %q contains control characters", ErrInvalidMount, value)
		}
	}
	return nil
}

func validateRuntimeSnapshot(value RuntimeSnapshot) error {
	const prefix = "snapshot-sha256-"
	digest := strings.TrimPrefix(string(value.ID), prefix)
	if !strings.HasPrefix(string(value.ID), prefix) || len(digest) != 64 || value.Location == "" || value.Directory.SchemaVersion != 1 || value.Search.SchemaVersion != 1 {
		return fmt.Errorf("%w: %q", ErrInvalidSnapshot, value.ID)
	}
	for _, character := range digest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return fmt.Errorf("%w: %q", ErrInvalidSnapshot, value.ID)
		}
	}
	return nil
}

func validateTombstoneState(value TombstoneState) error {
	if value != TombstoneWithdrawn && value != TombstoneDeleted {
		return fmt.Errorf("%w: state %q", ErrInvalidTombstone, value)
	}
	return nil
}

func validateRuntimeTombstone(value MountTombstone) error {
	if err := validateTombstoneState(value.State); err != nil {
		return err
	}
	if !strings.HasPrefix(string(value.SnapshotID), "snapshot-sha256-") {
		return fmt.Errorf("%w: snapshot %q", ErrInvalidTombstone, value.SnapshotID)
	}
	if err := validateRuntimeSnapshot(RuntimeSnapshot{ID: value.SnapshotID, Location: "tombstone", Directory: CatalogArtifactV1{SchemaVersion: 1}, Search: SearchDirectoryV1{SchemaVersion: 1}}); err != nil {
		return fmt.Errorf("%w: snapshot %q", ErrInvalidTombstone, value.SnapshotID)
	}
	if err := domain.ValidateCatalogID(value.CatalogID); err != nil {
		return fmt.Errorf("%w: catalog identity: %v", ErrInvalidTombstone, err)
	}
	return nil
}

func cloneRouteTable(source *RouteTable) *RouteTable {
	result := &RouteTable{Generation: source.Generation, Mounts: make(map[string]MountState, len(source.Mounts)), Tombstones: cloneTombstones(source.Tombstones)}
	for mount, state := range source.Mounts {
		cloned := MountState{Active: cloneRuntimeSnapshot(state.Active)}
		if state.Previous != nil {
			previous := cloneRuntimeSnapshot(*state.Previous)
			cloned.Previous = &previous
		}
		result.Mounts[mount] = cloned
	}
	return result
}

func cloneTombstones(source map[string]MountTombstone) map[string]MountTombstone {
	result := make(map[string]MountTombstone, len(source))
	for mount, tombstone := range source {
		result[mount] = tombstone
	}
	return result
}

func cloneRuntimeSnapshot(source RuntimeSnapshot) RuntimeSnapshot {
	result := source
	result.Directory.Documents = append([]DocumentDirectoryV1(nil), source.Directory.Documents...)
	for index := range result.Directory.Documents {
		document := &result.Directory.Documents[index]
		document.SecuritySchemes = append([]SecuritySchemeDirectoryV1(nil), document.SecuritySchemes...)
		document.Overview.Servers = append([]projection.Server(nil), document.Overview.Servers...)
		for serverIndex := range document.Overview.Servers {
			server := &document.Overview.Servers[serverIndex]
			server.Variables = append([]projection.ServerVariable(nil), server.Variables...)
			for variableIndex := range server.Variables {
				server.Variables[variableIndex].Enum = append([]projection.TextRecord(nil), server.Variables[variableIndex].Enum...)
			}
		}
		document.SchemaNodeShards = append([]ShardReferenceV1(nil), document.SchemaNodeShards...)
		document.Operations = append([]OperationDirectoryV1(nil), document.Operations...)
		for operationIndex := range document.Operations {
			operation := &document.Operations[operationIndex]
			operation.Tags = append([]string(nil), operation.Tags...)
			operation.Facets = append([]FacetV1(nil), operation.Facets...)
		}
		document.Schemas = append([]SchemaDirectoryV1(nil), document.Schemas...)
	}
	result.Search.ExactBuckets = append([]SearchExactBucketReferenceV1(nil), source.Search.ExactBuckets...)
	result.Search.TokenRoutes = append([]SearchPostingRouteV1(nil), source.Search.TokenRoutes...)
	result.Search.TrigramRoutes = append([]SearchPostingRouteV1(nil), source.Search.TrigramRoutes...)
	result.Search.PostingSegments = append([]SearchSegmentReferenceV1(nil), source.Search.PostingSegments...)
	result.Search.TrigramSegments = append([]SearchSegmentReferenceV1(nil), source.Search.TrigramSegments...)
	result.Search.RecordSegments = append([]SearchRecordSegmentReferenceV1(nil), source.Search.RecordSegments...)
	result.Search.Ranks = append([]SearchRankRecordV1(nil), source.Search.Ranks...)
	result.Manifest.Identity.Sources = append([]SourceIdentityV1(nil), source.Manifest.Identity.Sources...)
	result.Manifest.Identity.Children = append([]ChildIdentityV1(nil), source.Manifest.Identity.Children...)
	result.Manifest.Children = append([]ChildIdentityV1(nil), source.Manifest.Children...)
	return result
}
