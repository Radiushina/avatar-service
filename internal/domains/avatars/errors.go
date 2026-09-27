package avatars

import "errors"

var (
	ErrNotFound  = errors.New("avatar not found")
	ErrForbidden = errors.New("forbidden")
)
