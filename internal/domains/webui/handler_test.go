package webui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/domains/webui"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubAvatar struct {
	list []entity.Avatar
}

func (s *stubAvatar) Upload(context.Context, avatars.UploadInput) (entity.Avatar, error) {
	return entity.Avatar{}, nil
}

func (s *stubAvatar) SelectUserAvatars(context.Context, string) ([]entity.Avatar, error) {
	return s.list, nil
}

func TestUploadForm(t *testing.T) {
	e := echo.New()
	require.NoError(t, webui.NewRouter(e.Group("/web"), &stubAvatar{}, "../../../web/templates", zap.NewNop()))

	req := httptest.NewRequest(http.MethodGet, "/web/upload", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `action="/web/upload"`)
	require.Contains(t, rec.Body.String(), `name="file"`)
}

func TestGallery(t *testing.T) {
	e := echo.New()
	svc := &stubAvatar{list: []entity.Avatar{{
		ID:     "1ca79251-f6e3-45a4-992f-404f6e4e13ed",
		UserID: "user-123",
		URL:    "/api/v1/avatars/1ca79251-f6e3-45a4-992f-404f6e4e13ed",
		Status: "ready",
	}}}
	require.NoError(t, webui.NewRouter(e.Group("/web"), svc, "../../../web/templates", zap.NewNop()))

	req := httptest.NewRequest(http.MethodGet, "/web/gallery/user-123", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "user-123")
	require.Contains(t, body, "/api/v1/avatars/1ca79251-f6e3-45a4-992f-404f6e4e13ed")
	require.True(t, strings.Contains(body, "ready"))
}
