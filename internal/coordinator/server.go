// Package coordinator - вхідна точка для клієнтів. Знає таблиці й шарди і для кожного запиту
// вибирає шард через кільце consistent hashing. Без шардів зберігає дані сам (режим одного вузла).
// Контракт: api/openapi.yaml.
package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"kvstore/internal/httpx"
	"kvstore/internal/ring"
	"kvstore/internal/shard"
	"kvstore/internal/store"
)

type Table struct {
	Name         string `json:"name"`
	PartitionKey string `json:"partitionKey"`
	SortKey      string `json:"sortKey,omitempty"`
}

type ShardInfo struct {
	ID     string `json:"id"`
	Addr   string `json:"addr"`
	BootID string `json:"bootId"`
	// заповнює coordinator
	LastSeen time.Time `json:"lastSeen"`
	Restarts int       `json:"restarts"`
}

// errNotReady - у кільці ще менше шардів, ніж EXPECTED_SHARDS. Див. специфікацію sharding.
var errNotReady = errors.New("cluster not ready")

// backend - куди coordinator кладе запис: віддалений шард або власне сховище.
type backend interface {
	Put(ctx context.Context, table string, it store.Item) error
	Get(ctx context.Context, table, pk, sk string) (store.Item, bool, error)
	Exists(ctx context.Context, table, pk, sk string) (bool, error)
	Delete(ctx context.Context, table, pk, sk string) error
	Query(ctx context.Context, table, pk, from, to string, limit int) ([]store.Item, error)
}

type Server struct {
	mu     sync.RWMutex
	tables map[string]Table
	shards map[string]ShardInfo
	conns  map[string]*shard.Client

	ring     *ring.Ring
	local    localBackend
	hc       *http.Client
	expected int
}

// New: expectedShards = 0 - дозволено режим одного вузла; більше за 0 - записи лише в повне кільце.
func New(vnodes, expectedShards int) *Server {
	return &Server{
		expected: expectedShards,
		tables:   make(map[string]Table),
		shards:   make(map[string]ShardInfo),
		conns:    make(map[string]*shard.Client),
		ring:     ring.New(vnodes),
		local:    localBackend{store.NewMemory()},
		hc:       shard.NewHTTPClient(),
	}
}

// RingKey - що хешується для вибору шарда: таблиця і partition key, без sort key.
// \x00 розділяє частини, щоб ("ab","c") не збігалося з ("a","bc"). Див. ADR-0008.
func RingKey(table, pk string) string { return table + "\x00" + pk }

func (s *Server) backendFor(table, pk string) (backend, error) {
	if s.expected > 0 && s.ring.Size() < s.expected {
		// неповне кільце: запис сюди потім став би недосяжним, див. AI-рев'ю M2, пункт 2
		return nil, errNotReady
	}
	id, ok := s.ring.Owner(RingKey(table, pk))
	if !ok {
		return s.local, nil // жодного шарда: режим одного вузла
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.conns[id], nil
}

// Handler - клієнтський порт 8080: API за OpenAPI і перегляд стану кластера.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	httpx.Health(mux)

	mux.HandleFunc("POST /tables", s.createTable)
	mux.HandleFunc("GET /tables", s.listTables)
	mux.HandleFunc("GET /tables/{table}", s.getTable)

	// GET обробляє й HEAD, тобто EXISTS
	mux.HandleFunc("PUT /tables/{table}/items/{pk}", s.partitionRoute)
	mux.HandleFunc("GET /tables/{table}/items/{pk}", s.partitionRoute)
	mux.HandleFunc("DELETE /tables/{table}/items/{pk}", s.partitionRoute)
	mux.HandleFunc("PUT /tables/{table}/items/{pk}/{sk}", s.sortKeyRoute)
	mux.HandleFunc("GET /tables/{table}/items/{pk}/{sk}", s.sortKeyRoute)
	mux.HandleFunc("DELETE /tables/{table}/items/{pk}/{sk}", s.sortKeyRoute)

	// лише перегляд; POST сюди отримає 405, реєстрація - на внутрішньому порту
	mux.HandleFunc("GET /internal/shards", s.listShards)
	mux.HandleFunc("GET /internal/distribution", s.distribution)
	return mux
}

// InternalHandler - внутрішній порт 8081, його security group відкриває лише для шардів.
// Тут єдина операція, що змінює кільце, тому вона окремо від клієнтського API (AI-рев'ю M2, пункт 4).
func (s *Server) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /internal/shards", s.registerShard)
	return mux
}

