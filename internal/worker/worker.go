package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	maxAttempts   = 5
	relayInterval = 10 * time.Second

	thumbSmall  = 100
	thumbMedium = 300
)

type (
	AvatarRepo interface {
		GetAvatar(ctx context.Context, id string) (avatars.StoredAvatar, error)
		UpdateProcessingStatus(ctx context.Context, id string, status entity.ProcessingStatus, thumbs map[string]string, etags map[string]string) error
		ListUploadsToPublish(ctx context.Context) ([]avatars.PendingUpload, error)
		MarkUploadPublished(ctx context.Context, id string) error
	}

	Publisher interface {
		Publish(ctx context.Context, exchange, routingKey string, event any) error
	}

	ObjectStore interface {
		Get(ctx context.Context, key string) ([]byte, error)
		Put(ctx context.Context, key, contentType string, body []byte) error
		Delete(ctx context.Context, key string) error
	}

	Consumer interface {
		Listen(ctx context.Context, queue string, handle func(context.Context, []byte) error) error
	}

	Worker struct {
		repo      AvatarRepo
		objects   ObjectStore
		resizer   Resizer
		mq        Consumer
		publisher Publisher
		log       *zap.Logger
		backoff   func(attempt int) time.Duration
	}
)

func New(repo AvatarRepo, objects ObjectStore, resizer Resizer, mq Consumer, publisher Publisher, log *zap.Logger) *Worker {
	if log == nil {
		log = zap.NewNop()
	}
	return &Worker{
		repo:      repo,
		objects:   objects,
		resizer:   resizer,
		mq:        mq,
		publisher: publisher,
		log:       log,
		backoff:   defaultBackoff,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("avatar worker started")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return w.mq.Listen(ctx, broker.KeyUploaded, w.HandleUploadEvent)
	})
	g.Go(func() error {
		return w.mq.Listen(ctx, broker.KeyDeleted, w.HandleDeleteEvent)
	})
	if w.publisher != nil {
		g.Go(func() error {
			w.relayUploads(ctx)
			return nil
		})
	}
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// HandleUploadEvent creates 100x100 and 300x300 thumbnails for an uploaded avatar.
func (w *Worker) HandleUploadEvent(ctx context.Context, body []byte) error {
	var event broker.AvatarUploadEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return permanent(fmt.Errorf("decode upload event: %w", err))
	}
	return retry(ctx, w.backoff, func() error {
		return w.processUpload(ctx, event)
	})
}

func (w *Worker) processUpload(ctx context.Context, event broker.AvatarUploadEvent) error {
	avatar, err := w.repo.GetAvatar(ctx, event.AvatarID)
	if err != nil {
		if errors.Is(err, avatars.ErrNotFound) {
			return permanent(err)
		}
		return fmt.Errorf("load avatar: %w", err)
	}
	if avatar.Deleted || avatar.ProcessingStatus == entity.Completed {
		return nil
	}

	image, err := w.objects.Get(ctx, event.S3Key)
	if err != nil {
		if errors.Is(err, avatars.ErrNotFound) {
			return permanent(err)
		}
		return fmt.Errorf("download avatar: %w", err)
	}

	small, err := w.resizer.Resize(image, thumbSmall, thumbSmall)
	if err != nil {
		return fmt.Errorf("resize thumbnail: %w", err)
	}
	medium, err := w.resizer.Resize(image, thumbMedium, thumbMedium)
	if err != nil {
		return fmt.Errorf("resize thumbnail: %w", err)
	}
	thumbnails := []struct {
		size string
		data []byte
	}{
		{string(entity.ThumbnailSmol), small},
		{string(entity.ThumbnailMedium), medium},
	}

	keys := make(map[string]string, len(thumbnails))
	etags := make(map[string]string, len(thumbnails))
	for _, thumb := range thumbnails {
		key := fmt.Sprintf("thumbnails/%s/%s.jpg", event.AvatarID, thumb.size)
		if err := w.objects.Put(ctx, key, string(entity.ImageJpeg), thumb.data); err != nil {
			return fmt.Errorf("upload thumbnail: %w", err)
		}
		keys[thumb.size] = key
		etags[thumb.size] = avatars.FileETag(thumb.data)
	}
	if err := w.repo.UpdateProcessingStatus(ctx, event.AvatarID, entity.Completed, keys, etags); err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}
	w.log.Info("avatar processed", zap.String("avatar_id", event.AvatarID))
	return nil
}

// HandleDeleteEvent removes avatar objects from object storage. Missing keys are success.
func (w *Worker) HandleDeleteEvent(ctx context.Context, body []byte) error {
	var event broker.AvatarDeleteEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return permanent(fmt.Errorf("decode delete event: %w", err))
	}
	return retry(ctx, w.backoff, func() error {
		for _, key := range event.S3Keys {
			if key == "" {
				continue
			}
			if err := w.objects.Delete(ctx, key); err != nil {
				return fmt.Errorf("delete object: %w", err)
			}
		}
		return nil
	})
}

func (w *Worker) relayUploads(ctx context.Context) {
	ticker := time.NewTicker(relayInterval)
	defer ticker.Stop()
	w.publishPending(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.publishPending(ctx)
		}
	}
}

func (w *Worker) publishPending(ctx context.Context) {
	rows, err := w.repo.ListUploadsToPublish(ctx)
	if err != nil {
		w.log.Error("list uploads to publish", zap.Error(err))
		return
	}
	for _, row := range rows {
		event := broker.AvatarUploadEvent{
			AvatarID: row.ID,
			UserID:   row.UserID,
			S3Key:    row.S3Key,
		}
		if err := w.publisher.Publish(ctx, broker.Exchange, broker.KeyUploaded, event); err != nil {
			w.log.Error("relay upload event", zap.String("avatar_id", row.ID), zap.Error(err))
			continue
		}
		// Bump the timestamp so a still-processing row is not republished on every tick.
		if err := w.repo.MarkUploadPublished(ctx, row.ID); err != nil {
			w.log.Error("mark upload published", zap.String("avatar_id", row.ID), zap.Error(err))
		}
	}
}

func retry(ctx context.Context, backoff func(int) time.Duration, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = fn()
		if err == nil || isPermanent(err) || errors.Is(err, context.Canceled) {
			return err
		}
		if attempt == maxAttempts-1 {
			break
		}
		timer := time.NewTimer(backoff(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return err
}

func defaultBackoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 4 {
		attempt = 4
	}
	return time.Second * time.Duration(1<<attempt)
}

type permanentError struct{ error }

func permanent(err error) error { return permanentError{err} }

func isPermanent(err error) bool {
	var target permanentError
	return errors.As(err, &target)
}
