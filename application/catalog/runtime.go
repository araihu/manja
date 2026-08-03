package catalog

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

var (
	ErrInvalidMount     = errors.New("catalog: invalid mount")
	ErrInvalidSnapshot  = errors.New("catalog: invalid runtime snapshot")
	ErrStaleSnapshot    = errors.New("catalog: stale snapshot")
	ErrStaleGeneration  = errors.New("catalog: stale configuration generation")
	ErrMountUnavailable = errors.New("catalog: mount unavailable")
	ErrRuntimeBusy      = errors.New("catalog: runtime has active admissions")
)

// RuntimeSnapshot is the verified, immutable snapshot admitted for rendering.
// Location names the content-addressed directory that owns Directory and Search.
type RuntimeSnapshot struct {
	ID        SnapshotID
	Location  string
	Directory CatalogArtifactV1
	Search    SearchDirectoryV1
}

type MountState struct {
	Active   RuntimeSnapshot
	Previous *RuntimeSnapshot
}

type RouteTable struct {
	Generation uint64
	Mounts     map[string]MountState
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
	runtime.table.Store(&RouteTable{Generation: generation, Mounts: make(map[string]MountState)})
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
	state, exists := current.Mounts[mount]
	if exists {
		if state.Active.ID != expectedOld {
			return nil, fmt.Errorf("%w: mount %q expected %q, current %q", ErrStaleSnapshot, mount, expectedOld, state.Active.ID)
		}
	} else if expectedOld != "" {
		return nil, fmt.Errorf("%w: mount %q has no active snapshot", ErrStaleSnapshot, mount)
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
			return nil, err
		}
	}
	runtime.table.Store(next)
	return cloneRouteTable(next), nil
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
	runtime.table.Store(&RouteTable{Generation: nextGeneration, Mounts: validated})
	return nil
}

// FallbackMountDurably removes a corrupt active snapshot and promotes at most
// one verified previous snapshot. It never walks an unbounded history.
func (runtime *Runtime) FallbackMountDurably(
	mount string,
	corrupt SnapshotID,
	generation uint64,
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
	if state.Previous == nil {
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
	if err := validateRuntimeMount(mount); err != nil {
		return nil, err
	}
	runtime.writes.Lock()
	defer runtime.writes.Unlock()
	state, ok := runtime.table.Load().Mounts[mount]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrMountUnavailable, mount)
	}
	runtime.refs[state.Active.ID]++
	admission := &Admission{Mount: mount, Snapshot: cloneRuntimeSnapshot(state.Active)}
	admission.release = func() {
		runtime.writes.Lock()
		defer runtime.writes.Unlock()
		if runtime.refs[state.Active.ID] <= 1 {
			delete(runtime.refs, state.Active.ID)
			return
		}
		runtime.refs[state.Active.ID]--
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

func cloneRouteTable(source *RouteTable) *RouteTable {
	result := &RouteTable{Generation: source.Generation, Mounts: make(map[string]MountState, len(source.Mounts))}
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

func cloneRuntimeSnapshot(source RuntimeSnapshot) RuntimeSnapshot {
	result := source
	result.Directory.Documents = append([]DocumentDirectoryV1(nil), source.Directory.Documents...)
	for index := range result.Directory.Documents {
		document := &result.Directory.Documents[index]
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
	return result
}
