package avatars_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/stretchr/testify/require"
)

const maxAvatarBytes = 10 << 20

func TestUpload(t *testing.T) {
	t.Parallel()

	body := []byte("img")
	tooBig := make([]byte, maxAvatarBytes+1)
	exact := make([]byte, maxAvatarBytes)

	tests := []struct {
		name       string
		in         avatars.UploadInput
		uploadErr  error
		publishErr error
		markErr    error
		wantErr    error
		wantMime   string
		published  bool
		marked     bool
	}{
		{
			name:    "user id required",
			in:      avatars.UploadInput{FileName: "a.jpg", ContentType: "image/jpeg", Body: body},
			wantErr: avatars.ErrInvalid,
		},
		{
			name:    "file name required",
			in:      avatars.UploadInput{UserID: "user-1", ContentType: "image/jpeg", Body: body},
			wantErr: avatars.ErrInvalid,
		},
		{
			name:    "empty file",
			in:      avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg"},
			wantErr: avatars.ErrInvalid,
		},
		{
			name:    "too large",
			in:      avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: tooBig},
			wantErr: avatars.ErrTooLarge,
		},
		{
			name:    "unsupported type",
			in:      avatars.UploadInput{UserID: "user-1", FileName: "a.gif", ContentType: "image/gif", Body: body},
			wantErr: avatars.ErrInvalid,
		},
		{
			name:      "jpeg",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: body},
			wantMime:  string(entity.ImageJpeg),
			published: true,
			marked:    true,
		},
		{
			name:      "jpg alias",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpg", Body: body},
			wantMime:  string(entity.ImageJpeg),
			published: true,
			marked:    true,
		},
		{
			name:      "png",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.png", ContentType: "image/png", Body: body},
			wantMime:  string(entity.ImagePng),
			published: true,
			marked:    true,
		},
		{
			name:      "webp",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.webp", ContentType: "image/webp", Body: body},
			wantMime:  string(entity.ImageWebp),
			published: true,
			marked:    true,
		},
		{
			name:      "exact size limit",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: exact},
			wantMime:  string(entity.ImageJpeg),
			published: true,
			marked:    true,
		},
		{
			name:      "repo error",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: body},
			uploadErr: errors.New("db down"),
			wantErr:   errors.New("upload avatar"),
		},
		{
			name:       "publish failure still returns avatar",
			in:         avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: body},
			publishErr: errors.New("rabbit down"),
			wantMime:   string(entity.ImageJpeg),
			published:  true,
		},
		{
			name:      "mark published failure still returns avatar",
			in:        avatars.UploadInput{UserID: "user-1", FileName: "a.jpg", ContentType: "image/jpeg", Body: body},
			markErr:   errors.New("db down"),
			wantMime:  string(entity.ImageJpeg),
			published: true,
			marked:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{uploadErr: tt.uploadErr, markErr: tt.markErr}
			pub := &memPub{err: tt.publishErr}
			got, err := avatars.NewService(repo, pub, nil).Upload(context.Background(), tt.in)
			if tt.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tt.wantErr, avatars.ErrInvalid) || errors.Is(tt.wantErr, avatars.ErrTooLarge) {
					require.ErrorIs(t, err, tt.wantErr)
				}
				require.ErrorContains(t, err, tt.wantErr.Error())
				require.Zero(t, pub.calls)
				require.Empty(t, repo.marked)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.in.UserID, got.UserID)
			require.Equal(t, "/api/v1/avatars/"+got.ID, got.URL)
			require.Equal(t, tt.wantMime, repo.uploaded.MimeType)
			require.Equal(t, tt.in.FileName, repo.uploaded.FileName)
			require.Equal(t, int64(len(tt.in.Body)), repo.uploaded.Size)
			require.Equal(t, avatars.FileETag(tt.in.Body), repo.uploaded.Etag)
			require.Equal(t, "avatars/"+tt.in.UserID+"/"+got.ID+"/original", repo.uploaded.S3Key)
			require.Equal(t, tt.in.Body, repo.body)

			if !tt.published {
				require.Zero(t, pub.calls)
				require.Empty(t, repo.marked)
				return
			}
			require.Equal(t, 1, pub.calls)
			require.Equal(t, broker.Exchange, pub.exchange)
			require.Equal(t, broker.KeyUploaded, pub.key)
			require.Equal(t, broker.AvatarUploadEvent{
				AvatarID: got.ID,
				UserID:   tt.in.UserID,
				S3Key:    repo.uploaded.S3Key,
			}, pub.event)
			if tt.marked {
				require.Equal(t, got.ID, repo.marked)
				return
			}
			require.Empty(t, repo.marked)
		})
	}
}

