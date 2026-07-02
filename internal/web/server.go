package web

import (
	"net/http"

	"github.com/araihu/manja/internal/core"
)

type Options struct {
	Public     PublicOptions
	Management ManagementOptions
}

func NewServer(idx core.SpecIndex) http.Handler {
	return NewServerWithOptions(idx, Options{})
}

func NewServerWithOptions(idx core.SpecIndex, opts Options) http.Handler {
	mux := http.NewServeMux()
	management := NewManagementServer(idx, opts.Management)
	mux.Handle("/manage", management)
	mux.Handle("/manage/publication", management)
	mux.Handle("/manage/sync", management)
	mux.Handle("/", NewPublicServerWithOptions(idx, opts.Public))
	mux.Handle("/api/", NewAPIServer())
	return mux
}
