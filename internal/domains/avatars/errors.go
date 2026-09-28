package avatars

import "errors"

var (
	ErrNotFound  = errors.New("avatar not found")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid avatar")
	ErrTooLarge  = errors.New("avatar too large")
)
