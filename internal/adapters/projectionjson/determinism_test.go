package projectionjson

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestProjectionTopLevelPermutationsKeepBytesAndDigest(t *testing.T) {
	base := fullFixture()
	want := mustMarshal(t, mustBuild(t, base))
	for seed := int64(0); seed < 1000; seed++ {
		input := base
		input.Operations = append([]domain.Operation(nil), base.Operations...)
		input.Schemas = append([]domain.Schema(nil), base.Schemas...)
		input.Search = append([]domain.SearchDocument(nil), base.Search...)
		input.PublicRoutes = append([]domain.PublicRoute(nil), base.PublicRoutes...)
		random := rand.New(rand.NewSource(seed))
		random.Shuffle(len(input.Operations), func(i, j int) { input.Operations[i], input.Operations[j] = input.Operations[j], input.Operations[i] })
		random.Shuffle(len(input.Schemas), func(i, j int) { input.Schemas[i], input.Schemas[j] = input.Schemas[j], input.Schemas[i] })
		random.Shuffle(len(input.Search), func(i, j int) { input.Search[i], input.Search[j] = input.Search[j], input.Search[i] })
		random.Shuffle(len(input.PublicRoutes), func(i, j int) {
			input.PublicRoutes[i], input.PublicRoutes[j] = input.PublicRoutes[j], input.PublicRoutes[i]
		})
		document, err := (projection.Builder{}).Build(context.Background(), input)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		got := mustMarshal(t, document)
		if !bytes.Equal(got, want) || Digest(got) != Digest(want) {
			t.Fatalf("seed %d changed bytes/digest", seed)
		}
	}
}
