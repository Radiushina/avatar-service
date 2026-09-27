package avatars

import (
	"context"

	"github.com/Radiushina/avatar-service/internal/entity"
)

const avatarURLPrefix = "/api/v1/avatars/"

type (
	Service struct {
		repo RepoProvider
	}

	RepoProvider interface {
		Upload() error
		SelectById() error
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

func (s *Service) SelectById() error {

	return nil
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
