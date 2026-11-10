package worker_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/Radiushina/avatar-service/internal/worker"
)

func TestRunGoroutinesExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		w := worker.New(&relayRepo{}, nil, nil, listenFunc(func(ctx context.Context, _ string, _ func(context.Context, []byte) error) error {
			<-ctx.Done()
			return ctx.Err()
		}), &relayPublisher{}, nil)

		go func() { _ = w.Run(ctx) }()
		synctest.Wait()
		cancel()
	})
}
