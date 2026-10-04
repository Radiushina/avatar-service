package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/Radiushina/avatar-service/internal/worker"
	"github.com/stretchr/testify/require"
)

func TestPublishPending(t *testing.T) {
	t.Parallel()

	pending := []avatars.PendingUpload{{
		ID:     "id-1",
		UserID: "user-1",
		S3Key:  "avatars/original",
	}}
	tests := []struct {
		name       string
		pubErr     error
		wantIDs    []string
		wantMarked []string
	}{
		{
			name:       "marks after publish",
			wantIDs:    []string{"id-1"},
			wantMarked: []string{"id-1"},
		},
		{
			name:    "publish error skips mark",
			pubErr:  errors.New("down"),
			wantIDs: []string{"id-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &relayRepo{pending: pending}
			pub := &relayPublisher{err: tt.pubErr}
			ctx, cancel := context.WithCancel(context.Background())
			w := worker.New(repo, nil, nil, listenFunc(func(ctx context.Context, _ string, _ func(context.Context, []byte) error) error {
				<-ctx.Done()
				return ctx.Err()
			}), pub, nil)

			done := make(chan error, 1)
			go func() { done <- w.Run(ctx) }()

			require.Eventually(t, func() bool {
				return len(pub.ids()) == len(tt.wantIDs) && len(repo.markedIDs()) == len(tt.wantMarked)
			}, time.Second, 5*time.Millisecond)

			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not return")
			}
			require.Equal(t, tt.wantIDs, pub.ids())
			require.Equal(t, tt.wantMarked, repo.markedIDs())
		})
	}
}

type relayRepo struct {
	mu      sync.Mutex
	pending []avatars.PendingUpload
	marked  []string
}

func (r *relayRepo) GetAvatar(context.Context, string) (avatars.StoredAvatar, error) {
	return avatars.StoredAvatar{}, nil
}

func (r *relayRepo) UpdateProcessingStatus(context.Context, string, entity.ProcessingStatus, map[string]string, map[string]string) error {
	return nil
}

func (r *relayRepo) ListUploadsToPublish(context.Context) ([]avatars.PendingUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]avatars.PendingUpload(nil), r.pending...), nil
}

func (r *relayRepo) MarkUploadPublished(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marked = append(r.marked, id)
	return nil
}

func (r *relayRepo) markedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.marked...)
}

type relayPublisher struct {
	mu  sync.Mutex
	err error
	got []string
}

func (p *relayPublisher) Publish(_ context.Context, _, _ string, event any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := event.(broker.AvatarUploadEvent)
	p.got = append(p.got, e.AvatarID)
	return p.err
}

func (p *relayPublisher) ids() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.got...)
}
