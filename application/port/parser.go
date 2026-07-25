package port

import (
	"context"

	"github.com/araihu/manja/domain"
)

type Parser interface {
	Parse(context.Context, domain.SpecFile, domain.ContractRevision) (domain.SpecIndex, error)
}

type ContractSnapshotBuilder interface {
	Build(context.Context, string, domain.SpecFile, domain.ContractRevision) (domain.ContractSnapshot, error)
}

type ReviewInputLoader interface {
	Load(context.Context, string, domain.ReviewInputLocator) (domain.SpecFile, domain.ContractRevision, error)
}
