package avatars

import (
	"context"
	"errors"
	"net/http"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/labstack/echo/v5"
)

const userIDHeader = "X-User-ID"

type (
	avatarRouter struct {
		avatar ServiceProvider
	}

	ServiceProvider interface {
		Upload() error
		SelectById() error
		DeleteById() error
		SelectAvatarMeta() error
		SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error)
		DeleteCurrent(ctx context.Context, actorID, userID string) error
		SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error)
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

func (h *avatarRouter) getUserAvatar(c *echo.Context) error {
	userID, err := pathUserID(c)
	if err != nil || userID == "" {
		return err
	}
	if c.Request().Header.Get(userIDHeader) == "" {
		return c.JSON(http.StatusBadRequest, apiError{Error: "X-User-ID is required"})
	}

	avatar, err := h.avatar.SelectCurrent(c.Request().Context(), userID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return c.JSON(http.StatusOK, avatar)
}

func (h *avatarRouter) deleteUserAvatar(c *echo.Context) error {
	userID, err := pathUserID(c)
	if err != nil || userID == "" {
		return err
	}
	actorID := c.Request().Header.Get(userIDHeader)
	if actorID == "" {
		return c.JSON(http.StatusBadRequest, apiError{Error: "X-User-ID is required"})
	}

	if err := h.avatar.DeleteCurrent(c.Request().Context(), actorID, userID); err != nil {
		return writeServiceErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *avatarRouter) listUserAvatars(c *echo.Context) error {
	userID, err := pathUserID(c)
	if err != nil || userID == "" {
		return err
	}

	list, err := h.avatar.SelectUserAvatars(c.Request().Context(), userID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

type apiError struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func pathUserID(c *echo.Context) (string, error) {
	userID := c.Param("user_id")
	if userID == "" {
		return "", c.JSON(http.StatusBadRequest, apiError{Error: "user_id is required"})
	}
	return userID, nil
}

func writeServiceErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, apiError{Error: "Avatar not found"})
	case errors.Is(err, ErrForbidden):
		return c.JSON(http.StatusForbidden, apiError{
			Error:   "Forbidden",
			Details: "You can only delete your own avatars",
		})
	default:
		return c.JSON(http.StatusInternalServerError, apiError{Error: "internal error"})
	}
}
