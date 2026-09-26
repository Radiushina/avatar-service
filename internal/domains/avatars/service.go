package avatars

type (
	Service struct {
		repo RepoProvider
	}

	RepoProvider interface {
		Upload() error
		SelectById() error
		DeleteById() error
		SelectAvatarMeta() error
		SelectCurrent() error
		DeleteCurrent() error
		SelectUserAvatars() error
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

func (s *Service) SelectCurrent() error {

	return nil
}

func (s *Service) DeleteCurrent() error {

	return nil
}

func (s *Service) SelectUserAvatars() error {

	return nil
}
