package providers

import (
	"context"
	"net"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestCollectRuntimeMetricsExits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prev := collectSample
		t.Cleanup(func() { collectSample = prev })
		collectSample = func(context.Context, *pgxpool.Pool) error { return nil }

		ctx, cancel := context.WithCancel(context.Background())
		go CollectRuntimeMetrics(ctx, nil, nil)
		synctest.Wait()
		cancel()
	})
}

func TestListenGoroutineExits(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	srv := NewServers(&http.Server{
		Addr:              addr,
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not listen: %v", dialErr)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listen goroutine did not exit")
	}
}
