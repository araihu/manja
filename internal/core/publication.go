package core

import (
	"context"
	"io/fs"
)

type PublicResolver struct {
	Store Store
}

func (r PublicResolver) PublicationByPath(ctx context.Context, publicPath string) (Publication, error) {
	p, err := r.Store.PublicPublicationByPath(ctx, publicPath)
	if err != nil {
		return Publication{}, err
	}
	if !p.VisibleTo(Actor{Anonymous: true}) {
		return Publication{}, fs.ErrNotExist
	}
	return p, nil
}
