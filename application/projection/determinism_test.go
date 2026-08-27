package projection

import (
	"context"
	"math/rand"
	"reflect"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestBuilderTopLevelPermutationDeterminism(t *testing.T) {
	base := minimalIndex()
	base.Operations = []domain.Operation{
		{Anchor: "operation-c", Method: "GET", Path: "/c"},
		{Anchor: "operation-a", Method: "GET", Path: "/a"},
		{Anchor: "operation-b", Method: "GET", Path: "/b"},
	}
	base.Schemas = []domain.Schema{{Name: "C"}, {Name: "A"}, {Name: "B"}}
	base.Search = []domain.SearchDocument{
		{ID: "c", Kind: "operation", Href: "#operation-c"},
		{ID: "a", Kind: "operation", Href: "#operation-a"},
		{ID: "b", Kind: "operation", Href: "#operation-b"},
	}
	base.PublicRoutes = []domain.PublicRoute{{Path: "/c?selected=operation-c#operation-c", Title: "C"}, {Path: "/", Title: "Root"}, {Path: "/a?selected=operation-a#operation-a", Title: "A"}}
	want, err := (Builder{}).Build(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}

	for seed := int64(0); seed < 1000; seed++ {
		input := deepCopySpecIndex(t, base)
		random := rand.New(rand.NewSource(seed))
		random.Shuffle(len(input.Operations), func(i, j int) { input.Operations[i], input.Operations[j] = input.Operations[j], input.Operations[i] })
		random.Shuffle(len(input.Schemas), func(i, j int) { input.Schemas[i], input.Schemas[j] = input.Schemas[j], input.Schemas[i] })
		random.Shuffle(len(input.Search), func(i, j int) { input.Search[i], input.Search[j] = input.Search[j], input.Search[i] })
		random.Shuffle(len(input.PublicRoutes), func(i, j int) {
			input.PublicRoutes[i], input.PublicRoutes[j] = input.PublicRoutes[j], input.PublicRoutes[i]
		})
		before := deepCopySpecIndex(t, input)
		got, err := (Builder{}).Build(context.Background(), input)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d produced different DTO", seed)
		}
		if !reflect.DeepEqual(input, before) {
			t.Fatalf("seed %d mutated input", seed)
		}
	}
}
