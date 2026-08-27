package projection

import (
	"context"
	"reflect"
	"testing"
)

func TestBuilderExcludesRawSpecSentinels(t *testing.T) {
	base := minimalIndex()
	base.SpecDownload.Filename = "openapi.json"
	want, err := (Builder{}).Build(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}

	mutated := base
	mutated.SpecDownload.JSON = []byte("__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__")
	mutated.ExampleSpecJSON = "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__"
	got, err := (Builder{}).Build(context.Background(), mutated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("excluded fields changed DTO\ngot:  %#v\nwant: %#v", got, want)
	}

	mutated.SpecDownload.JSON = []byte{0xff}
	mutated.ExampleSpecJSON = string([]byte{0xff})
	got, err = (Builder{}).Build(context.Background(), mutated)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid excluded UTF-8 changed result: %#v, %v", got, err)
	}

	mutated = base
	mutated.SpecDownload.Filename = "different.json"
	got, err = (Builder{}).Build(context.Background(), mutated)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(got, want) {
		t.Fatal("included filename did not change DTO")
	}
}
