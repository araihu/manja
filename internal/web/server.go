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
	public := NewPublicServerWithOptions(idx, opts.Public)
	mux.Handle("/manage", management)
	mux.Handle("/manage/specs", management)
	mux.Handle("/manage/spec/", management)
	mux.Handle("/manage/publication", management)
	mux.Handle("/manage/sync", management)
	mux.Handle("/api/", NewAPIServer())
	mux.Handle("/", publishedDocsPathHandler(public, opts.Management.Store))
	return mux
}

func publishedDocsPathHandler(public http.Handler, store ManagementStore) http.Handler {
	resolverStore, ok := store.(core.Store)
	if !ok {
		return public
	}
	resolver := core.PublicResolver{Store: resolverStore}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := resolver.PublicationByPath(r.Context(), r.URL.Path); err == nil {
				rewritten := r.Clone(r.Context())
				rewritten.URL.Path = "/"
				public.ServeHTTP(w, rewritten)
				return
			}
		}
		public.ServeHTTP(w, r)
	})
}
