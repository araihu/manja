package cache

import (
	"context"
	"testing"
)

func TestMemoryCacheCopiesValues(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()
	in := []byte("abc")
	if err := c.Set(ctx, "k", in); err != nil {
		t.Fatal(err)
	}
	in[0] = 'z'
	got, ok, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(got) != "abc" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got[0] = 'x'
	got, _, err = c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Fatalf("cache leaked mutable slice: %q", got)
	}
}