// ---------- таблиці ----------

var tableName = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

func (s *Server) createTable(w http.ResponseWriter, r *http.Request) {
	var t Table
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&t); err != nil {
		httpx.Err(w, http.StatusBadRequest, "bad_request", "body must be a JSON table definition")
		return
	}
	if !tableName.MatchString(t.Name) {
		httpx.Err(w, http.StatusBadRequest, "bad_table_name", "name: 3-64 chars of A-Z a-z 0-9 _ -")
		return
	}
	if t.PartitionKey == "" {
		httpx.Err(w, http.StatusBadRequest, "partition_key_required", "partitionKey is required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tables[t.Name]; exists {
		httpx.Err(w, http.StatusConflict, "table_exists", "table "+t.Name+" already exists")
		return
	}
	s.tables[t.Name] = t
	httpx.JSON(w, http.StatusCreated, t)
}

func (s *Server) listTables(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	list := make([]Table, 0, len(s.tables))
	for _, t := range s.tables {
		list = append(list, t)
	}
	s.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	httpx.JSON(w, http.StatusOK, map[string]any{"tables": list})
}

func (s *Server) getTable(w http.ResponseWriter, r *http.Request) {
	if t, ok := s.table(w, r); ok {
		httpx.JSON(w, http.StatusOK, t)
	}
}

// table знаходить таблицю зі шляху або відповідає 404.
func (s *Server) table(w http.ResponseWriter, r *http.Request) (Table, bool) {
	s.mu.RLock()
	t, ok := s.tables[r.PathValue("table")]
	s.mu.RUnlock()
	if !ok {
		fail(w, r, http.StatusNotFound, "table_not_found", "no table "+r.PathValue("table"))
	}
	return t, ok
}

// ---------- записи ----------

// /tables/{table}/items/{pk}: запис таблиці без sort key, або діапазон для таблиці з sort key.
func (s *Server) partitionRoute(w http.ResponseWriter, r *http.Request) {
	t, ok := s.table(w, r)
	if !ok {
		return
	}
	if t.SortKey == "" {
		s.item(w, r, t, r.PathValue("pk"), "")
		return
	}
	if r.Method == http.MethodGet {
		s.query(w, r, t)
		return
	}
	fail(w, r, http.StatusBadRequest, "sort_key_required", "table "+t.Name+" has a sort key: use /items/{pk}/{sk}")
}

// /tables/{table}/items/{pk}/{sk}: лише для таблиць з sort key.
func (s *Server) sortKeyRoute(w http.ResponseWriter, r *http.Request) {
	t, ok := s.table(w, r)
	if !ok {
		return
	}
	if t.SortKey == "" {
		fail(w, r, http.StatusBadRequest, "unexpected_sort_key", "table "+t.Name+" has no sort key: use /items/{pk}")
		return
	}
	s.item(w, r, t, r.PathValue("pk"), r.PathValue("sk"))
}

