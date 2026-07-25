package openapi

import (
	"context"
	"fmt"

	"github.com/araihu/manja/internal/core"
)

// SnapshotBuilder builds normalized contract snapshots from OpenAPI files.
type SnapshotBuilder struct {
	Parser Parser
}

func (b SnapshotBuilder) Build(
	ctx context.Context,
	contractID string,
	file core.SpecFile,
	rev core.Revision,
) (core.ContractSnapshot, error) {
	parser := b.Parser
	if parser == (Parser{}) {
		parser = Parser{}
	}
	idx, err := parser.Parse(ctx, file, rev)
	if err != nil {
		return core.ContractSnapshot{}, fmt.Errorf("build contract snapshot: %w", err)
	}
	return core.NewContractSnapshot(contractID, rev.ID, file.Bytes, idx), nil
}
