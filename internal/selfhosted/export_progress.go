//go:build !manja_runtime

package selfhosted

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExportProgressEvent is the stable event schema emitted by an export progress
// reporter. Events are intended for newline-delimited JSON consumers as well
// as human-readable log adapters.
type ExportProgressEvent struct {
	SchemaVersion uint32 `json:"schemaVersion"`
	Event         string `json:"event"`
	Status        string `json:"status"`
	Phase         string `json:"phase"`
	ElapsedMS     int64  `json:"elapsedMs"`

	Catalog  string `json:"catalog,omitempty"`
	Document string `json:"document,omitempty"`

	CatalogsCompleted   uint64 `json:"catalogsCompleted"`
	CatalogsTotal       uint64 `json:"catalogsTotal"`
	DocumentsCompleted  uint64 `json:"documentsCompleted"`
	DocumentsTotal      uint64 `json:"documentsTotal"`
	OperationsCompleted uint64 `json:"operationsCompleted"`
	OperationsTotal     uint64 `json:"operationsTotal"`
	SchemasCompleted    uint64 `json:"schemasCompleted"`
	SchemasTotal        uint64 `json:"schemasTotal"`
	FragmentsCompleted  uint64 `json:"fragmentsCompleted"`
	FragmentsTotal      uint64 `json:"fragmentsTotal"`
	Files               uint64 `json:"files"`
	Bytes               uint64 `json:"bytes"`
	ActiveWorkers       uint64 `json:"activeWorkers"`
	PeakWorkers         uint64 `json:"peakWorkers"`

	Error string `json:"error,omitempty"`
}

// ExportProgress receives periodic export progress events. A nil callback
// disables progress collection and reporting entirely.
type ExportProgress func(ExportProgressEvent)

type exportProgressKey struct{}

const exportProgressInterval = 5 * time.Second

type exportProgress struct {
	emit ExportProgress

	mu         sync.Mutex
	emitMu     sync.Mutex
	started    time.Time
	state      ExportProgressEvent
	stop       chan struct{}
	done       chan struct{}
	finishOnce sync.Once
	wg         sync.WaitGroup
}

func newExportProgress(ctx context.Context, emit ExportProgress) *exportProgress {
	if emit == nil {
		return nil
	}
	p := &exportProgress{
		emit:    emit,
		started: time.Now(),
		state: ExportProgressEvent{
			SchemaVersion: 1,
			Event:         "start",
			Status:        "running",
			Phase:         "load/normalize/compile",
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	p.wg.Add(1)
	go p.run(ctx)
	p.emitSnapshot()
	return p
}

func (p *exportProgress) run(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(exportProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.emitSnapshot()
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func withExportProgress(ctx context.Context, progress *exportProgress) context.Context {
	if progress == nil {
		return ctx
	}
	return context.WithValue(ctx, exportProgressKey{}, progress)
}

func exportProgressFromContext(ctx context.Context) *exportProgress {
	if ctx == nil {
		return nil
	}
	progress, _ := ctx.Value(exportProgressKey{}).(*exportProgress)
	return progress
}

func (p *exportProgress) phase(phase string) {
	if p == nil || strings.TrimSpace(phase) == "" {
		return
	}
	p.mu.Lock()
	if p.state.Phase == phase {
		p.mu.Unlock()
		return
	}
	p.state.Phase = phase
	p.state.Event = "progress"
	p.mu.Unlock()
	p.emitSnapshot()
}

func (p *exportProgress) setCatalogTotals(total int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.CatalogsTotal = uint64(total)
	p.mu.Unlock()
}

func (p *exportProgress) addCatalogTotals(documents, operations, schemas int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.DocumentsTotal += uint64(documents)
	p.state.OperationsTotal += uint64(operations)
	p.state.SchemasTotal += uint64(schemas)
	p.state.FragmentsTotal += uint64(operations + schemas)
	p.mu.Unlock()
}

func (p *exportProgress) catalogStarted(catalog string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.Catalog = catalog
	p.state.Document = ""
	p.mu.Unlock()
}

func (p *exportProgress) catalogCompleted() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.CatalogsCompleted++
	p.mu.Unlock()
}

func (p *exportProgress) documentStarted(catalog, document string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.Catalog = catalog
	p.state.Document = document
	p.mu.Unlock()
}

func (p *exportProgress) documentCompleted() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.DocumentsCompleted++
	p.mu.Unlock()
}

func (p *exportProgress) detailCompleted(operation bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if operation {
		p.state.OperationsCompleted++
	} else {
		p.state.SchemasCompleted++
	}
	p.state.FragmentsCompleted++
	p.mu.Unlock()
}

func (p *exportProgress) materialized(bytes uint64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.Files++
	p.state.Bytes += bytes
	p.mu.Unlock()
}

func (p *exportProgress) workerStarted() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.ActiveWorkers++
	if p.state.ActiveWorkers > p.state.PeakWorkers {
		p.state.PeakWorkers = p.state.ActiveWorkers
	}
	p.mu.Unlock()
}

func (p *exportProgress) workerFinished() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.state.ActiveWorkers > 0 {
		p.state.ActiveWorkers--
	}
	p.mu.Unlock()
}

