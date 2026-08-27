package projectionjson

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func FuzzBuildDeterminism(f *testing.F) {
	for _, seed := range [][]byte{{0}, {1, 2, 3}, []byte("projection"), []byte("__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed []byte) {
		if len(seed) > 1024 {
			t.Skip()
		}
		base := fullFixture()
		wantDocument, err := (projection.Builder{}).Build(context.Background(), base)
		if err != nil {
			t.Fatal(err)
		}
		want, err := Marshal(wantDocument)
		if err != nil {
			t.Fatal(err)
		}

		input := base
		input.Operations = append([]domain.Operation(nil), base.Operations...)
		input.Schemas = append([]domain.Schema(nil), base.Schemas...)
		input.Search = append([]domain.SearchDocument(nil), base.Search...)
		input.PublicRoutes = append([]domain.PublicRoute(nil), base.PublicRoutes...)
		if len(seed) != 0 && seed[0]&1 != 0 {
			reverse(input.Operations)
		}
		if len(seed) > 1 && seed[1]&1 != 0 {
			reverse(input.Schemas)
		}
		if len(seed) > 2 && seed[2]&1 != 0 {
			reverse(input.Search)
		}
		if len(seed) > 3 && seed[3]&1 != 0 {
			reverse(input.PublicRoutes)
		}
		gotDocument, err := (projection.Builder{}).Build(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Marshal(gotDocument)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) || Digest(got) != Digest(want) {
			t.Fatal("equivalent top-level permutation changed bytes or digest")
		}
	})
}

func FuzzUnmarshalCanonicalProjection(f *testing.F) {
	for _, name := range []string{"v2-empty.json", "v2-operation.json", "v2-full.json"} {
		f.Add(readFuzzFixture(f, name))
	}
	for _, seed := range [][]byte{
		[]byte(`{"formatVersion":1,"formatVersion":1}`),
		[]byte(`{"unknown":true}`),
		append(append([]byte(nil), readFuzzFixture(f, "v2-empty.json")...), '\n'),
		{0xff},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 256*1024 {
			t.Skip()
		}
		document, err := Unmarshal(input)
		if err != nil {
			return
		}
		canonical, err := Marshal(document)
		if err != nil {
			t.Fatalf("accepted document cannot marshal: %v", err)
		}
		if !bytes.Equal(canonical, input) {
			t.Fatal("accepted bytes do not round-trip identically")
		}
	})
}

func reverse[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func readFuzzFixture(f *testing.F, name string) []byte {
	f.Helper()
	bytes, err := os.ReadFile(fixturePath(name))
	if err != nil {
		f.Fatal(err)
	}
	return bytes
}
