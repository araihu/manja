package web

import (
	"context"
	"encoding/json"
	"net/http"
)

type apiServer struct{}

func NewAPIServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := apiServer{}.GetHealth(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	return mux
}

func (apiServer) GetHealth(context.Context) (Health, error) {
	return Health{Status: Ok}, nil
}
