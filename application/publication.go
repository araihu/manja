package application

import (
	"context"
	"io/fs"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type PublicResolver struct {
	publications port.PublicationReader
}

func NewPublicResolver(publications port.PublicationReader) (*PublicResolver, error) {
	if publications == nil {
		return nil, dependencyError("construct public resolver", "publication reader is required")
	}
	return &PublicResolver{publications: publications}, nil
}

func (r *PublicResolver) PublicationByPath(ctx context.Context, publicPath string) (domain.Publication, error) {
	publication, err := r.publications.PublicPublicationByPath(ctx, publicPath)
	if err != nil {
		return domain.Publication{}, err
	}
	if !publication.VisibleTo(domain.Actor{Anonymous: true}) {
		return domain.Publication{}, fs.ErrNotExist
	}
	return publication, nil
}
