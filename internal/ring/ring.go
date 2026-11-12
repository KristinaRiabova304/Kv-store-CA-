// Package ring - кільце consistent hashing з віртуальними вузлами.
// Хеш xxHash64, кожен шард ставить на кільце VNodes точок. Див. docs/adr/0007-hash-and-vnodes.md.
package ring

import (
	"sort"
	"strconv"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const DefaultVNodes = 128

type point struct {
	hash  uint64
	owner string
}

// Ring безпечне для використання з кількох горутин.
type Ring struct {
	mu      sync.RWMutex
	vnodes  int
	points  []point // відсортовані за hash
	members map[string]bool
}

func New(vnodes int) *Ring {
	if vnodes <= 0 {
		vnodes = DefaultVNodes
	}
	return &Ring{vnodes: vnodes, members: make(map[string]bool)}
}

// Hash - хеш, яким і ключі, і віртуальні вузли кладуться на кільце.
func Hash(s string) uint64 { return xxhash.Sum64String(s) }

// Add додає шард. Повертає false, якщо він уже був: тоді кільце не змінюється.
func (r *Ring) Add(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members[id] {
		return false
	}
	r.members[id] = true
	for i := 0; i < r.vnodes; i++ {
		r.points = append(r.points, point{hash: Hash(id + "#" + strconv.Itoa(i)), owner: id})
	}
	sort.Slice(r.points, func(a, b int) bool { return r.points[a].hash < r.points[b].hash })
	return true
}

// Remove прибирає шард і всі його віртуальні вузли.
func (r *Ring) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.members[id] {
		return false
	}
	delete(r.members, id)
	kept := r.points[:0]
	for _, p := range r.points {
		if p.owner != id {
			kept = append(kept, p)
		}
	}
	r.points = kept
	return true
}

// Owner повертає шард, якому належить ключ: перший віртуальний вузол, чий хеш не менший
// за хеш ключа. Якщо такого немає, кільце замикається і береться перший вузол.
// false - кільце порожнє.
func (r *Ring) Owner(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 {
		return "", false
	}
	h := Hash(key)
	i := sort.Search(len(r.points), func(i int) bool { return r.points[i].hash >= h })
	if i == len(r.points) {
		i = 0
	}
	return r.points[i].owner, true
}

// Size - кількість шардів у кільці. Дешевше за Members, викликається на кожен запит.
func (r *Ring) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}

// Members - шарди в кільці, відсортовані за id.
func (r *Ring) Members() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.members))
	for id := range r.members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
