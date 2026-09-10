package render

import (
	"bytes"
	"context"
	"errors"
	"html"
	"io"
	"strings"
	"testing"
)

func TestSchemaDescriptionRemainsPlainText(t *testing.T) {
	description := "Use **either** `value`.\n\n- [Help](/docs/identity/verification-checks)\n- Second item\n\n![Image](https://example.com/image.png)\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert%281%29)"
	var out bytes.Buffer
	if err := schemaDescriptionContent(description, "test").Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	plain := strings.TrimSuffix(strings.TrimPrefix(out.String(), `<p class="whitespace-pre-wrap">`), `</p>`)
	if html.UnescapeString(plain) != description {
		t.Fatalf("description text changed: %s", out.String())
	}
	for _, unsafe := range []string{"<script", "<a ", "<img", "<strong", "<code", "<ul", " id="} {
		if strings.Contains(out.String(), unsafe) {
			t.Errorf("unsafe or fragment-colliding markup %q", unsafe)
		}
	}
	if schemaDescriptionContent(" \n", "test") != nil {
		t.Fatal("empty description must not create a slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := schemaDescriptionContent("text", "test").Render(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}
