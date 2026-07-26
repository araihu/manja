package application

import (
	"context"
	"fmt"
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
	if err := domain.ValidateCanonicalPublicPath("public path", publicPath, false); err != nil {
		return domain.Publication{}, validationError("resolve public publication", err.Error())
	}
	publication, err := r.publications.PublicPublicationByPath(ctx, publicPath)
	if err != nil {
		return domain.Publication{}, err
	}
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "publication contract id", value: publication.ProjectID},
		{name: "publication revision id", value: publication.RevisionID},
		{name: "publication hostname", value: publication.Hostname, allowEmpty: true},
	} {
		if err := domain.ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return domain.Publication{}, wrapError(ErrorIntegrity, "validate public publication", err)
		}
	}
	if err := domain.ValidateCanonicalPublicPath("publication path", publication.Path, false); err != nil {
		return domain.Publication{}, wrapError(ErrorIntegrity, "validate public publication", err)
	}
	if publication.Path != publicPath {
		return domain.Publication{}, wrapError(ErrorIntegrity, "validate public publication", fmt.Errorf("returned path does not match requested path"))
	}
	if !publication.VisibleTo(domain.Actor{Anonymous: true}) {
		return domain.Publication{}, fs.ErrNotExist
	}
	return publication, nil
}
