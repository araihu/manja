package markdown

import (
	"context"
	"strings"
	"testing"
)

func TestRendererDisablesRawHTMLAndExtractsPlainText(t *testing.T) {
	r := NewRenderer()
	out, err := r.Render(context.Background(), "# Hello\n\n<script>alert(1)</script>\n\nUse `pets`.")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.HTML, "<script>") {
		t.Fatalf("raw HTML was not escaped: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, `class="manja-markdown"`) {
		t.Fatalf("missing wrapper: %s", out.HTML)
	}
	if !strings.Contains(out.Plain, "Hello") || !strings.Contains(out.Plain, "Use pets") {
		t.Fatalf("plain text = %q", out.Plain)
	}
}
