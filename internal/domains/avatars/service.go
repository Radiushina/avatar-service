package avatars

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	avatarURLPrefix = "/api/v1/avatars/"
	maxAvatarBytes  = 10 << 20
)

type (
	Service struct {
		repo      RepoProvider
		publisher Publisher
		log       *zap.Logger
	}

	UploadInput struct {
		UserID      string
		FileName    string
		ContentType string
		Body        []byte
	}

	Publisher interface {
		Publish(ctx context.Context, exchange, routingKey string, event any) error
	}

	Removal struct {
		ID     string
		S3Keys []string
	}

	StoredAvatar struct {
		ID               string
		UserID           string
		S3Key            string
		Thumbnails       map[string]string
		ProcessingStatus entity.ProcessingStatus
		Deleted          bool
	}

	RepoProvider interface {
		Upload(ctx context.Context, opt entity.AvatarOpt, body []byte) (entity.Avatar, error)
		SelectByID(ctx context.Context, id string) (entity.AvatarObject, error)
		GetObject(ctx context.Context, key string) ([]byte, error)
		DeleteByID(ctx context.Context, avatarID, userID string) (Removal, error)
		SelectAvatarMeta(ctx context.Context, id string) (entity.AvatarMetadata, error)
		SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error)
		DeleteCurrent(ctx context.Context, userID string) (Removal, error)
		SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error)
		MarkUploadPublished(ctx context.Context, id string) error
	}
)

func NewService(repo RepoProvider, publisher Publisher, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{
		repo:      repo,
		publisher: publisher,
		log:       log,
	}
}

func (s *Service) Upload(ctx context.Context, in UploadInput) (entity.Avatar, error) {
	if in.UserID == "" {
		return entity.Avatar{}, fmt.Errorf("%w: user_id is required", ErrInvalid)
	}
	if in.FileName == "" {
		return entity.Avatar{}, fmt.Errorf("%w: file name is required", ErrInvalid)
	}
	if len(in.Body) == 0 {
		return entity.Avatar{}, fmt.Errorf("%w: empty file", ErrInvalid)
	}
	if len(in.Body) > maxAvatarBytes {
		return entity.Avatar{}, ErrTooLarge
	}
	mime := normalizeMime(in.ContentType)
	if mime == "" {
		return entity.Avatar{}, fmt.Errorf("%w: supported formats: jpeg, png, webp", ErrInvalid)
	}

	id := uuid.NewString()
	key := fmt.Sprintf("avatars/%s/%s/original", in.UserID, id)
	etag := FileETag(in.Body)
	opt := entity.AvatarOpt{
		ID:       id,
		UserID:   in.UserID,
		FileName: in.FileName,
		MimeType: mime,
		S3Key:    key,
		Etag:     etag,
		Size:     int64(len(in.Body)),
	}
	avatar, err := s.repo.Upload(ctx, opt, in.Body)
	if err != nil {
		return entity.Avatar{}, fmt.Errorf("upload avatar: %w", err)
	}
	if err := s.publishUploadEvent(ctx, avatar.ID, avatar.UserID, key); err != nil {
		// upload_published_at stays NULL; the worker relays the event later.
		s.log.Error("publish upload event", zap.String("avatar_id", avatar.ID), zap.Error(err))
		return withAvatarURL(avatar), nil
	}
	if err := s.repo.MarkUploadPublished(ctx, avatar.ID); err != nil {
		// The event is already queued. A missing timestamp only causes a duplicate publish.
		s.log.Error("mark upload published", zap.String("avatar_id", avatar.ID), zap.Error(err))
	}
	return withAvatarURL(avatar), nil
}

func (s *Service) publishUploadEvent(ctx context.Context, avatarID, userID, s3Key string) error {
	event := broker.AvatarUploadEvent{
		AvatarID: avatarID,
		UserID:   userID,
		S3Key:    s3Key,
	}
	if err := s.publisher.Publish(ctx, broker.Exchange, broker.KeyUploaded, event); err != nil {
		return fmt.Errorf("publish upload event: %w", err)
	}
	return nil
}

func (s *Service) publishDeleteEvent(ctx context.Context, avatarID string, s3Keys []string) error {
	event := broker.AvatarDeleteEvent{
		AvatarID: avatarID,
		S3Keys:   s3Keys,
	}
	if err := s.publisher.Publish(ctx, broker.Exchange, broker.KeyDeleted, event); err != nil {
		return fmt.Errorf("publish delete event: %w", err)
	}
	return nil
}

