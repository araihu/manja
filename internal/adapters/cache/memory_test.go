package cache

import "testing"

func TestMemoryCacheCopiesValues(t *testing.T) {
	c := NewMemory()
	in := []byte("abc")
	c.Set("k", in)
	in[0] = 'z'
	got, ok := c.Get("k")
	if !ok || string(got) != "abc" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got[0] = 'x'
	got, _ = c.Get("k")
	if string(got) != "abc" {
		t.Fatalf("cache leaked mutable slice: %q", got)
	}
}
