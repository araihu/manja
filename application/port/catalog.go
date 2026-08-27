package port

import (
	"context"

	"github.com/araihu/manja/domain"
)

type CatalogSource interface {
	Load(context.Context) (domain.CatalogCandidate, error)
}
