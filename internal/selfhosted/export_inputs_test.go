//go:build !manja_runtime

package selfhosted

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/renderer"
)

func TestExportProjectionLoaderCapturesOnceAndReturnsIndependentBytes(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprintf("corrupt=%t", corrupt), func(t *testing.T) {
			body := []byte(`{"schema":"immutable"}`)
			child := catalog.ChildIdentityV1{Path: "schema-nodes/test.json", Kind: "schema-node", Length: uint64(len(body)), SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
			var captures atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captures.Add(1)
				if corrupt {
					_, _ = w.Write([]byte(`{"schema":"corrupted"}`))
					return
				}
				_, _ = w.Write(body)
			})
			loader := newExportProjectionLoader(context.Background(), handler, renderer.ActivationReceipt{Mount: "/test", SnapshotID: "snapshot"}, catalog.ManifestV1{Children: []catalog.ChildIdentityV1{child}}, catalog.CatalogArtifactV1{}, t.TempDir())
			var workers sync.WaitGroup
			for range 16 {
				workers.Go(func() {
					data, err := loader(child.Path)
					if corrupt {
						if err == nil {
							t.Error("accepted corrupted input")
						}
						return
					}
					if err != nil {
						t.Error(err)
						return
					}
					if string(data) != string(body) {
						t.Errorf("input changed: %q", data)
					}
					data[0] = '!'
				})
			}
			workers.Wait()
			if got := captures.Load(); got != 1 {
				t.Fatalf("captures = %d, want 1", got)
			}
			if _, err := loader("schema-nodes/undeclared.json"); err == nil {
				t.Error("accepted undeclared child")
			}
		})
	}
}
