package port

import (
	"context"

	"github.com/araihu/manja/domain"
)

type CatalogParser interface {
	Parse(context.Context, domain.CatalogCandidate) (domain.CatalogIndex, error)
}
