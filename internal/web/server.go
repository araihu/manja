package web

import (
	"net/http"

	"github.com/araihu/manja/internal/core"
)

func NewServer(idx core.SpecIndex) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", NewPublicServer(idx))
	mux.Handle("/api/", NewAPIServer())
	return mux
}
