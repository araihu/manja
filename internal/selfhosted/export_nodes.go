//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
	artifactstore "github.com/araihu/manja/internal/adapters/htmlartifact"
	localbrowser "github.com/araihu/manja/internal/localdocs/browser"
	"github.com/araihu/manja/renderer"
	"golang.org/x/net/html"
)

// schemaPanelOrdinals includes root panels and every linked node, so traversal
// covers deep links and cycles without duplicating panels for each schema.
func schemaPanelOrdinals(data []byte) ([]uint32, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var result []uint32
	err = walkExportHTML(doc, func(node *html.Node) error {
		if node.Type != html.ElementNode {
			return nil
		}
		value := htmlAttribute(node, "data-manja-schema-node")
		if node.Data == "a" && hasHTMLAttribute(node, "data-catalog-schema-reference", "true") {
			u, e := url.Parse(htmlAttribute(node, "href"))
			if e != nil {
				return e
			}
			value = u.Query().Get("node")
		}
		if value != "" {
			n, e := strconv.ParseUint(value, 10, 32)
			if e != nil {
				return fmt.Errorf("invalid schema panel ordinal: %w", e)
			}
			result = append(result, uint32(n))
		}
		return nil
	})
	return result, err
}

func emitSchemaNodePanels(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, browser *localbrowser.Browser, active renderer.ActivationReceipt, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, binaryIdentity string, dependencies exportDetailDependencies) error {
	var queue []uint32
	seen := map[uint32]bool{}
	collect := func(data []byte) error {
		ordinals, err := schemaPanelOrdinals(data)
		if err != nil {
			return err
		}
		for _, n := range ordinals {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
		return nil
	}
	for _, schema := range document.Schemas {
		p, _, err := artifact.FragmentLocation(active.Mount, document.Key, artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSchema, Resource: string(schema.DetailID)})
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if err = collect(data); err != nil {
			return err
		}
	}
	for i := 0; i < len(queue); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		ordinal := queue[i]
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSchema, Resource: "node-" + strconv.FormatUint(uint64(ordinal), 10)}
		p, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(struct {
			Document string
			Ordinal  uint32
		}{dependencies.documentSHA256, ordinal})
		key, err := artifact.NewBuildKey(artifact.BuildKeyInput{Fragment: identity, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload), ManjaVersion: binaryIdentity, RendererFingerprint: binaryIdentity, UIFingerprint: binaryIdentity, CompilerIdentity: dependencies.compilerIdentity, NormalizerIdentity: manifest.Identity.SourceManifestSHA256})
		if err != nil {
			return err
		}
		expected := artifact.Expectation{Fragment: identity, BuildKey: key}
		hit := false
		if cacheStore != nil {
			cached, err := cacheStore.VerifyHTML(ctx, p, expected)
			if err != nil {
				return err
			}
			if cached.Hit() {
				if err = linkCachedHTMLArtifact(writer, cacheRoot, p, cached.Manifest); err != nil {
					return err
				}
				hit = true
			}
		}
		if !hit {
			data, err := browser.RenderSchemaNodePanel(ctx, document.Key, ordinal)
			browser.ReleaseChildren()
			if err != nil {
				return fmt.Errorf("render schema node %s/%d: %w", document.Key, ordinal, err)
			}
			data, err = rewriteExportFragmentHTML(data, "/", nil)
			if err != nil {
				return err
			}
			if _, err = store.CommitHTML(ctx, p, expected, func(out io.Writer) error { _, err := out.Write(data); return err }); err != nil {
				return err
			}
			if err = writer.registerExisting(p, "text/html"); err != nil {
				return err
			}
			if err = writer.registerExisting(p+".meta.json", "application/json"); err != nil {
				return err
			}
		}
		data, err := os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if err = collect(data); err != nil {
			return err
		}
	}
	return nil
}
