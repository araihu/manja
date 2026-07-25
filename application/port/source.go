package port

import (
	"context"

	"github.com/araihu/manja/domain"
)

type SourceFetcher interface {
	Fetch(context.Context) (domain.SpecFile, domain.ContractRevision, error)
}

type SourceDiscoverer interface {
	Discover(context.Context) ([]domain.RevisionCandidate, error)
}
