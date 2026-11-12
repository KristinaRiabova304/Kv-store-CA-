package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kvstore/internal/shard"
	"kvstore/internal/store"
)

// cluster - coordinator і n шардів у пам'яті, на справжніх локальних HTTP-портах.
// coord - клієнтський порт, internal - порт реєстрації шардів.
type cluster struct {
	t        *testing.T
	coord    *httptest.Server
	internal *httptest.Server
	shards   []*httptest.Server
	stores   []*store.Memory
}

func newCluster(t *testing.T, n int) *cluster { return newClusterExpecting(t, n, 0) }

func newClusterExpecting(t *testing.T, n, expected int) *cluster {
	srv := New(128, expected)
	c := &cluster{t: t, coord: httptest.NewServer(srv.Handler()), internal: httptest.NewServer(srv.InternalHandler())}
	t.Cleanup(c.coord.Close)
	t.Cleanup(c.internal.Close)
	for i := 1; i <= n; i++ {
		c.addShard(fmt.Sprintf("shard-%d", i))
	}
	return c
}

func (c *cluster) addShard(id string) {
	m := store.NewMemory()
	s := httptest.NewServer(shard.NewHandler(m))
	c.t.Cleanup(s.Close)
	c.shards = append(c.shards, s)
	c.stores = append(c.stores, m)
	c.register(id, strings.TrimPrefix(s.URL, "http://"), "boot-"+id)
}

func (c *cluster) register(id, addr, bootID string) {
	c.t.Helper()
	if err := shard.Register(context.Background(), http.DefaultClient, c.internal.URL, id, addr, bootID); err != nil {
		c.t.Fatal(err)
	}
}

func (c *cluster) do(method, path, body string) (int, string) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.coord.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (c *cluster) want(method, path, body string, status int) string {
	c.t.Helper()
	got, resp := c.do(method, path, body)
	if got != status {
		c.t.Fatalf("%s %s: статус %d, чекала %d; тіло %s", method, path, got, status, resp)
	}
	return resp
}

func TestTables(t *testing.T) {
	c := newCluster(t, 0)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"userId"}`, 201)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"other"}`, 409)
	c.want("POST", "/tables", `{"name":"x","partitionKey":"id"}`, 400)
	c.want("POST", "/tables", `{"name":"nopk"}`, 400)
	if body := c.want("GET", "/tables/users", "", 200); !strings.Contains(body, `"partitionKey":"userId"`) {
		t.Fatalf("409 не мав змінити таблицю: %s", body)
	}
	c.want("GET", "/tables/missing", "", 404)
	if body := c.want("GET", "/tables", "", 200); !strings.Contains(body, `"users"`) {
		t.Fatalf("список таблиць: %s", body)
	}
}

// Сценарії CRUD однакові для режиму одного вузла і для двох шардів.
func TestCRUD(t *testing.T) {
	for _, n := range []int{0, 2} {
		t.Run(fmt.Sprintf("%d shards", n), func(t *testing.T) {
			c := newCluster(t, n)
			c.want("POST", "/tables", `{"name":"users","partitionKey":"userId"}`, 201)

			c.want("HEAD", "/tables/users/items/u1", "", 404)
			c.want("PUT", "/tables/users/items/u1", `{"name":"Ann"}`, 200)
			c.want("HEAD", "/tables/users/items/u1", "", 200)
			if body := c.want("GET", "/tables/users/items/u1", "", 200); !strings.Contains(body, `"name":"Ann"`) {
				t.Fatalf("READ: %s", body)
			}
			c.want("PUT", "/tables/users/items/u1", `{"name":"Bob"}`, 200)
			if body := c.want("GET", "/tables/users/items/u1", "", 200); !strings.Contains(body, `"name":"Bob"`) {
				t.Fatalf("повторний CREATE має замінити запис: %s", body)
			}
			c.want("DELETE", "/tables/users/items/u1", "", 204)
			c.want("DELETE", "/tables/users/items/u1", "", 204)
			c.want("HEAD", "/tables/users/items/u1", "", 404)
			c.want("GET", "/tables/users/items/u1", "", 404)

			// помилки
			c.want("PUT", "/tables/users/items/u1", `not json`, 400)
			c.want("PUT", "/tables/users/items/u1", `[1,2]`, 400)
			c.want("PUT", "/tables/users/items/u1/sk", `{}`, 400)
			c.want("PUT", "/tables/nope/items/u1", `{}`, 404)
		})
	}
}

func TestSortKey(t *testing.T) {
	c := newCluster(t, 2)
	c.want("POST", "/tables", `{"name":"orders","partitionKey":"userId","sortKey":"date"}`, 201)
	for _, d := range []string{"2026-10-31", "2026-11-01", "2026-11-15", "2026-11-30", "2026-12-01"} {
		c.want("PUT", "/tables/orders/items/u1/"+d, `{"total":1}`, 200)
	}
	c.want("PUT", "/tables/orders/items/u1", `{}`, 400) // без sort key
	c.want("HEAD", "/tables/orders/items/u1/2026-11-15", "", 200)

	body := c.want("GET", "/tables/orders/items/u1?from=2026-11-01&to=2026-11-30", "", 200)
	var out struct{ Items []store.Item }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range out.Items {
		got = append(got, it.SK)
	}
	if strings.Join(got, ",") != "2026-11-01,2026-11-15,2026-11-30" {
		t.Fatalf("діапазон: %v", got)
	}
	c.want("GET", "/tables/orders/items/u1?limit=0", "", 400)

	// усі записи одного partition key - на одному шарді
	holders := 0
	for _, m := range c.stores {
		if m.Len() > 0 {
			holders++
		}
	}
	if holders != 1 {
		t.Fatalf("записи u1 лежать на %d шардах, мають на одному", holders)
	}
}