func TestSelectByID(t *testing.T) {
	t.Parallel()

	obj := entity.AvatarObject{
		MimeType: string(entity.ImagePng),
		S3Key:    "avatars/original",
		ETag:     `"orig"`,
		Thumbnails: map[string]string{
			"100x100": "avatars/100",
			"300x300": "avatars/300",
		},
		ThumbnailETags: map[string]string{
			"100x100": `"e100"`,
			"300x300": `"e300"`,
		},
	}

	tests := []struct {
		name      string
		req       entity.AvatarReq
		object    entity.AvatarObject
		objectErr error
		want      entity.S3AvatarFile
		wantErr   error
	}{
		{
			name:   "original by default",
			req:    entity.AvatarReq{AvatarID: "id-1"},
			object: obj,
			want:   entity.S3AvatarFile{ContentType: string(entity.ImagePng), Key: "avatars/original", ETag: `"orig"`},
		},
		{
			name:   "explicit original",
			req:    entity.AvatarReq{AvatarID: "id-1", Size: string(entity.ThumbnailOriginal)},
			object: obj,
			want:   entity.S3AvatarFile{ContentType: string(entity.ImagePng), Key: "avatars/original", ETag: `"orig"`},
		},
		{
			name:   "small thumbnail is jpeg",
			req:    entity.AvatarReq{AvatarID: "id-1", Size: string(entity.ThumbnailSmol)},
			object: obj,
			want:   entity.S3AvatarFile{ContentType: string(entity.ImageJpeg), Key: "avatars/100", ETag: `"e100"`},
		},
		{
			name:   "medium thumbnail",
			req:    entity.AvatarReq{AvatarID: "id-1", Size: string(entity.ThumbnailMedium)},
			object: obj,
			want:   entity.S3AvatarFile{ContentType: string(entity.ImageJpeg), Key: "avatars/300", ETag: `"e300"`},
		},
		{
			name:    "missing thumbnail",
			req:     entity.AvatarReq{AvatarID: "id-1", Size: string(entity.ThumbnailSmol)},
			object:  entity.AvatarObject{S3Key: "avatars/original", MimeType: string(entity.ImagePng)},
			wantErr: avatars.ErrNotFound,
		},
		{
			name:    "missing original",
			req:     entity.AvatarReq{AvatarID: "id-1"},
			object:  entity.AvatarObject{MimeType: string(entity.ImagePng)},
			wantErr: avatars.ErrNotFound,
		},
		{
			name:   "matching format",
			req:    entity.AvatarReq{AvatarID: "id-1", Format: "png"},
			object: obj,
			want:   entity.S3AvatarFile{ContentType: string(entity.ImagePng), Key: "avatars/original", ETag: `"orig"`},
		},
		{
			name:    "format mismatch",
			req:     entity.AvatarReq{AvatarID: "id-1", Format: "jpeg"},
			object:  obj,
			wantErr: avatars.ErrNotFound,
		},
		{
			name:      "repo error",
			req:       entity.AvatarReq{AvatarID: "id-1"},
			objectErr: avatars.ErrNotFound,
			wantErr:   avatars.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{object: tt.object, objectErr: tt.objectErr}
			got, err := avatars.NewService(repo, &memPub{}, nil).SelectByID(context.Background(), tt.req)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.req.AvatarID, repo.selectedID)
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		body    []byte
		getErr  error
		wantErr string
	}{
		{name: "ok", key: "avatars/original", body: []byte("img")},
		{name: "missing", key: "missing", getErr: avatars.ErrNotFound, wantErr: "read avatar file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{getBody: tt.body, getErr: tt.getErr}
			got, err := avatars.NewService(repo, &memPub{}, nil).Read(context.Background(), tt.key)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorIs(t, err, tt.getErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.body, got)
			require.Equal(t, tt.key, repo.getKey)
		})
	}
}

