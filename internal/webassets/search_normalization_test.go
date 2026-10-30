package webassets

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/application/catalog"
)

func TestBrowserQueryNormalizationMatchesCompiler(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	inputs := []string{"FICTÍCIO", "ficti\u0301cio", "Ｎｅｅｄｌｅ", "Straße", "ẞ", "ΟΣ", "ος", "Ꭰꭰ", "İ", "界", "𐐀", "\u00a0Needle\u3000", "\ufeffNeedle", "\u0085Needle", "needle\n", strings.Repeat("İ", 85), strings.Repeat("a", 129), ""}
	type vector struct {
		Input string `json:"input"`
		Exact string `json:"exact"`
		Valid bool   `json:"valid"`
	}
	vectors := make([]vector, 0, len(inputs))
	for _, input := range inputs {
		query, err := catalog.CanonicalizeSearchQuery(input)
		vectors = append(vectors, vector{input, query.String(), err == nil})
	}
	payload, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	const script = `
const fs = require('node:fs'), vm = require('node:vm'), assert = require('node:assert/strict');
const document = {readyState:'complete', addEventListener(){}, querySelectorAll(){return []}};
const window = {addEventListener(){}};
const source = fs.readFileSync('../web/static/catalog-search.js','utf8').replace('window.ManjaCatalogSearchRouter = {','window.normalizeQueryForTest = normalizeExact; window.ManjaCatalogSearchRouter = {');
vm.runInNewContext(source,{window,document,navigator:{},TextEncoder});
for (const entry of JSON.parse(fs.readFileSync(0,'utf8'))) {
 if (entry.valid) assert.equal(window.normalizeQueryForTest(entry.input),entry.exact,entry.input);
 else assert.throws(()=>window.normalizeQueryForTest(entry.input),/Invalid search query/);
}`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "-e", script)
	command.Stdin = bytes.NewReader(payload)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser/compiler normalization differs: %v\n%s", err, output)
	}
}
