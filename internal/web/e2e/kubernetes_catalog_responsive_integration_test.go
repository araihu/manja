//go:build integration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/araihu/manja/internal/selfhosted"
	"github.com/mxschmitt/playwright-go"
)

func TestKubernetesRichOperationResponseSchemaStaysWithinPrimaryViewport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration e2e test in short mode")
	}
	chdirRepoRoot(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := filepath.Join("internal", "renderer", "testdata", "kubernetes")
	handler, _, err := selfhosted.NewRenderer(ctx, selfhosted.RendererOptions{
		ConfigPath: filepath.Join(fixture, "renderer.yaml"),
		DataDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	searchResponse, err := server.Client().Get(server.URL + "/search.json?q=listCoreV1NamespacedPod")
	if err != nil {
		t.Fatal(err)
	}
	defer searchResponse.Body.Close()
	var search struct {
		Results []struct {
			Href string `json:"href"`
		} `json:"results"`
	}
	if err := json.NewDecoder(searchResponse.Body).Decode(&search); err != nil {
		t.Fatal(err)
	}
	if len(search.Results) == 0 || search.Results[0].Href == "" {
		t.Fatalf("listCoreV1NamespacedPod search results = %#v", search.Results)
	}

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server.URL+search.Results[0].Href, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`.manja-endpoint-responses-section .manja-schema-tree`).First().WaitFor(); err != nil {
		t.Fatalf("Kubernetes response schema tree: %v", err)
	}

	for _, width := range []int{390, 768} {
		t.Run(fmt.Sprintf("%dpx", width), func(t *testing.T) {
			if err := page.SetViewportSize(width, 900); err != nil {
				t.Fatal(err)
			}
			result, err := page.Evaluate(`() => {
				const main = document.querySelector('[data-manja-primary-scroll]');
				const response = document.querySelector('.manja-endpoint-responses-section');
				const trees = Array.from(response?.querySelectorAll('.manja-schema-tree') || []);
				const descriptions = trees.flatMap((tree) => Array.from(tree.querySelectorAll('.manja-schema-description')));
				return {
					mainClientWidth: main?.clientWidth || 0,
					mainScrollWidth: main?.scrollWidth || 0,
					responseSchemaTrees: trees.length,
					visibleSchemaRows: trees.reduce((total, tree) => total + tree.querySelectorAll('.manja-schema-row').length, 0),
					responseSchemaTextLength: trees.reduce((total, tree) => total + String(tree.textContent || '').trim().length, 0),
					schemaDescriptions: descriptions.length,
					maxDescriptionOverflow: descriptions.reduce((maximum, description) => {
						return Math.max(maximum, description.scrollWidth - description.clientWidth);
					}, 0),
				};
			}`)
			if err != nil {
				t.Fatal(err)
			}
			metrics, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("%dpx Kubernetes response metrics should be a map, got %#v", width, result)
			}
			t.Logf("%dpx Kubernetes response schema: main=%v/%v trees=%v rows=%v text=%v descriptions=%v max-description-overflow=%v", width, metrics["mainScrollWidth"], metrics["mainClientWidth"], metrics["responseSchemaTrees"], metrics["visibleSchemaRows"], metrics["responseSchemaTextLength"], metrics["schemaDescriptions"], metrics["maxDescriptionOverflow"])
			if metricNumber(t, metrics, "responseSchemaTrees") < 1 || metricNumber(t, metrics, "visibleSchemaRows") < 1 || metricNumber(t, metrics, "responseSchemaTextLength") < 100 {
				t.Fatalf("%dpx response schema content must remain available, metrics %#v", width, metrics)
			}
			if got := metricNumber(t, metrics, "maxDescriptionOverflow"); got != 0 {
				t.Fatalf("%dpx schema descriptions overflow their local width by %v pixels, metrics %#v", width, got, metrics)
			}
			if got, want := metricNumber(t, metrics, "mainScrollWidth"), metricNumber(t, metrics, "mainClientWidth"); got != want {
				t.Fatalf("%dpx primary viewport owns horizontal overflow: scrollWidth=%v clientWidth=%v, metrics %#v", width, got, want, metrics)
			}
		})
	}
}
