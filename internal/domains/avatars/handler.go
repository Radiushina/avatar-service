package avatars

import (
	"github.com/labstack/echo/v5"
)

type (
	avatarRouter struct {
		avatar ServiceProvider
	}

	ServiceProvider interface {
		Upload() error
		SelectById() error
		DeleteById() error
		SelectAvatarMeta() error
		SelectCurrent() error
		DeleteCurrent() error
		SelectUserAvatars() error
	}
)

func NewAvatarRouter(group *echo.Group, avatar ServiceProvider) {
	r := avatarRouter{
		avatar: avatar,
	}
	avatars := group.Group("/avatars")
	{
		avatars.POST("", r.uploadFile)
		avatars.GET("/:avatar_id", r.getAvatarById)
		avatars.DELETE("/:avatar_id", r.deleteAvatarById)
		avatars.GET("/:avatar_id/metadata", r.getAvatarMeta)
	}

	users := group.Group("/users")
	{
		users.GET("/:user_id/avatar", r.getUserAvatar)
		users.DELETE("/:user_id/avatar", r.deleteUserAvatar)
		users.GET("/:user_id/avatars", r.listUserAvatars)
	}
}

func (h *avatarRouter) uploadFile(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) getAvatarById(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) deleteAvatarById(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) getAvatarMeta(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) getUserAvatar(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) deleteUserAvatar(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) listUserAvatars(ctx *echo.Context) error {

	return nil
}
