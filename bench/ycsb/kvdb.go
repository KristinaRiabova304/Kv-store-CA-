package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/magiconair/properties"
	"github.com/pingcap/go-ycsb/pkg/prop"
	"github.com/pingcap/go-ycsb/pkg/ycsb"
)

// Драйвер YCSB для kv-store. Кожна операція YCSB - один HTTP-запит до coordinator:
//
//	insert, update → PUT    /tables/{table}/items/{key}
//	read           → GET    /tables/{table}/items/{key}
//	delete         → DELETE /tables/{table}/items/{key}
//	scan           → не підтримується: діапазони є лише в межах одного partition key, а ключі YCSB різні
//
// Мій PUT замінює запис повністю, тому в сценаріях стоїть writeallfields=true:
// update пише всі поля, і заміна дає той самий результат, що й оновлення.
type kvCreator struct{}

type kvDB struct {
	base string
	hc   *http.Client
}

func (kvCreator) Create(p *properties.Properties) (ycsb.DB, error) {
	threads := p.GetInt(prop.ThreadCount, 1)
	db := &kvDB{
		base: p.GetString("kvstore.url", "http://127.0.0.1:8080"),
		hc: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{MaxIdleConns: threads * 2, MaxIdleConnsPerHost: threads * 2},
		},
	}
	// таблицю YCSB (за замовчуванням usertable) реєструю сам; 409 означає, що вона вже є
	table := p.GetString(prop.TableName, prop.TableNameDefault)
	body := fmt.Sprintf(`{"name":%q,"partitionKey":"ycsb_key"}`, table)
	resp, err := db.hc.Post(db.base+"/tables", "application/json", bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("coordinator %s: %w", db.base, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return nil, fmt.Errorf("create table %s: status %d", table, resp.StatusCode)
	}
	return db, nil
}

func (db *kvDB) itemURL(table, key string) string {
	return db.base + "/tables/" + url.PathEscape(table) + "/items/" + url.PathEscape(key)
}

func (db *kvDB) do(ctx context.Context, method, u string, body []byte, want int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := db.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body) // читаю до кінця, щоб з'єднання повернулося в пул
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != want {
		return nil, fmt.Errorf("%s %s: status %d", method, u, resp.StatusCode)
	}
	return b, nil
}

func (db *kvDB) put(ctx context.Context, table, key string, values map[string][]byte) error {
	// значення полів YCSB - байти; у JSON кладу їх рядками
	obj := make(map[string]string, len(values))
	for k, v := range values {
		obj[k] = string(v)
	}
	body, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = db.do(ctx, http.MethodPut, db.itemURL(table, key), body, http.StatusOK)
	return err
}

func (db *kvDB) Insert(ctx context.Context, table, key string, values map[string][]byte) error {
	return db.put(ctx, table, key, values)
}

func (db *kvDB) Update(ctx context.Context, table, key string, values map[string][]byte) error {
	return db.put(ctx, table, key, values)
}

func (db *kvDB) Read(ctx context.Context, table, key string, fields []string) (map[string][]byte, error) {
	b, err := db.do(ctx, http.MethodGet, db.itemURL(table, key), nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var it struct {
		Value map[string]string `json:"value"`
	}
	if err := json.Unmarshal(b, &it); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(it.Value))
	for k, v := range it.Value {
		out[k] = []byte(v)
	}
	return out, nil
}

func (db *kvDB) Delete(ctx context.Context, table, key string) error {
	_, err := db.do(ctx, http.MethodDelete, db.itemURL(table, key), nil, http.StatusNoContent)
	return err
}

func (db *kvDB) Scan(ctx context.Context, table, startKey string, count int, fields []string) ([]map[string][]byte, error) {
	return nil, errors.New("scan is not supported, see bench/ycsb/kvdb.go")
}

func (db *kvDB) InitThread(ctx context.Context, _ int, _ int) context.Context { return ctx }
func (db *kvDB) CleanupThread(_ context.Context)                              {}
func (db *kvDB) Close() error                                                 { return nil }
