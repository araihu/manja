//go:build !manja_runtime

package selfhosted

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestExportProgressEmitsFinalSummary(t *testing.T) {
	var mu sync.Mutex
	events := make([]ExportProgressEvent, 0, 4)
	progress := newExportProgress(context.Background(), func(event ExportProgressEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	progress.setCatalogTotals(1)
	progress.addCatalogTotals(2, 3, 4)
	progress.catalogStarted("pets")
	progress.documentStarted("pets", "petstore")
	progress.phase("render")
	progress.workerStarted()
	progress.detailCompleted(true)
	progress.workerFinished()
	progress.materialized(42)
	progress.documentCompleted()
	progress.catalogCompleted()
	progress.setFinalTotals(7, 99)
	progress.finish(nil)

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 3 {
		t.Fatalf("events = %#v", events)
	}
	start := events[0]
	if start.Event != "start" || start.Status != "running" || start.SchemaVersion != 1 {
		t.Fatalf("start = %#v", start)
	}
	final := events[len(events)-1]
	if final.Event != "complete" || final.Status != "success" || final.Phase != "complete" {
		t.Fatalf("final = %#v", final)
	}
	if final.CatalogsCompleted != 1 || final.CatalogsTotal != 1 || final.DocumentsCompleted != 1 || final.DocumentsTotal != 2 || final.OperationsCompleted != 1 || final.OperationsTotal != 3 || final.SchemasTotal != 4 || final.FragmentsCompleted != 1 || final.FragmentsTotal != 7 || final.Files != 7 || final.Bytes != 99 || final.PeakWorkers != 1 {
		t.Fatalf("final counters = %#v", final)
	}
	var sawRender bool
	for _, event := range events {
		if event.Event == "progress" && event.Phase == "render" {
			sawRender = true
		}
	}
	if !sawRender {
		t.Fatalf("render phase missing: %#v", events)
	}
}

func TestFormatExportProgressIsCIReadable(t *testing.T) {
	line := FormatExportProgress(ExportProgressEvent{Event: "complete", Status: "success", Phase: "complete", ElapsedMS: 1200, Files: 4, Bytes: 99})
	for _, want := range []string{"manja export: progress", "event=complete", "status=success", "phase=complete", "elapsed=1.2s", "files=4", "bytes=99"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
	if strings.ContainsAny(line, "\x1b\r") {
		t.Fatalf("line contains terminal control: %q", line)
	}
}
