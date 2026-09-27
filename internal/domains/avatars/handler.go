package avatars

import (
	"context"
	"errors"
	"net/http"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const userIDHeader = "X-User-ID"

type (
	avatarRouter struct {
		avatar ServiceProvider
		log    *zap.Logger
	}

	ServiceProvider interface {
		Upload() error
		SelectById(ctx context.Context, req entity.AvatarReq) (entity.S3AvatarFile, error)
		DeleteById() error
		SelectAvatarMeta(ctx context.Context, id string) (entity.AvatarMetadata, error)
		SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error)
		DeleteCurrent(ctx context.Context, actorID, userID string) error
		SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error)
	}
)

func NewAvatarRouter(group *echo.Group, avatar ServiceProvider, log *zap.Logger) {
	r := avatarRouter{
		avatar: avatar,
		log:    log,
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

func (h *avatarRouter) getAvatarById(c *echo.Context) error {
	avatarID, err := pathAvatarID(c)
	if err != nil || avatarID == "" {
		return err
	}

	var req entity.AvatarReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, apiError{Error: "invalid request"})
	}
	req.AvatarID = avatarID
	if err := req.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, apiError{Error: err.Error()})
	}

	file, err := h.avatar.SelectById(c.Request().Context(), req)
	if err != nil {
		return h.writeServiceErr(c, err)
	}

	c.Response().Header().Set(echo.HeaderCacheControl, "max-age=86400")
	c.Response().Header().Set("ETag", file.ETag)
	if c.Request().Header.Get("If-None-Match") == file.ETag {
		return c.NoContent(http.StatusNotModified)
	}
	// A Stream could have been used, but Blob was chosen because an ETag needs to be provided.
	// The ETag must be calculated in advance—upon upload—and stored in the database, since the
	// header is sent before the body, making it impossible to calculate the hash of the entire file while streaming.
	return c.Blob(http.StatusOK, file.ContentType, file.Body)
}

func (h *avatarRouter) deleteAvatarById(ctx *echo.Context) error {

	return nil
}

func (h *avatarRouter) getAvatarMeta(c *echo.Context) error {
	avatarID, err := pathAvatarID(c)
	if err != nil || avatarID == "" {
		return err
	}

	meta, err := h.avatar.SelectAvatarMeta(c.Request().Context(), avatarID)
	if err != nil {
		return h.writeServiceErr(c, err)
	}
	return c.JSON(http.StatusOK, meta)
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
		return h.writeServiceErr(c, err)
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
		return h.writeServiceErr(c, err)
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
		return h.writeServiceErr(c, err)
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

func pathAvatarID(c *echo.Context) (string, error) {
	avatarID := c.Param("avatar_id")
	if avatarID == "" {
		return "", c.JSON(http.StatusBadRequest, apiError{Error: "avatar_id is required"})
	}
	if _, err := uuid.Parse(avatarID); err != nil {
		return "", c.JSON(http.StatusBadRequest, apiError{Error: "avatar_id must be a uuid"})
	}
	return avatarID, nil
}

func (h *avatarRouter) writeServiceErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, apiError{Error: "Avatar not found"})
	case errors.Is(err, ErrForbidden):
		return c.JSON(http.StatusForbidden, apiError{
			Error:   "Forbidden",
			Details: "You can only delete your own avatars",
		})
	default:
		h.log.Error("request failed",
			zap.String("method", c.Request().Method),
			zap.String("uri", c.Request().URL.RequestURI()),
			zap.Error(err),
		)
		return c.JSON(http.StatusInternalServerError, apiError{Error: "internal error"})
	}
}
