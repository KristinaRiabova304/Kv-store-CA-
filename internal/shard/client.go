package shard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"kvstore/internal/store"
)

// ErrUnavailable - шард не відповів або відповів не так, як очікувалося. Coordinator перетворює це на 503.
var ErrUnavailable = errors.New("shard unavailable")

// Timeout - скільки coordinator чекає на шард, див. специфікацію api.
const Timeout = 2 * time.Second

type Client struct {
	base string
	http *http.Client
}

// NewClient: addr у форматі host:port. Усі клієнти ділять один http.Client,
// щоб з'єднання з шардами перевикористовувалися.
func NewClient(addr string, hc *http.Client) *Client {
	return &Client{base: "http://" + addr, http: hc}
}

func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   Timeout,
		Transport: &http.Transport{MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second},
	}
}

func itemQuery(table, pk, sk string) string {
	return url.Values{"table": {table}, "pk": {pk}, "sk": {sk}}.Encode()
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp, nil
}

func unexpected(resp *http.Response) error {
	return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
}

func (c *Client) Put(ctx context.Context, table string, it store.Item) error {
	resp, err := c.do(ctx, http.MethodPut, "/internal/item?"+itemQuery(table, it.PK, it.SK), it.Value)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return unexpected(resp)
	}
	return nil
}

func (c *Client) Get(ctx context.Context, table, pk, sk string) (store.Item, bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/internal/item?"+itemQuery(table, pk, sk), nil)
	if err != nil {
		return store.Item{}, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var it store.Item
		if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
			return store.Item{}, false, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return it, true, nil
	case http.StatusNotFound:
		return store.Item{}, false, nil
	default:
		return store.Item{}, false, unexpected(resp)
	}
}

func (c *Client) Exists(ctx context.Context, table, pk, sk string) (bool, error) {
	resp, err := c.do(ctx, http.MethodHead, "/internal/item?"+itemQuery(table, pk, sk), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, unexpected(resp)
	}
}

func (c *Client) Delete(ctx context.Context, table, pk, sk string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/internal/item?"+itemQuery(table, pk, sk), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return unexpected(resp)
	}
	return nil
}

func (c *Client) Query(ctx context.Context, table, pk, from, to string, limit int) ([]store.Item, error) {
	q := url.Values{"table": {table}, "pk": {pk}, "from": {from}, "to": {to}, "limit": {strconv.Itoa(limit)}}
	resp, err := c.do(ctx, http.MethodGet, "/internal/query?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, unexpected(resp)
	}
	var out struct {
		Items []store.Item `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return out.Items, nil
}

// Stats - скільки записів зараз на шарді.
func (c *Client) Stats(ctx context.Context) (int, error) {
	resp, err := c.do(ctx, http.MethodGet, "/internal/stats", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, unexpected(resp)
	}
	var out struct {
		Items int `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return out.Items, nil
}

// Register - шард повідомляє coordinator про себе. Викликається при старті й далі кожні 10 секунд.
// bootID - випадковий ID процесу шарда: за ним coordinator бачить, що шард перезапустився.
func Register(ctx context.Context, hc *http.Client, coordinatorURL, id, addr, bootID string) error {
	body, _ := json.Marshal(map[string]string{"id": id, "addr": addr, "bootId": bootID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coordinatorURL+"/internal/shards", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration rejected: status %d", resp.StatusCode)
	}
	return nil
}
