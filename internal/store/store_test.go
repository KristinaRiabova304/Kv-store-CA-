package store

import (
	"encoding/json"
	"fmt"
	"testing"
)

func val(s string) json.RawMessage { return json.RawMessage(fmt.Sprintf(`{"v":%q}`, s)) }

func TestPutGetDelete(t *testing.T) {
	m := NewMemory()
	m.Put("users", Item{PK: "u1", Value: val("a")})

	it, ok := m.Get("users", "u1", "")
	if !ok || string(it.Value) != `{"v":"a"}` {
		t.Fatalf("get = %v %s", ok, it.Value)
	}
	// той самий ключ у іншій таблиці - інший запис
	if _, ok := m.Get("orders", "u1", ""); ok {
		t.Fatal("ключ не має бачитися з іншої таблиці")
	}

	m.Put("users", Item{PK: "u1", Value: val("b")})
	if it, _ := m.Get("users", "u1", ""); string(it.Value) != `{"v":"b"}` {
		t.Fatalf("повторний Put має замінити значення, маємо %s", it.Value)
	}
	if m.Len() != 1 {
		t.Fatalf("Len = %d, чекала 1", m.Len())
	}

	m.Delete("users", "u1", "")
	m.Delete("users", "u1", "") // повторне видалення нічого не ламає
	if _, ok := m.Get("users", "u1", ""); ok {
		t.Fatal("запис мав зникнути")
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d, чекала 0", m.Len())
	}
}

func TestQueryOrderAndBounds(t *testing.T) {
	m := NewMemory()
	for _, sk := range []string{"2026-11-03", "2026-10-30", "2026-11-01", "2026-12-01", "2026-11-30"} {
		m.Put("orders", Item{PK: "u1", SK: sk, Value: val(sk)})
	}
	m.Put("orders", Item{PK: "u2", SK: "2026-11-02", Value: val("other")})

	got := m.Query("orders", "u1", "2026-11-01", "2026-11-30", 0)
	want := []string{"2026-11-01", "2026-11-03", "2026-11-30"}
	if len(got) != len(want) {
		t.Fatalf("маємо %d записів, чекала %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].SK != want[i] || got[i].PK != "u1" {
			t.Fatalf("запис %d: %+v, чекала sk=%s", i, got[i], want[i])
		}
	}

	if got := m.Query("orders", "u1", "", "", 2); len(got) != 2 || got[0].SK != "2026-10-30" {
		t.Fatalf("limit 2 без меж: %v", got)
	}
	if got := m.Query("orders", "nobody", "", "", 0); got == nil || len(got) != 0 {
		t.Fatalf("для порожнього partition key чекала порожній список, маємо %v", got)
	}

	m.Delete("orders", "u1", "2026-11-03")
	if got := m.Query("orders", "u1", "2026-11-01", "2026-11-30", 0); len(got) != 2 {
		t.Fatalf("після видалення чекала 2 записи, маємо %v", got)
	}
}
