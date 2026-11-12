package ring

import (
	"fmt"
	"math/rand"
	"testing"
)

const numKeys = 100_000

// keys - ключі у форматі YCSB, випадкові, але однакові при кожному запуску.
func keys() []string {
	rnd := rand.New(rand.NewSource(42))
	ks := make([]string, numKeys)
	for i := range ks {
		ks[i] = fmt.Sprintf("usertable\x00user%d", rnd.Uint64())
	}
	return ks
}

func owners(r *Ring, ks []string) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i], _ = r.Owner(k)
	}
	return out
}

func TestEmptyRing(t *testing.T) {
	if _, ok := New(0).Owner("x"); ok {
		t.Fatal("порожнє кільце не має повертати власника")
	}
}

func TestSameKeySameShard(t *testing.T) {
	r := New(DefaultVNodes)
	r.Add("shard-1")
	r.Add("shard-2")
	first, _ := r.Owner("users\x00u1")
	for i := 0; i < 100; i++ {
		if o, _ := r.Owner("users\x00u1"); o != first {
			t.Fatalf("той самий ключ пішов на %s, а раніше на %s", o, first)
		}
	}
	if r.Add("shard-1") {
		t.Fatal("повторне додавання шарда має повертати false")
	}
	if o, _ := r.Owner("users\x00u1"); o != first {
		t.Fatal("повторна реєстрація не має змінювати кільце")
	}
}

// Рівномірність: на 2 шарди кожен має отримати від 45% до 55% ключів.
func TestBalance(t *testing.T) {
	r := New(DefaultVNodes)
	r.Add("shard-1")
	r.Add("shard-2")
	counts := map[string]int{}
	for _, o := range owners(r, keys()) {
		counts[o]++
	}
	for id, n := range counts {
		share := float64(n) / numKeys * 100
		t.Logf("%s: %d ключів, %.2f%%", id, n, share)
		if share < 45 || share > 55 {
			t.Errorf("%s отримав %.2f%%, а має бути 45-55%%", id, share)
		}
	}
}

// Переїзд при додаванні третього шарда: consistent hashing проти hash % N.
func TestMovementOnAdd(t *testing.T) {
	ks := keys()
	r := New(DefaultVNodes)
	r.Add("shard-1")
	r.Add("shard-2")
	before := owners(r, ks)
	r.Add("shard-3")
	after := owners(r, ks)

	moved, modMoved := 0, 0
	for i, k := range ks {
		if before[i] != after[i] {
			moved++
			if after[i] != "shard-3" {
				t.Fatalf("ключ переїхав між старими шардами: %s → %s", before[i], after[i])
			}
		}
		h := Hash(k)
		if h%2 != h%3 {
			modMoved++
		}
	}
	share := float64(moved) / numKeys * 100
	modShare := float64(modMoved) / numKeys * 100
	t.Logf("consistent hashing: переїхало %.2f%% ключів; hash %% N: %.2f%%", share, modShare)
	if share > 40 {
		t.Errorf("переїхало %.2f%%, а має бути не більше 40%%", share)
	}
}

func TestRemove(t *testing.T) {
	r := New(DefaultVNodes)
	r.Add("shard-1")
	r.Add("shard-2")
	r.Remove("shard-2")
	for _, o := range owners(r, keys()[:1000]) {
		if o != "shard-1" {
			t.Fatalf("після видалення shard-2 ключ пішов на %s", o)
		}
	}
	if got := r.Members(); len(got) != 1 || got[0] != "shard-1" {
		t.Fatalf("Members = %v", got)
	}
}

func BenchmarkOwner(b *testing.B) {
	r := New(DefaultVNodes)
	r.Add("shard-1")
	r.Add("shard-2")
	ks := keys()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Owner(ks[i%len(ks)])
	}
}