func (s *Server) item(w http.ResponseWriter, r *http.Request, t Table, pk, sk string) {
	b, err := s.backendFor(t.Name, pk)
	if err != nil {
		backendError(w, r, err)
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, shard.MaxValueBytes))
		if err != nil || !isJSONObject(body) {
			httpx.Err(w, http.StatusBadRequest, "bad_value", "value must be a JSON object up to 400 KB")
			return
		}
		it := store.Item{PK: pk, SK: sk, Value: body}
		if err := b.Put(ctx, t.Name, it); err != nil {
			backendError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, it)

	case http.MethodHead:
		ok, err := b.Exists(ctx, t.Name, pk, sk)
		switch {
		case err != nil:
			backendError(w, r, err)
		case ok:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}

	case http.MethodGet:
		it, ok, err := b.Get(ctx, t.Name, pk, sk)
		switch {
		case err != nil:
			backendError(w, r, err)
		case !ok:
			httpx.Err(w, http.StatusNotFound, "item_not_found", "no item with this key")
		default:
			httpx.JSON(w, http.StatusOK, it)
		}

	case http.MethodDelete:
		if err := b.Delete(ctx, t.Name, pk, sk); err != nil {
			backendError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) query(w http.ResponseWriter, r *http.Request, t Table) {
	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			httpx.Err(w, http.StatusBadRequest, "bad_limit", "limit must be 1-1000")
			return
		}
		limit = n
	}
	pk := r.PathValue("pk")
	b, err := s.backendFor(t.Name, pk)
	if err != nil {
		backendError(w, r, err)
		return
	}
	// усі записи partition key на одному шарді, тому питаю лише його
	items, err := b.Query(r.Context(), t.Name, pk, q.Get("from"), q.Get("to"), limit)
	if err != nil {
		backendError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---------- шарди ----------

func (s *Server) registerShard(w http.ResponseWriter, r *http.Request) {
	var si ShardInfo
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&si); err != nil || si.ID == "" || si.Addr == "" || si.BootID == "" {
		httpx.Err(w, http.StatusBadRequest, "bad_request", `body must be {"id":..., "addr":"host:port", "bootId":...}`)
		return
	}
	s.mu.Lock()
	old, known := s.shards[si.ID]
	si.LastSeen = time.Now().UTC()
	if known {
		si.Restarts = old.Restarts
		if old.BootID != si.BootID {
			// той самий шард, але новий процес: дані в пам'яті зникли (AI-рев'ю M2, пункт 3)
			si.Restarts++
			log.Printf("WARNING shard %s restarted (bootId %s -> %s): its data is lost, reads of its keys will return 404",
				si.ID, old.BootID, si.BootID)
		}
	}
	s.shards[si.ID] = si
	if !known || old.Addr != si.Addr {
		s.conns[si.ID] = shard.NewClient(si.Addr, s.hc)
	}
	s.mu.Unlock()
	if !known && s.ring.Size() == 0 && s.local.s.Len() > 0 {
		log.Printf("WARNING first shard joined, %d items in single-node mode become unreachable (no migration until M5)", s.local.s.Len())
	}
	// у кільце додаю лише після того, як з'єднання вже є, щоб backendFor ніколи не отримав nil
	if s.ring.Add(si.ID) {
		log.Printf("shard %s registered at %s, ring members: %v", si.ID, si.Addr, s.ring.Members())
	} else if known && old.Addr != si.Addr {
		log.Printf("shard %s moved from %s to %s", si.ID, old.Addr, si.Addr)
	}
	httpx.JSON(w, http.StatusOK, si)
}

func (s *Server) shardList() []ShardInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]ShardInfo, 0, len(s.shards))
	for _, si := range s.shards {
		list = append(list, si)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func (s *Server) listShards(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"shards": s.shardList()})
}

// distribution показує, скільки записів на кожному шарді. Для демо і для M5.
func (s *Server) distribution(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID    string `json:"id"`
		Items int    `json:"items"`
		Error string `json:"error,omitempty"`
	}
	out := []row{}
	for _, si := range s.shardList() {
		s.mu.RLock()
		c := s.conns[si.ID]
		s.mu.RUnlock()
		n, err := c.Stats(r.Context())
		rw := row{ID: si.ID, Items: n}
		if err != nil {
			rw.Error = err.Error()
		}
		out = append(out, rw)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"shards": out, "local": s.local.s.Len()})
}

// ---------- допоміжне ----------

func isJSONObject(b []byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) > 0 && b[0] == '{' && json.Valid(b)
}

// fail пише помилку; на HEAD - лише статус, бо тіла в HEAD бути не може.
func fail(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}
	httpx.Err(w, status, code, msg)
}

func backendError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errNotReady) {
		fail(w, r, http.StatusServiceUnavailable, "cluster_not_ready", "not all expected shards have registered yet")
		return
	}
	if errors.Is(err, shard.ErrUnavailable) {
		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
		fail(w, r, http.StatusServiceUnavailable, "shard_unavailable", "the shard owning this key did not answer")
		return
	}
	fail(w, r, http.StatusInternalServerError, "internal", err.Error())
}

// localBackend - власне сховище coordinator для режиму одного вузла.
type localBackend struct{ s store.Store }

func (l localBackend) Put(_ context.Context, table string, it store.Item) error {
	l.s.Put(table, it)
	return nil
}

func (l localBackend) Get(_ context.Context, table, pk, sk string) (store.Item, bool, error) {
	it, ok := l.s.Get(table, pk, sk)
	return it, ok, nil
}

func (l localBackend) Exists(_ context.Context, table, pk, sk string) (bool, error) {
	_, ok := l.s.Get(table, pk, sk)
	return ok, nil
}

func (l localBackend) Delete(_ context.Context, table, pk, sk string) error {
	l.s.Delete(table, pk, sk)
	return nil
}

func (l localBackend) Query(_ context.Context, table, pk, from, to string, limit int) ([]store.Item, error) {
	return l.s.Query(table, pk, from, to, limit), nil
}
