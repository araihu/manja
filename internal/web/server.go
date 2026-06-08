package web

import (
	"net/http"

	"github.com/araihu/manja/internal/core"
)

type Options struct {
	Public PublicOptions
}

func NewServer(idx core.SpecIndex) http.Handler {
	return NewServerWithOptions(idx, Options{})
}

func NewServerWithOptions(idx core.SpecIndex, opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", NewPublicServerWithOptions(idx, opts.Public))
	mux.Handle("/api/", NewAPIServer())
	return mux
}
