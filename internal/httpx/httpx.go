// Package httpx - спільні дрібниці для HTTP-відповідей coordinator і шарда.
package httpx

import (
	"encoding/json"
	"net/http"

	"kvstore/internal/buildinfo"
)

// Error - тіло кожної відповіді з помилкою, див. api/openapi.yaml.
type Error struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Err(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, Error{Error: code, Message: msg})
}

// Health додає /health і /version, вони однакові для coordinator і шарда.
func Health(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"version": buildinfo.Version})
	})
}
