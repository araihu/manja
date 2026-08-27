package projectionjson

import (
	"context"
	"os"
	"testing"

	"github.com/araihu/manja/application/projection"
)

func BenchmarkMarshalFullProjection(b *testing.B) {
	document, err := (projection.Builder{}).Build(context.Background(), fullFixture())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := Marshal(document); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalFullProjection(b *testing.B) {
	bytes, err := os.ReadFile(fixturePath("v2-full.json"))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := Unmarshal(bytes); err != nil {
			b.Fatal(err)
		}
	}
}