func (p *exportProgress) setFinalTotals(files, bytes uint64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.state.Files = files
	p.state.Bytes = bytes
	p.mu.Unlock()
}

func (p *exportProgress) emitSnapshot() {
	if p == nil {
		return
	}
	p.mu.Lock()
	event := p.state
	event.ElapsedMS = time.Since(p.started).Milliseconds()
	p.mu.Unlock()
	p.emitMu.Lock()
	p.emit(event)
	p.emitMu.Unlock()
}

func (p *exportProgress) finish(err error) {
	if p == nil {
		return
	}
	p.finishOnce.Do(func() {
		p.mu.Lock()
		if err != nil {
			p.state.Event = "error"
			p.state.Status = "failure"
			p.state.Error = err.Error()
		} else {
			p.state.Event = "complete"
			p.state.Status = "success"
			p.state.Phase = "complete"
		}
		p.mu.Unlock()
		close(p.stop)
		p.wg.Wait()
		p.emitSnapshot()
		close(p.done)
	})
}

// FormatExportProgress formats one event without terminal control sequences,
// so the same line is useful in a TTY and in CI logs.
func FormatExportProgress(event ExportProgressEvent) string {
	parts := []string{
		"manja export: progress",
		"event=" + event.Event,
		"status=" + event.Status,
		"phase=" + event.Phase,
		fmt.Sprintf("elapsed=%s", (time.Duration(event.ElapsedMS) * time.Millisecond).Round(time.Millisecond)),
		fmt.Sprintf("catalogs=%d/%d", event.CatalogsCompleted, event.CatalogsTotal),
		fmt.Sprintf("documents=%d/%d", event.DocumentsCompleted, event.DocumentsTotal),
		fmt.Sprintf("operations=%d/%d", event.OperationsCompleted, event.OperationsTotal),
		fmt.Sprintf("schemas=%d/%d", event.SchemasCompleted, event.SchemasTotal),
		fmt.Sprintf("fragments=%d/%d", event.FragmentsCompleted, event.FragmentsTotal),
		fmt.Sprintf("files=%d", event.Files),
		fmt.Sprintf("bytes=%d", event.Bytes),
	}
	if event.Catalog != "" {
		parts = append(parts, "catalog="+event.Catalog)
	}
	if event.Document != "" {
		parts = append(parts, "document="+event.Document)
	}
	if event.ActiveWorkers != 0 || event.PeakWorkers != 0 {
		parts = append(parts, fmt.Sprintf("workers=%d/%d", event.ActiveWorkers, event.PeakWorkers))
	}
	if event.Error != "" {
		parts = append(parts, "error="+event.Error)
	}
	return strings.Join(parts, " ")
}
