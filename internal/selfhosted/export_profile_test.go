package selfhosted

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	artifact "github.com/araihu/manja/application/htmlartifact"
	"github.com/araihu/manja/internal/adapters/schemacache"
)

// TestExportProfile is an opt-in, fixed-binary export profiling harness. All
// writable paths are supplied explicitly; normal test runs skip the corpus.
func TestExportProfile(t *testing.T) {
	config := os.Getenv("MANJA_PERF_CONFIG")
	if config == "" {
		t.Skip("set MANJA_PERF_CONFIG, MANJA_PERF_DATA, MANJA_PERF_OUTPUT and MANJA_PERF_REPORT")
	}
	data, output, report := os.Getenv("MANJA_PERF_DATA"), os.Getenv("MANJA_PERF_OUTPUT"), os.Getenv("MANJA_PERF_REPORT")
	if data == "" || output == "" || report == "" {
		t.Fatal("explicit data/output/report paths required")
	}
	workers, err := strconv.Atoi(os.Getenv("MANJA_PERF_WORKERS"))
	if err != nil || workers < 1 || workers > 32 {
		t.Fatal("workers must be 1..32")
	}
	ctx := context.Background()
	start := time.Now()
	handler, receipts, err := NewRenderer(ctx, RendererOptions{ConfigPath: config, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range receipts {
		if r.Degraded || r.SnapshotID == "" {
			t.Fatalf("invalid activation: %+v", r)
		}
	}
	ingestion := time.Since(start)
	if os.Getenv("MANJA_PERF_INGEST_ONLY") != "" {
		t.Logf("ingestion=%s", ingestion)
		return
	}
	cpu, err := os.Create(report + ".cpu")
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(cpu); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	budget := uint64(128 << 20)
	if value := os.Getenv("MANJA_PERF_SCHEMA_CACHE_MIB"); value != "" {
		mib, e := strconv.ParseUint(value, 10, 32)
		if e != nil {
			t.Fatal(e)
		}
		budget = mib << 20
	}
	cache := schemacache.New(budget)
	start = time.Now()
	_, err = exportFromHandlerWithCache(ctx, handler, receipts, output, "/", artifact.BuildProfile{}, uint32(workers), cache)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	pprof.StopCPUProfile()
	cpu.Close()
	if err != nil {
		t.Fatal(err)
	}
	heap, err := os.Create(report + ".heap")
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(heap); err != nil {
		t.Fatal(err)
	}
	heap.Close()
	start = time.Now()
	if _, err := VerifyExport(ctx, output); err != nil {
		t.Fatal(err)
	}
	verify := time.Since(start)
	var count, bytes int64
	err = filepath.WalkDir(output, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			count++
			bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"ingestion_seconds": ingestion.Seconds(), "export_seconds": elapsed.Seconds(), "verify_seconds": verify.Seconds(), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs, "files": count, "bytes": bytes, "workers": workers, "schema_cache_budget": budget, "schema_cache": cache.Stats()}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(report+".json", encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
