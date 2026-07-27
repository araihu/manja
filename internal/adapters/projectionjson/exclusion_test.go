package projectionjson

import (
	"bytes"
	"context"
	"testing"

	"github.com/araihu/manja/application/projection"
)

func TestProjectionExcludedFieldsDoNotAffectBytesOrDigest(t *testing.T) {
	base := fullFixture()
	base.SpecDownload.JSON = nil
	base.ExampleSpecJSON = ""
	want := mustMarshal(t, mustBuild(t, base))

	mutated := base
	mutated.SpecDownload.JSON = []byte("__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__")
	mutated.ExampleSpecJSON = "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__"
	got := mustMarshal(t, mustBuild(t, mutated))
	if !bytes.Equal(got, want) || Digest(got) != Digest(want) {
		t.Fatal("excluded source fields changed bytes or digest")
	}
	for _, sentinel := range [][]byte{mutated.SpecDownload.JSON, []byte(mutated.ExampleSpecJSON)} {
		if bytes.Contains(got, sentinel) {
			t.Fatalf("projection contains excluded sentinel")
		}
	}

	mutated.SpecDownload.Filename = "different.json"
	changed, err := (projection.Builder{}).Build(context.Background(), mutated)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(mustMarshal(t, changed), want) {
		t.Fatal("included filename did not change bytes")
	}
}
