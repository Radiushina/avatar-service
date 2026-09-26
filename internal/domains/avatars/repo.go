package avatars

import (
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db      *pgxpool.Pool
	builder goqu.DialectWrapper
}

func NewAvatarRepo(db *pgxpool.Pool) *Repo {
	return &Repo{
		db:      db,
		builder: goqu.Dialect("postgres"),
	}
}

var (
	usersTable = goqu.T("avatars")
)

func (r *Repo) Upload() error {

	return nil
}

func (r *Repo) SelectById() error {

	return nil
}

func (r *Repo) DeleteById() error {

	return nil
}

func (r *Repo) SelectAvatarMeta() error {

	return nil
}

func (r *Repo) SelectCurrent() error {

	return nil
}

func (r *Repo) DeleteCurrent() error {

	return nil
}

func (r *Repo) SelectUserAvatars() error {

	return nil
}
