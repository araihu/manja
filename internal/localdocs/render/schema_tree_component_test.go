package render

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSchemaDescriptionMarkdownIsRichAndSafe(t *testing.T) {
	description := "Use **either** `value`.\n\n- [Help](/help)\n- Second item\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert%281%29)"
	var out bytes.Buffer
	if err := schemaDescriptionContent(description).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<strong>either</strong>", "<code>value</code>", "<ul>", `href="/help"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	for _, unsafe := range []string{"<script", "javascript:", " id="} {
		if strings.Contains(out.String(), unsafe) {
			t.Errorf("unsafe or fragment-colliding markup %q", unsafe)
		}
	}
	if schemaDescriptionContent(" \n") != nil {
		t.Fatal("empty description must not create a slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := schemaDescriptionContent("text").Render(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}
