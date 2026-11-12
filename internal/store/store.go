// Package store зберігає записи. Його використовують шард і coordinator у режимі одного вузла.
// Див. docs/adr/0011-go-and-in-memory.md.
package store

import (
	"encoding/json"
	"sort"
	"sync"
)

// Item - один запис. SK порожній для таблиць без sort key.
type Item struct {
	PK    string          `json:"pk"`
	SK    string          `json:"sk,omitempty"`
	Value json.RawMessage `json:"value"`
}

// Store - те, що потрібно coordinator від сховища. У M3 з'явиться реалізація на диску.
type Store interface {
	Put(table string, it Item)
	Get(table, pk, sk string) (Item, bool)
	Delete(table, pk, sk string)
	// Query повертає записи одного partition key з sort key у [from, to], відсортовані за зростанням.
	// Порожні from або to означають «без межі».
	Query(table, pk, from, to string, limit int) []Item
	Len() int
}

// partition - усі записи одного partition key: значення за sort key і sort key у порядку зростання.
type partition struct {
	values map[string]json.RawMessage
	keys   []string
}

// Memory зберігає все в пам'яті. RWMutex: багато читань одночасно, запис - по одному.
type Memory struct {
	mu     sync.RWMutex
	tables map[string]map[string]*partition
	count  int
}

func NewMemory() *Memory {
	return &Memory{tables: make(map[string]map[string]*partition)}
}

func (m *Memory) Put(table string, it Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parts, ok := m.tables[table]
	if !ok {
		parts = make(map[string]*partition)
		m.tables[table] = parts
	}
	p, ok := parts[it.PK]
	if !ok {
		p = &partition{values: make(map[string]json.RawMessage)}
		parts[it.PK] = p
	}
	if _, exists := p.values[it.SK]; !exists {
		// вставляю sort key на своє місце, щоб масив лишався відсортованим
		i := sort.SearchStrings(p.keys, it.SK)
		p.keys = append(p.keys, "")
		copy(p.keys[i+1:], p.keys[i:])
		p.keys[i] = it.SK
		m.count++
	}
	p.values[it.SK] = it.Value
}

func (m *Memory) Get(table, pk, sk string) (Item, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p := m.tables[table][pk]
	if p == nil {
		return Item{}, false
	}
	v, ok := p.values[sk]
	if !ok {
		return Item{}, false
	}
	return Item{PK: pk, SK: sk, Value: v}, true
}

func (m *Memory) Delete(table, pk, sk string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.tables[table][pk]
	if p == nil {
		return
	}
	if _, ok := p.values[sk]; !ok {
		return
	}
	delete(p.values, sk)
	i := sort.SearchStrings(p.keys, sk)
	p.keys = append(p.keys[:i], p.keys[i+1:]...)
	m.count--
	if len(p.keys) == 0 {
		delete(m.tables[table], pk)
	}
}

func (m *Memory) Query(table, pk, from, to string, limit int) []Item {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := []Item{}
	p := m.tables[table][pk]
	if p == nil {
		return items
	}
	start := 0
	if from != "" {
		start = sort.SearchStrings(p.keys, from)
	}
	for _, sk := range p.keys[start:] {
		if to != "" && sk > to {
			break
		}
		if limit > 0 && len(items) == limit {
			break
		}
		items = append(items, Item{PK: pk, SK: sk, Value: p.values[sk]})
	}
	return items
}

// Len - кількість записів. Потрібно, щоб показати, скільки ключів на кожному шарді.
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.count
}
