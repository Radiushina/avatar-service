package avatars_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const userIDHeader = "X-User-ID"

func TestUserAvatarRoutes(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	current := entity.Avatar{
		ID:        "1ca79251-f6e3-45a4-992f-404f6e4e13ed",
		UserID:    "user-123",
		Status:    "ready",
		CreatedAt: created,
	}

	t.Run("current avatar", func(t *testing.T) {
		t.Parallel()

		repo := &stubRepo{current: current}
		rec := doRequest(t, repo, http.MethodGet, "/api/v1/users/user-123/avatar", "user-123")

		require.Equal(t, http.StatusOK, rec.Code)
		var got entity.Avatar
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, "/api/v1/avatars/"+current.ID, got.URL)
		require.Equal(t, current.ID, got.ID)
		require.Equal(t, "ready", got.Status)
	})

	t.Run("current avatar missing", func(t *testing.T) {
		t.Parallel()

		repo := &stubRepo{currentErr: avatars.ErrNotFound}
		rec := doRequest(t, repo, http.MethodGet, "/api/v1/users/user-123/avatar", "user-123")

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"Avatar not found"}`, rec.Body.String())
	})

	t.Run("current avatar requires caller", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{}, http.MethodGet, "/api/v1/users/user-123/avatar", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("delete own avatar", func(t *testing.T) {
		t.Parallel()

		repo := &stubRepo{}
		rec := doRequest(t, repo, http.MethodDelete, "/api/v1/users/user-123/avatar", "user-123")

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.True(t, repo.deleted)
	})

	t.Run("delete someone else's avatar", func(t *testing.T) {
		t.Parallel()

		repo := &stubRepo{}
		rec := doRequest(t, repo, http.MethodDelete, "/api/v1/users/user-123/avatar", "other")

		require.Equal(t, http.StatusForbidden, rec.Code)
		require.False(t, repo.deleted)
		require.JSONEq(t, `{"error":"Forbidden","details":"You can only delete your own avatars"}`, rec.Body.String())
	})

	t.Run("delete missing avatar", func(t *testing.T) {
		t.Parallel()

		repo := &stubRepo{deleteErr: avatars.ErrNotFound}
		rec := doRequest(t, repo, http.MethodDelete, "/api/v1/users/user-123/avatar", "user-123")

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("list avatars newest first", func(t *testing.T) {
		t.Parallel()

		older := current
		older.ID = "older"
		repo := &stubRepo{list: []entity.Avatar{current, older}}
		rec := doRequest(t, repo, http.MethodGet, "/api/v1/users/user-123/avatars", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var got []entity.Avatar
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, []string{current.ID, older.ID}, []string{got[0].ID, got[1].ID})
		require.Equal(t, "/api/v1/avatars/"+current.ID, got[0].URL)
	})

	t.Run("list empty", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{}, http.MethodGet, "/api/v1/users/user-123/avatars", "")
		require.Equal(t, http.StatusOK, rec.Code)
		require.JSONEq(t, `[]`, rec.Body.String())
	})
}

func TestGetAvatarByID(t *testing.T) {
	t.Parallel()

	const id = "1ca79251-f6e3-45a4-992f-404f6e4e13ed"
	original := []byte("original-bytes")
	thumb := []byte("thumb-bytes")
	repo := func() *stubRepo {
		return &stubRepo{
			object: entity.AvatarObject{
				ID:       id,
				MimeType: string(entity.ImageJpeg),
				S3Key:    "avatars/original",
				Thumbnails: map[string]string{
					"100x100": "avatars/100",
				},
			},
			bodies: map[string][]byte{
				"avatars/original": original,
				"avatars/100":      thumb,
			},
		}
	}

	t.Run("original image", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id, "")

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, original, rec.Body.Bytes())
		require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		require.Equal(t, "max-age=86400", rec.Header().Get("Cache-Control"))
		require.NotEmpty(t, rec.Header().Get("ETag"))
	})

	t.Run("thumbnail", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?size=100x100", "")

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, thumb, rec.Body.Bytes())
	})

	t.Run("format must match stored mime", func(t *testing.T) {
		t.Parallel()

		ok := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?format=jpeg", "")
		require.Equal(t, http.StatusOK, ok.Code)

		missing := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?format=png", "")
		require.Equal(t, http.StatusNotFound, missing.Code)
	})

	t.Run("not modified", func(t *testing.T) {
		t.Parallel()

		store := repo()
		rec := doRequest(t, store, http.MethodGet, "/api/v1/avatars/"+id, "")
		etag := rec.Header().Get("ETag")

		again := doRaw(t, store, http.MethodGet, "/api/v1/avatars/"+id, func(req *http.Request) {
			req.Header.Set("If-None-Match", etag)
		})
		require.Equal(t, http.StatusNotModified, again.Code)
		require.Empty(t, again.Body.Bytes())
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{objectErr: avatars.ErrNotFound}, http.MethodGet, "/api/v1/avatars/"+id, "")
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"Avatar not found"}`, rec.Body.String())
	})

	t.Run("missing thumbnail", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?size=300x300", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{}, http.MethodGet, "/api/v1/avatars/not-a-uuid", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"error":"avatar_id must be a uuid"}`, rec.Body.String())
	})

	t.Run("invalid size", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?size=huge", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"error":"invalid size"}`, rec.Body.String())
	})

	t.Run("invalid format", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, repo(), http.MethodGet, "/api/v1/avatars/"+id+"?format=gif", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"error":"invalid format"}`, rec.Body.String())
	})

	t.Run("storage failure", func(t *testing.T) {
		t.Parallel()

		store := repo()
		store.getErr = errors.New("boom")
		rec := doRequest(t, store, http.MethodGet, "/api/v1/avatars/"+id, "")
		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.JSONEq(t, `{"error":"internal error"}`, rec.Body.String())
	})
}

func TestInternalErrorIsLogged(t *testing.T) {
	t.Parallel()

	const id = "1ca79251-f6e3-45a4-992f-404f6e4e13ed"
	core, logs := observer.New(zap.ErrorLevel)
	e := echo.New()
	avatars.NewAvatarRouter(e.Group("/api/v1"), avatars.NewService(&stubRepo{
		object: entity.AvatarObject{
			ID:       id,
			MimeType: string(entity.ImageJpeg),
			S3Key:    "k",
		},
		getErr: errors.New("boom"),
	}), zap.New(core))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+id, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "request failed", entries[0].Message)
	require.Equal(t, "read avatar file: boom", entries[0].ContextMap()["error"])
}

func TestGetAvatarMeta(t *testing.T) {
	t.Parallel()

	const id = "1ca79251-f6e3-45a4-992f-404f6e4e13ed"
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := entity.AvatarMetadata{
		ID:       id,
		UserID:   "user-123",
		FileName: "avatar.jpg",
		MimeType: entity.ImageJpeg,
		Size:     1024000,
		Dimensions: entity.Dimensions{
			Width:  1920,
			Height: 1080,
		},
		Thumbnails: []entity.Thumbnail{
			{Size: entity.ThumbnailSmol},
			{Size: entity.ThumbnailMedium},
		},
		CreatedAt: created,
		UpdatedAt: created,
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{meta: meta}, http.MethodGet, "/api/v1/avatars/"+id+"/metadata", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var got entity.AvatarMetadata
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, id, got.ID)
		require.Equal(t, "user-123", got.UserID)
		require.Equal(t, "avatar.jpg", got.FileName)
		require.Equal(t, entity.ImageJpeg, got.MimeType)
		require.Equal(t, int64(1024000), got.Size)
		require.Equal(t, entity.Dimensions{Width: 1920, Height: 1080}, got.Dimensions)
		require.Equal(t, []entity.Thumbnail{
			{Size: entity.ThumbnailSmol, URL: "/api/v1/avatars/" + id + "?size=100x100"},
			{Size: entity.ThumbnailMedium, URL: "/api/v1/avatars/" + id + "?size=300x300"},
		}, got.Thumbnails)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{metaErr: avatars.ErrNotFound}, http.MethodGet, "/api/v1/avatars/"+id+"/metadata", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"Avatar not found"}`, rec.Body.String())
	})

	t.Run("invalid uuid", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, &stubRepo{}, http.MethodGet, "/api/v1/avatars/not-a-uuid/metadata", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestThumbnailsFromKeys(t *testing.T) {
	t.Parallel()

	got := avatars.ThumbnailsFromKeys(map[string]string{
		"300x300": "k3",
		"100x100": "k1",
		"other":   "kx",
	})
	require.Equal(t, []entity.Thumbnail{
		{Size: entity.ThumbnailSmol},
		{Size: entity.ThumbnailMedium},
	}, got)
}

func doRequest(t *testing.T, repo avatars.RepoProvider, method, path, actorID string) *httptest.ResponseRecorder {
	t.Helper()

	return doRaw(t, repo, method, path, func(req *http.Request) {
		if actorID != "" {
			req.Header.Set(userIDHeader, actorID)
		}
	})
}

func doRaw(t *testing.T, repo avatars.RepoProvider, method, path string, setup func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	avatars.NewAvatarRouter(e.Group("/api/v1"), avatars.NewService(repo), zap.NewNop())

	req := httptest.NewRequest(method, path, nil)
	if setup != nil {
		setup(req)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

type stubRepo struct {
	current    entity.Avatar
	currentErr error
	list       []entity.Avatar
	deleteErr  error
	deleted    bool
	object     entity.AvatarObject
	objectErr  error
	bodies     map[string][]byte
	getErr     error
	meta       entity.AvatarMetadata
	metaErr    error
}

func (s *stubRepo) Upload(_ context.Context, id, userID, fileName, mimeType, s3Key string, size int64, body []byte) (entity.Avatar, error) {
	return entity.Avatar{ID: id, UserID: userID, Status: "processing"}, nil
}
func (s *stubRepo) DeleteByID() error { return nil }

func (s *stubRepo) SelectAvatarMeta(_ context.Context, id string) (entity.AvatarMetadata, error) {
	if s.metaErr != nil {
		return entity.AvatarMetadata{}, s.metaErr
	}
	if s.meta.ID == "" {
		return entity.AvatarMetadata{}, avatars.ErrNotFound
	}
	meta := s.meta
	meta.ID = id
	return meta, nil
}

func (s *stubRepo) SelectByID(context.Context, string) (entity.AvatarObject, error) {
	if s.objectErr != nil {
		return entity.AvatarObject{}, s.objectErr
	}
	if s.object.ID == "" {
		return entity.AvatarObject{}, avatars.ErrNotFound
	}
	return s.object, nil
}

func (s *stubRepo) GetObject(_ context.Context, key string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	body, ok := s.bodies[key]
	if !ok {
		return nil, avatars.ErrNotFound
	}
	return body, nil
}

func (s *stubRepo) SelectCurrent(context.Context, string) (entity.Avatar, error) {
	if s.currentErr != nil {
		return entity.Avatar{}, s.currentErr
	}
	if s.current.ID == "" {
		return entity.Avatar{}, avatars.ErrNotFound
	}
	return s.current, nil
}

func (s *stubRepo) DeleteCurrent(context.Context, string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = true
	return nil
}

func (s *stubRepo) SelectUserAvatars(context.Context, string) ([]entity.Avatar, error) {
	if s.list == nil {
		return []entity.Avatar{}, nil
	}
	return s.list, nil
}
