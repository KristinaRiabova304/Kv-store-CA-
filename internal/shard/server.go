// Package shard - внутрішній API шарда на порту 9090 і клієнт, яким coordinator до нього звертається.
// Шард не знає схем таблиць: він зберігає записи за (таблиця, partition key, sort key).
// Ключі передаються в query-параметрах, бо sort key може бути порожнім.
package shard

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"kvstore/internal/httpx"
	"kvstore/internal/store"
)

const MaxValueBytes = 400 * 1024

func NewHandler(s store.Store) http.Handler {
	mux := http.NewServeMux()
	httpx.Health(mux)

	mux.HandleFunc("PUT /internal/item", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxValueBytes))
		if err != nil || !json.Valid(body) {
			httpx.Err(w, http.StatusBadRequest, "bad_value", "value must be valid JSON up to 400 KB")
			return
		}
		it := store.Item{PK: q.Get("pk"), SK: q.Get("sk"), Value: body}
		s.Put(q.Get("table"), it)
		httpx.JSON(w, http.StatusOK, it)
	})

	// GET обробляє й HEAD: Go сам не надсилає тіло у відповідь на HEAD.
	mux.HandleFunc("GET /internal/item", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		it, ok := s.Get(q.Get("table"), q.Get("pk"), q.Get("sk"))
		if !ok {
			httpx.Err(w, http.StatusNotFound, "item_not_found", "no item with this key")
			return
		}
		httpx.JSON(w, http.StatusOK, it)
	})

	mux.HandleFunc("DELETE /internal/item", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.Delete(q.Get("table"), q.Get("pk"), q.Get("sk"))
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /internal/query", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		items := s.Query(q.Get("table"), q.Get("pk"), q.Get("from"), q.Get("to"), limit)
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	})

	// скільки записів на шарді: так видно, наскільки рівно кільце розклало дані
	mux.HandleFunc("GET /internal/stats", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]int{"items": s.Len()})
	})
	return mux
}
