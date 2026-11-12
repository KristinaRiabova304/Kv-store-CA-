// Coordinator. Налаштування через змінні середовища:
//
//	LISTEN_ADDR      клієнтський порт, за замовчуванням :8080
//	INTERNAL_ADDR    внутрішній порт для реєстрації шардів, за замовчуванням :8081
//	VNODES           віртуальних вузлів на шард, за замовчуванням 128 (ADR-0007)
//	EXPECTED_SHARDS  скільки шардів чекати, перш ніж приймати записи; 0 - режим одного вузла дозволено
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"kvstore/internal/buildinfo"
	"kvstore/internal/coordinator"
	"kvstore/internal/ring"
)

func main() {
	addr := env("LISTEN_ADDR", ":8080")
	internalAddr := env("INTERNAL_ADDR", ":8081")
	vnodes, err := strconv.Atoi(env("VNODES", strconv.Itoa(ring.DefaultVNodes)))
	if err != nil || vnodes < 1 {
		log.Fatalf("VNODES must be a positive number")
	}
	expected, err := strconv.Atoi(env("EXPECTED_SHARDS", "0"))
	if err != nil || expected < 0 {
		log.Fatalf("EXPECTED_SHARDS must be 0 or a positive number")
	}

	c := coordinator.New(vnodes, expected)
	servers := []*http.Server{
		{Addr: addr, Handler: c.Handler(), ReadHeaderTimeout: 5 * time.Second},
		{Addr: internalAddr, Handler: c.InternalHandler(), ReadHeaderTimeout: 5 * time.Second},
	}
	log.Printf("coordinator %s: clients on %s, shard registration on %s, %d vnodes per shard, expecting %d shards",
		buildinfo.Version, addr, internalAddr, vnodes, expected)
	run(servers)
}

// run запускає сервери і коректно зупиняє їх на SIGTERM від systemd або docker.
func run(servers []*http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	for _, srv := range servers {
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal(err)
			}
		}()
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