func TestDeleteByID(t *testing.T) {
	t.Parallel()

	removed := avatars.Removal{ID: "av-1", S3Keys: []string{"orig", "smol"}}

	tests := []struct {
		name       string
		actorID    string
		avatarID   string
		removal    avatars.Removal
		deleteErr  error
		publishErr error
		wantErr    error
		wantCalls  int
		wantEvent  any
	}{
		{
			name:     "actor required",
			avatarID: "av-1",
			wantErr:  avatars.ErrInvalid,
		},
		{
			name:      "publishes keys",
			actorID:   "user-1",
			avatarID:  "av-1",
			removal:   removed,
			wantCalls: 1,
			wantEvent: broker.AvatarDeleteEvent{AvatarID: "av-1", S3Keys: []string{"orig", "smol"}},
		},
		{
			name:      "forbidden",
			actorID:   "user-1",
			avatarID:  "av-1",
			deleteErr: avatars.ErrForbidden,
			wantErr:   avatars.ErrForbidden,
		},
		{
			name:       "publish error",
			actorID:    "user-1",
			avatarID:   "av-1",
			removal:    removed,
			publishErr: errors.New("rabbit down"),
			wantErr:    errors.New("publish delete event"),
			wantCalls:  1,
			wantEvent:  broker.AvatarDeleteEvent{AvatarID: "av-1", S3Keys: []string{"orig", "smol"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{removal: tt.removal, deleteErr: tt.deleteErr}
			pub := &memPub{err: tt.publishErr}
			err := avatars.NewService(repo, pub, nil).DeleteByID(context.Background(), tt.actorID, tt.avatarID)
			if tt.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tt.wantErr, avatars.ErrInvalid) || errors.Is(tt.wantErr, avatars.ErrForbidden) {
					require.ErrorIs(t, err, tt.wantErr)
				}
				require.ErrorContains(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantCalls, pub.calls)
			if tt.wantCalls == 0 {
				return
			}
			require.Equal(t, tt.avatarID, repo.deletedID)
			require.Equal(t, tt.actorID, repo.deletedUser)
			require.Equal(t, broker.Exchange, pub.exchange)
			require.Equal(t, broker.KeyDeleted, pub.key)
			require.Equal(t, tt.wantEvent, pub.event)
		})
	}
}

func TestSelectAvatarMeta(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	meta := entity.AvatarMetadata{
		ID:        "id-1",
		UserID:    "user-1",
		FileName:  "a.png",
		MimeType:  entity.ImagePng,
		Size:      12,
		CreatedAt: created,
		Thumbnails: []entity.Thumbnail{
			{Size: entity.ThumbnailMedium},
			{Size: entity.ThumbnailSmol},
		},
	}

	tests := []struct {
		name    string
		meta    entity.AvatarMetadata
		metaErr error
		want    entity.AvatarMetadata
		wantErr error
	}{
		{
			name: "fills thumbnail urls",
			meta: meta,
			want: entity.AvatarMetadata{
				ID:        meta.ID,
				UserID:    meta.UserID,
				FileName:  meta.FileName,
				MimeType:  meta.MimeType,
				Size:      meta.Size,
				CreatedAt: created,
				Thumbnails: []entity.Thumbnail{
					{Size: entity.ThumbnailMedium, URL: "/api/v1/avatars/id-1?size=300x300"},
					{Size: entity.ThumbnailSmol, URL: "/api/v1/avatars/id-1?size=100x100"},
				},
			},
		},
		{
			name:    "not found",
			metaErr: avatars.ErrNotFound,
			wantErr: avatars.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{meta: tt.meta, metaErr: tt.metaErr}
			got, err := avatars.NewService(repo, &memPub{}, nil).SelectAvatarMeta(context.Background(), "id-1")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSelectCurrent(t *testing.T) {
	t.Parallel()

	current := entity.Avatar{ID: "id-1", UserID: "user-1", Status: "processing"}

	tests := []struct {
		name    string
		avatar  entity.Avatar
		err     error
		wantURL string
		wantErr error
	}{
		{name: "adds url", avatar: current, wantURL: "/api/v1/avatars/id-1"},
		{name: "not found", err: avatars.ErrNotFound, wantErr: avatars.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{current: tt.avatar, currentErr: tt.err}
			got, err := avatars.NewService(repo, &memPub{}, nil).SelectCurrent(context.Background(), "user-1")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantURL, got.URL)
			require.Equal(t, current.ID, got.ID)
			require.Equal(t, "user-1", repo.currentUser)
		})
	}
}

func TestDeleteCurrent(t *testing.T) {
	t.Parallel()

	removed := avatars.Removal{ID: "av-1", S3Keys: []string{"orig"}}

	tests := []struct {
		name       string
		actorID    string
		userID     string
		removal    avatars.Removal
		deleteErr  error
		publishErr error
		wantErr    error
		wantEvent  any
	}{
		{name: "empty actor", userID: "user-1", wantErr: avatars.ErrForbidden},
		{name: "other user", actorID: "other", userID: "user-1", wantErr: avatars.ErrForbidden},
		{
			name:      "own avatar",
			actorID:   "user-1",
			userID:    "user-1",
			removal:   removed,
			wantEvent: broker.AvatarDeleteEvent{AvatarID: "av-1", S3Keys: []string{"orig"}},
		},
		{
			name:      "missing",
			actorID:   "user-1",
			userID:    "user-1",
			deleteErr: avatars.ErrNotFound,
			wantErr:   avatars.ErrNotFound,
		},
		{
			name:       "publish error",
			actorID:    "user-1",
			userID:     "user-1",
			removal:    removed,
			publishErr: errors.New("rabbit down"),
			wantErr:    errors.New("publish delete event"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{currentRemoval: tt.removal, deleteErr: tt.deleteErr}
			pub := &memPub{err: tt.publishErr}
			err := avatars.NewService(repo, pub, nil).DeleteCurrent(context.Background(), tt.actorID, tt.userID)
			if tt.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tt.wantErr, avatars.ErrForbidden) || errors.Is(tt.wantErr, avatars.ErrNotFound) {
					require.ErrorIs(t, err, tt.wantErr)
				}
				require.ErrorContains(t, err, tt.wantErr.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.userID, repo.currentUser)
			require.Equal(t, broker.KeyDeleted, pub.key)
			require.Equal(t, tt.wantEvent, pub.event)
		})
	}
}