func normalizeMime(contentType string) string {
	switch contentType {
	case string(entity.ImageJpeg), "image/jpg":
		return string(entity.ImageJpeg)
	case string(entity.ImagePng):
		return string(entity.ImagePng)
	case string(entity.ImageWebp):
		return string(entity.ImageWebp)
	default:
		return ""
	}
}

func (s *Service) SelectByID(ctx context.Context, req entity.AvatarReq) (entity.S3AvatarFile, error) {
	obj, err := s.repo.SelectByID(ctx, req.AvatarID)
	if err != nil {
		return entity.S3AvatarFile{}, fmt.Errorf("load avatar: %w", err)
	}

	key, err := objectKey(obj, req.Size)
	if err != nil {
		return entity.S3AvatarFile{}, err
	}
	if req.Format != "" && mimeForFormat(req.Format) != obj.MimeType {
		return entity.S3AvatarFile{}, ErrNotFound
	}

	contentType := obj.MimeType
	if key != obj.S3Key {
		contentType = string(entity.ImageJpeg)
	}
	return entity.S3AvatarFile{
		ContentType: contentType,
		Key:         key,
		ETag:        etagFor(obj, req.Size),
	}, nil
}

func etagFor(obj entity.AvatarObject, size string) string {
	if size == "" || size == string(entity.ThumbnailOriginal) {
		return obj.ETag
	}
	return obj.ThumbnailETags[size]
}

func (s *Service) Read(ctx context.Context, key string) ([]byte, error) {
	body, err := s.repo.GetObject(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("read avatar file: %w", err)
	}
	return body, nil
}

// objectKey selects the S3 file key for the requested size
func objectKey(obj entity.AvatarObject, size string) (string, error) {
	if size == "" || size == string(entity.ThumbnailOriginal) {
		if obj.S3Key == "" {
			return "", ErrNotFound
		}
		return obj.S3Key, nil
	}
	key := obj.Thumbnails[size]
	if key == "" {
		return "", ErrNotFound
	}
	return key, nil
}

func mimeForFormat(format string) string {
	switch format {
	case "jpeg":
		return string(entity.ImageJpeg)
	case "png":
		return string(entity.ImagePng)
	case "webp":
		return string(entity.ImageWebp)
	default:
		return ""
	}
}

func (s *Service) DeleteByID(ctx context.Context, actorID, avatarID string) error {
	if actorID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalid)
	}
	removed, err := s.repo.DeleteByID(ctx, avatarID, actorID)
	if err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	return s.publishDeleteEvent(ctx, removed.ID, removed.S3Keys)
}

func (s *Service) SelectAvatarMeta(ctx context.Context, id string) (entity.AvatarMetadata, error) {
	meta, err := s.repo.SelectAvatarMeta(ctx, id)
	if err != nil {
		return entity.AvatarMetadata{}, fmt.Errorf("load avatar metadata: %w", err)
	}
	for i := range meta.Thumbnails {
		meta.Thumbnails[i].URL = avatarURLPrefix + meta.ID + "?size=" + string(meta.Thumbnails[i].Size)
	}
	return meta, nil
}

func (s *Service) SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error) {
	avatar, err := s.repo.SelectCurrent(ctx, userID)
	if err != nil {
		return entity.Avatar{}, fmt.Errorf("load current avatar: %w", err)
	}
	return withAvatarURL(avatar), nil
}

func (s *Service) DeleteCurrent(ctx context.Context, actorID, userID string) error {
	if actorID == "" || actorID != userID {
		return ErrForbidden
	}
	removed, err := s.repo.DeleteCurrent(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return s.publishDeleteEvent(ctx, removed.ID, removed.S3Keys)
}

func (s *Service) SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error) {
	list, err := s.repo.SelectUserAvatars(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user avatars: %w", err)
	}
	if list == nil {
		list = []entity.Avatar{}
	}
	for i := range list {
		list[i] = withAvatarURL(list[i])
	}
	return list, nil
}

func withAvatarURL(avatar entity.Avatar) entity.Avatar {
	avatar.URL = avatarURLPrefix + avatar.ID
	return avatar
}

func FileETag(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("\"%x\"", sum[:8])
}
