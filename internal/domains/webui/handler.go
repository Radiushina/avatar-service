package webui

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"

	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

const maxAvatarBytes = 10 << 20

type (
	ServiceProvider interface {
		Upload(ctx context.Context, in avatars.UploadInput) (entity.Avatar, error)
		SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error)
	}

	router struct {
		avatar ServiceProvider
		tmpl   *template.Template
		log    *zap.Logger
	}

	uploadPage struct {
		UserID string
		Error  string
	}

	galleryPage struct {
		UserID  string
		Avatars []entity.Avatar
		Error   string
	}
)

func NewRouter(group *echo.Group, avatar ServiceProvider, templatesDir string, log *zap.Logger) error {
	tmpl, err := template.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	r := &router{avatar: avatar, tmpl: tmpl, log: log}

	group.GET("/upload", r.uploadForm)
	group.POST("/upload", r.uploadSubmit)
	group.GET("/gallery/:user_id", r.gallery)
	return nil
}

func (r *router) uploadForm(c *echo.Context) error {
	return r.render(c, http.StatusOK, "upload.html", uploadPage{})
}

func (r *router) uploadSubmit(c *echo.Context) error {
	userID := c.FormValue("user_id")
	page := uploadPage{UserID: userID}

	file, err := c.FormFile("file")
	if err != nil || userID == "" {
		page.Error = "User ID and file are required"
		return r.render(c, http.StatusBadRequest, "upload.html", page)
	}

	src, err := file.Open()
	if err != nil {
		page.Error = "Could not read uploaded file"
		return r.render(c, http.StatusBadRequest, "upload.html", page)
	}
	defer src.Close()

	body, err := io.ReadAll(io.LimitReader(src, maxAvatarBytes+1))
	if err != nil {
		page.Error = "Could not read uploaded file"
		return r.render(c, http.StatusBadRequest, "upload.html", page)
	}
	if len(body) > maxAvatarBytes {
		page.Error = "File too large (max 10MB)"
		return r.render(c, http.StatusRequestEntityTooLarge, "upload.html", page)
	}

	contentType := file.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(body)
	}

	_, err = r.avatar.Upload(c.Request().Context(), avatars.UploadInput{
		UserID:      userID,
		FileName:    file.Filename,
		ContentType: contentType,
		Body:        body,
	})
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, avatars.ErrInvalid):
			status = http.StatusBadRequest
			page.Error = err.Error()
		case errors.Is(err, avatars.ErrTooLarge):
			status = http.StatusRequestEntityTooLarge
			page.Error = "File too large (max 10MB)"
		default:
			r.log.Error("web upload failed", zap.Error(err))
			page.Error = "Upload failed"
		}
		return r.render(c, status, "upload.html", page)
	}

	return c.Redirect(http.StatusSeeOther, "/web/gallery/"+userID)
}

func (r *router) gallery(c *echo.Context) error {
	userID := c.Param("user_id")
	page := galleryPage{UserID: userID}
	if userID == "" {
		page.Error = "user_id is required"
		return r.render(c, http.StatusBadRequest, "gallery.html", page)
	}

	list, err := r.avatar.SelectUserAvatars(c.Request().Context(), userID)
	if err != nil {
		r.log.Error("web gallery failed", zap.Error(err))
		page.Error = "Could not load gallery"
		return r.render(c, http.StatusInternalServerError, "gallery.html", page)
	}
	page.Avatars = list
	return r.render(c, http.StatusOK, "gallery.html", page)
}

func (r *router) render(c *echo.Context, status int, name string, data any) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
	if err := r.tmpl.ExecuteTemplate(c.Response(), name, data); err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	return nil
}
