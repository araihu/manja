//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/forgejo"
)

func TestForgejoContainerStarts(t *testing.T) {
	ctx := context.Background()
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate(ctx)
}
