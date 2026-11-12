// Шард. Налаштування через змінні середовища:
//
//	SHARD_ID         ім'я шарда в кільці, наприклад shard-1 (обов'язково)
//	COORDINATOR_URL  внутрішній порт coordinator, наприклад http://10.20.0.15:8081 (обов'язково)
//	ADVERTISE_ADDR   host:port, за яким coordinator дістане шард (обов'язково)
//	LISTEN_ADDR      де слухати, за замовчуванням :9090
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kvstore/internal/buildinfo"
	"kvstore/internal/shard"
	"kvstore/internal/store"
)

// RegisterEvery - як часто шард повторює реєстрацію. Так coordinator після перезапуску
// знову бачить усі шарди не пізніше ніж за 10 секунд.
const RegisterEvery = 10 * time.Second

func main() {
	id := must("SHARD_ID")
	coordinatorURL := must("COORDINATOR_URL")
	advertise := must("ADVERTISE_ADDR")
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":9090"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           shard.NewHandler(store.NewMemory()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	// новий bootId при кожному старті: так coordinator відрізняє «той самий шард» від «шард, що втратив дані»
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	bootID := hex.EncodeToString(b)
	log.Printf("shard %s %s listening on %s, advertised as %s, bootId %s", id, buildinfo.Version, addr, advertise, bootID)

	go registerLoop(ctx, coordinatorURL, id, advertise, bootID)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func registerLoop(ctx context.Context, coordinatorURL, id, advertise, bootID string) {
	hc := &http.Client{Timeout: shard.Timeout}
	registered := false
	t := time.NewTicker(RegisterEvery)
	defer t.Stop()
	for {
		err := shard.Register(ctx, hc, coordinatorURL, id, advertise, bootID)
		switch {
		case err != nil:
			// coordinator може ще стартувати: пробую знову на наступному тіку
			log.Printf("registration with %s failed: %v", coordinatorURL, err)
			registered = false
		case !registered:
			log.Printf("registered with %s", coordinatorURL)
			registered = true
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}
