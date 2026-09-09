//go:build !manja_runtime

package selfhosted

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/renderer"
)

// Each immutable child is captured once into private build scratch space. Raw
// bytes are not retained in memory; browser admission still verifies every read.
// Independent children can be captured concurrently, while concurrent requests
// for one child wait for the same completed write.
func newExportProjectionLoader(ctx context.Context, handler http.Handler, active renderer.ActivationReceipt, manifest catalog.ManifestV1, directory catalog.CatalogArtifactV1, root string) func(string) ([]byte, error) {
	type input struct {
		identity catalog.ChildIdentityV1
		route    string
		once     sync.Once
		err      error
	}
	inputs := make(map[string]*input)
	for _, child := range manifest.Children {
		if child.Kind != "detail" && child.Kind != "schema-node" {
			continue
		}
		route, _, ok := exportedChildPath(active, directory, child)
		if ok {
			inputs[child.Path] = &input{identity: child, route: route}
		}
	}
	return func(childPath string) ([]byte, error) {
		child, ok := inputs[childPath]
		if !ok {
			return nil, fmt.Errorf("projection child %q is not declared", childPath)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filename := filepath.Join(root, filepath.FromSlash(childPath))
		var first []byte
		child.once.Do(func() {
			captured, err := captureHTTP(ctx, handler, child.route, child.identity.Length, child.identity.SHA256)
			if err == nil {
				err = os.MkdirAll(filepath.Dir(filename), 0700)
			}
			if err == nil {
				err = os.WriteFile(filename, captured.body, 0600)
			}
			child.err = err
			if err == nil {
				first = captured.body
			}
		})
		if child.err != nil {
			return nil, child.err
		}
		if first != nil {
			return first, nil
		}
		return os.ReadFile(filename)
	}
}
