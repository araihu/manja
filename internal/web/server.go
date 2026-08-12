package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
)

type publicDocsExternalPathContextKey struct{}

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
	mux.Handle("/manage/", management)
	mux.Handle("/api/", NewAPIServer())
	mux.Handle("/", publishedDocsPathHandler(public, opts.Management.Store))
	return mux
}

func publishedDocsPathHandler(public http.Handler, store ManagementStore) http.Handler {
	resolverStore, ok := store.(port.PublicationReader)
	if !ok {
		return public
	}
	resolver, err := application.NewPublicResolver(resolverStore)
	if err != nil {
		slog.Error("construct public resolver", "error", err)
		return public
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if publication, err := resolver.PublicationByPath(r.Context(), r.URL.Path); err == nil {
				ctx := context.WithValue(r.Context(), publicDocsExternalPathContextKey{}, publication.Path)
				rewritten := r.Clone(ctx)
				rewritten.URL.Path = "/"
				rewritten.URL.RawPath = ""
				public.ServeHTTP(w, rewritten)
				return
			}
		}
		public.ServeHTTP(w, r)
	})
}

func publicDocsExternalPath(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "/"
	}
	if external, ok := request.Context().Value(publicDocsExternalPathContextKey{}).(string); ok && external != "" {
		return (&url.URL{Path: external}).EscapedPath()
	}
	if path := request.URL.EscapedPath(); path != "" {
		return path
	}
	return "/"
}
