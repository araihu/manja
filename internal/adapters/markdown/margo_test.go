package markdown

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/net/html"
	"os"
	"strings"
	"testing"
)

func TestMargoDescriptionFragment(t *testing.T) {
	input := "## Details\n\n**Important** and `code`. [Docs](https://docs.github.com/rest). [Local](#details). [Unknown](/docs/missing).\n\n- First\n- Second\n\n| Name | Type |\n| --- | --- |\n| id | string |\n\n```json\n{\"id\": 1}\n```"
	first, err := RenderDescription(context.Background(), input, "operation-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="margo-document"`, "<strong>Important</strong>", "<table", "<ul", `href="https://docs.github.com/rest"`, `data-margo-code-copy`, `data-manja-unresolved-link="/docs/missing"`} {
		if !strings.Contains(first.HTML, want) {
			t.Errorf("missing %q in %s", want, first.HTML)
		}
	}
	for _, bad := range []string{`href="/docs/missing"`, `href="#details"`, `id="details"`, "<script", "<style", "<html"} {
		if strings.Contains(first.HTML, bad) {
			t.Errorf("unexpected %q", bad)
		}
	}
	second, err := RenderDescription(context.Background(), input, "operation-a")
	if err != nil || first != second {
		t.Fatalf("not deterministic: %v", err)
	}
	other, err := RenderDescription(context.Background(), input, "operation-b")
	if err != nil || first.HTML == other.HTML {
		t.Fatalf("scope did not affect heading identities: %v", err)
	}
}

func TestMargoRepeatedCodeBlocksHaveDistinctTargets(t *testing.T) {
	result, err := RenderDescription(context.Background(), "```go\nx := 1\n```\n\n```go\nx := 1\n```", "operation")
	if err != nil {
		t.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(result.HTML))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	var targets []string
	walkDescription(root, func(node *html.Node) {
		for _, attr := range node.Attr {
			if attr.Key == "id" {
				if ids[attr.Val] {
					t.Errorf("duplicate id %s", attr.Val)
				}
				ids[attr.Val] = true
			}
			if attr.Key == "data-code-block-target" {
				targets = append(targets, attr.Val)
			}
		}
	})
	if len(targets) != 2 || targets[0] == targets[1] {
		t.Fatalf("copy targets: %v", targets)
	}
	for _, target := range targets {
		if !ids[target] {
			t.Errorf("missing code target %s", target)
		}
	}
}

func TestMargoDescriptionSafetyAndCancellation(t *testing.T) {
	for _, input := range []string{`<script>alert(1)</script>`, `![image](https://example.com/image.png)`} {
		var output bytes.Buffer
		if err := DescriptionComponent(input, "operation").Render(context.Background(), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `data-manja-markdown-fallback="plain-text"`) || strings.Contains(output.String(), "<script") || strings.Contains(output.String(), "<img") {
			t.Fatal(output.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := DescriptionComponent("hello", "operation").Render(ctx, &output); err != context.Canceled || output.Len() != 0 {
		t.Fatalf("cancellation: %v %s", err, output.String())
	}
}

func TestMargoGitHubOperationDescriptions(t *testing.T) {
	data, err := os.ReadFile("../openapi/testdata/github-v3-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	count := 0
	for path, methods := range spec.Paths {
		for method, data := range methods {
			if !strings.Contains(" get put post delete options head patch trace ", " "+method+" ") {
				continue
			}
			var operation struct {
				Description string `json:"description"`
			}
			if err := json.Unmarshal(data, &operation); err != nil {
				t.Fatal(err)
			}
			if operation.Description == "" {
				continue
			}
			count++
			result, err := RenderDescription(context.Background(), operation.Description, method+" "+path)
			if err != nil {
				t.Errorf("%s %s: %v", method, path, err)
				continue
			}
			if result.Plain == "" {
				t.Errorf("empty rendered description: %s %s", method, path)
			}
		}
	}
	if count != 529 {
		t.Fatalf("GitHub corpus changed: %d descriptions, want 529", count)
	}
}