func TestShardsGetData(t *testing.T) {
	c := newCluster(t, 2)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"id"}`, 201)
	for i := 0; i < 1000; i++ {
		c.want("PUT", fmt.Sprintf("/tables/users/items/user%d", i), `{"x":1}`, 200)
	}
	a, b := c.stores[0].Len(), c.stores[1].Len()
	t.Logf("shard-1: %d, shard-2: %d", a, b)
	if a+b != 1000 || a < 400 || b < 400 {
		t.Fatalf("розподіл %d / %d", a, b)
	}
	body := c.want("GET", "/internal/distribution", "", 200)
	if !strings.Contains(body, fmt.Sprintf(`"items":%d`, a)) {
		t.Fatalf("distribution: %s", body)
	}
}

func TestReRegistrationKeepsRing(t *testing.T) {
	c := newCluster(t, 2)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"id"}`, 201)
	c.want("PUT", "/tables/users/items/u1", `{"x":1}`, 200)
	c.register("shard-1", strings.TrimPrefix(c.shards[0].URL, "http://"), "boot-shard-1")
	c.want("GET", "/tables/users/items/u1", "", 200)
	body := c.want("GET", "/internal/shards", "", 200)
	if strings.Count(body, `"id"`) != 2 || strings.Contains(body, `"restarts":1`) {
		t.Fatalf("шардів має бути 2 і без перезапусків: %s", body)
	}
}

// Новий bootId у відомого шарда = шард перезапустився і втратив дані (AI-рев'ю M2, пункт 3).
func TestRestartedShardIsCounted(t *testing.T) {
	c := newCluster(t, 2)
	c.register("shard-1", strings.TrimPrefix(c.shards[0].URL, "http://"), "boot-after-restart")
	body := c.want("GET", "/internal/shards", "", 200)
	var out struct{ Shards []ShardInfo }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	for _, si := range out.Shards {
		want := map[string]int{"shard-1": 1, "shard-2": 0}[si.ID]
		if si.Restarts != want || si.LastSeen.IsZero() {
			t.Fatalf("%s: restarts=%d lastSeen=%v, чекала restarts=%d", si.ID, si.Restarts, si.LastSeen, want)
		}
	}
	if out.Shards[0].BootID != "boot-after-restart" {
		t.Fatalf("bootId не оновився: %+v", out.Shards[0])
	}
}

// Поки кільце неповне, записи і читання отримують 503 (AI-рев'ю M2, пункт 2).
func TestNotReadyUntilExpectedShards(t *testing.T) {
	c := newClusterExpecting(t, 1, 2)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"id"}`, 201) // таблиці працюють завжди
	body := c.want("PUT", "/tables/users/items/u1", `{"x":1}`, 503)
	if !strings.Contains(body, "cluster_not_ready") {
		t.Fatalf("тіло: %s", body)
	}
	c.want("GET", "/tables/users/items/u1", "", 503)
	c.want("HEAD", "/tables/users/items/u1", "", 503)
	if c.stores[0].Len() != 0 {
		t.Fatal("у неповне кільце нічого не мало записатися")
	}

	c.addShard("shard-2")
	c.want("PUT", "/tables/users/items/u1", `{"x":1}`, 200)
	c.want("GET", "/tables/users/items/u1", "", 200)
}

// Реєстрація лише на внутрішньому порту (AI-рев'ю M2, пункт 4).
func TestRegistrationNotOnClientPort(t *testing.T) {
	c := newCluster(t, 0)
	c.want("POST", "/internal/shards", `{"id":"evil","addr":"1.2.3.4:9090","bootId":"x"}`, 405)
	if body := c.want("GET", "/internal/shards", "", 200); strings.Contains(body, "evil") {
		t.Fatalf("шард зареєструвався через клієнтський порт: %s", body)
	}
	// без bootId реєстрація не приймається
	resp, err := http.Post(c.internal.URL+"/internal/shards", "application/json", strings.NewReader(`{"id":"s","addr":"1.2.3.4:9090"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("реєстрація без bootId: %d, чекала 400", resp.StatusCode)
	}
}

func TestShardDownGives503(t *testing.T) {
	c := newCluster(t, 1)
	c.want("POST", "/tables", `{"name":"users","partitionKey":"id"}`, 201)
	c.shards[0].Close()
	body := c.want("GET", "/tables/users/items/u1", "", 503)
	if !strings.Contains(body, "shard_unavailable") {
		t.Fatalf("тіло: %s", body)
	}
	c.want("HEAD", "/tables/users/items/u1", "", 503)
}