func TestSelectUserAvatars(t *testing.T) {
	t.Parallel()

	list := []entity.Avatar{
		{ID: "new", UserID: "user-1"},
		{ID: "old", UserID: "user-1"},
	}

	tests := []struct {
		name    string
		list    []entity.Avatar
		listErr error
		want    []entity.Avatar
		wantErr error
	}{
		{
			name: "adds urls",
			list: list,
			want: []entity.Avatar{
				{ID: "new", UserID: "user-1", URL: "/api/v1/avatars/new"},
				{ID: "old", UserID: "user-1", URL: "/api/v1/avatars/old"},
			},
		},
		{
			name: "nil becomes empty",
			want: []entity.Avatar{},
		},
		{
			name:    "repo error",
			listErr: errors.New("db down"),
			wantErr: errors.New("list user avatars"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &memRepo{list: tt.list, listErr: tt.listErr}
			got, err := avatars.NewService(repo, &memPub{}, nil).SelectUserAvatars(context.Background(), "user-1")
			if tt.wantErr != nil {
				require.ErrorContains(t, err, tt.wantErr.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFileETag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body []byte
		want string
	}{
		{name: "empty", body: nil, want: `"e3b0c44298fc1c14"`},
		{name: "hello", body: []byte("hello"), want: `"2cf24dba5fb0a30e"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, avatars.FileETag(tt.body))
		})
	}
}

type memRepo struct {
	uploadErr error
	uploaded  entity.AvatarOpt
	body      []byte

	object     entity.AvatarObject
	objectErr  error
	selectedID string

	getBody []byte
	getErr  error
	getKey  string

	removal     avatars.Removal
	deleteErr   error
	deletedID   string
	deletedUser string

	meta    entity.AvatarMetadata
	metaErr error

	current     entity.Avatar
	currentErr  error
	currentUser string

	currentRemoval avatars.Removal

	list    []entity.Avatar
	listErr error

	markErr error
	marked  string
}

func (m *memRepo) Upload(_ context.Context, opt entity.AvatarOpt, body []byte) (entity.Avatar, error) {
	m.uploaded = opt
	m.body = append([]byte(nil), body...)
	if m.uploadErr != nil {
		return entity.Avatar{}, m.uploadErr
	}
	return entity.Avatar{ID: opt.ID, UserID: opt.UserID, Status: string(entity.Processing)}, nil
}

func (m *memRepo) SelectByID(_ context.Context, id string) (entity.AvatarObject, error) {
	m.selectedID = id
	if m.objectErr != nil {
		return entity.AvatarObject{}, m.objectErr
	}
	return m.object, nil
}

func (m *memRepo) GetObject(_ context.Context, key string) ([]byte, error) {
	m.getKey = key
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getBody, nil
}

func (m *memRepo) DeleteByID(_ context.Context, avatarID, userID string) (avatars.Removal, error) {
	m.deletedID = avatarID
	m.deletedUser = userID
	if m.deleteErr != nil {
		return avatars.Removal{}, m.deleteErr
	}
	return m.removal, nil
}

func (m *memRepo) SelectAvatarMeta(context.Context, string) (entity.AvatarMetadata, error) {
	if m.metaErr != nil {
		return entity.AvatarMetadata{}, m.metaErr
	}
	return m.meta, nil
}

func (m *memRepo) SelectCurrent(_ context.Context, userID string) (entity.Avatar, error) {
	m.currentUser = userID
	if m.currentErr != nil {
		return entity.Avatar{}, m.currentErr
	}
	return m.current, nil
}

func (m *memRepo) DeleteCurrent(_ context.Context, userID string) (avatars.Removal, error) {
	m.currentUser = userID
	if m.deleteErr != nil {
		return avatars.Removal{}, m.deleteErr
	}
	return m.currentRemoval, nil
}

func (m *memRepo) SelectUserAvatars(context.Context, string) ([]entity.Avatar, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.list, nil
}

func (m *memRepo) MarkUploadPublished(_ context.Context, id string) error {
	m.marked = id
	return m.markErr
}

type memPub struct {
	err      error
	calls    int
	exchange string
	key      string
	event    any
}

func (p *memPub) Publish(_ context.Context, exchange, routingKey string, event any) error {
	p.calls++
	p.exchange = exchange
	p.key = routingKey
	p.event = event
	return p.err
}
