package avatars

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/Radiushina/avatar-service/internal/entity"
)

const avatarURLPrefix = "/api/v1/avatars/"

type (
	Service struct {
		repo RepoProvider
	}

	RepoProvider interface {
		Upload() error
		SelectById(ctx context.Context, id string) (entity.AvatarObject, error)
		GetObject(ctx context.Context, key string) ([]byte, error)
		DeleteById() error
		SelectAvatarMeta() error
		SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error)
		DeleteCurrent(ctx context.Context, userID string) error
		SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error)
	}
)

func NewService(repo RepoProvider) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Upload() error {

	return nil
}

func (s *Service) SelectById(ctx context.Context, req entity.AvatarReq) (entity.S3AvatarFile, error) {
	obj, err := s.repo.SelectById(ctx, req.AvatarID)
	if err != nil {
		return entity.S3AvatarFile{}, err
	}

	key, err := objectKey(obj, req.Size)
	if err != nil {
		return entity.S3AvatarFile{}, err
	}
	if req.Format != "" && mimeForFormat(req.Format) != obj.MimeType {
		return entity.S3AvatarFile{}, ErrNotFound
	}

	body, err := s.repo.GetObject(ctx, key)
	if err != nil {
		return entity.S3AvatarFile{}, err
	}

	sum := sha256.Sum256(body)
	return entity.S3AvatarFile{
		ContentType: obj.MimeType,
		Body:        body,
		ETag:        fmt.Sprintf("\"%x\"", sum[:8]),
	}, nil
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

func (s *Service) DeleteById() error {

	return nil
}

func (s *Service) SelectAvatarMeta() error {

	return nil
}

func (s *Service) SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error) {
	avatar, err := s.repo.SelectCurrent(ctx, userID)
	if err != nil {
		return entity.Avatar{}, err
	}
	return withAvatarURL(avatar), nil
}

func (s *Service) DeleteCurrent(ctx context.Context, actorID, userID string) error {
	if actorID == "" || actorID != userID {
		return ErrForbidden
	}
	return s.repo.DeleteCurrent(ctx, userID)
}

func (s *Service) SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error) {
	list, err := s.repo.SelectUserAvatars(ctx, userID)
	if err != nil {
		return nil, err
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
