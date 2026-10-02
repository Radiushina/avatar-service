package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/Radiushina/avatar-service/internal/worker"
	"github.com/stretchr/testify/require"
)

func TestHandleUploadEvent(t *testing.T) {
	t.Parallel()

	const id = "1ca79251-f6e3-45a4-992f-404f6e4e13ed"
	repo := &fakeRepo{avatar: avatars.StoredAvatar{
		ID:               id,
		ProcessingStatus: "processing",
	}}
	store := &memStore{objects: map[string][]byte{
		"avatars/original": mustPNG(t, 8, 4),
	}}
	w := worker.New(repo, store, worker.NewResizer(), nil, nil)

	body := mustJSON(t, broker.AvatarUploadEvent{
		AvatarID: id,
		UserID:   "user-1",
		S3Key:    "avatars/original",
	})
	require.NoError(t, w.HandleUploadEvent(t.Context(), body))
	require.Equal(t, entity.Completed, repo.status)
	require.Equal(t, map[string]string{
		"100x100": "thumbnails/" + id + "/100x100.jpg",
		"300x300": "thumbnails/" + id + "/300x300.jpg",
	}, repo.thumbs)
	require.Equal(t, map[string]string{
		"100x100": avatars.FileETag(store.objects["thumbnails/"+id+"/100x100.jpg"]),
		"300x300": avatars.FileETag(store.objects["thumbnails/"+id+"/300x300.jpg"]),
	}, repo.etags)

	small := decodeJPEG(t, store.objects["thumbnails/"+id+"/100x100.jpg"])
	require.Equal(t, image.Rect(0, 0, 100, 100), small.Bounds())
	medium := decodeJPEG(t, store.objects["thumbnails/"+id+"/300x300.jpg"])
	require.Equal(t, image.Rect(0, 0, 300, 300), medium.Bounds())

	puts := len(store.puts)
	require.NoError(t, w.HandleUploadEvent(t.Context(), body))
	require.Len(t, store.puts, puts)
	require.Equal(t, 1, repo.updates)
}

func TestHandleDeleteEvent(t *testing.T) {
	t.Parallel()

	store := &memStore{objects: map[string][]byte{
		"avatars/original":          []byte("a"),
		"thumbnails/id/100x100.jpg": []byte("b"),
	}}
	w := worker.New(&fakeRepo{}, store, worker.NewResizer(), nil, nil)
	body := mustJSON(t, broker.AvatarDeleteEvent{
		AvatarID: "id",
		S3Keys:   []string{"avatars/original", "thumbnails/id/100x100.jpg"},
	})
	require.NoError(t, w.HandleDeleteEvent(t.Context(), body))
	require.Empty(t, store.objects)

	require.NoError(t, w.HandleDeleteEvent(t.Context(), body))
}

func mustJSON(t *testing.T, event any) []byte {
	t.Helper()
	body, err := json.Marshal(event)
	require.NoError(t, err)
	return body
}

func mustPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func decodeJPEG(t *testing.T, raw []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

type fakeRepo struct {
	avatar  avatars.StoredAvatar
	status  entity.ProcessingStatus
	thumbs  map[string]string
	etags   map[string]string
	updates int
}

func (f *fakeRepo) GetAvatar(context.Context, string) (avatars.StoredAvatar, error) {
	return f.avatar, nil
}

func (f *fakeRepo) UpdateProcessingStatus(_ context.Context, _ string, status entity.ProcessingStatus, thumbs, etags map[string]string) error {
	f.updates++
	f.status = status
	f.thumbs = thumbs
	f.etags = etags
	f.avatar.ProcessingStatus = status
	return nil
}

type memStore struct {
	objects map[string][]byte
	puts    []string
}

func (m *memStore) Get(_ context.Context, key string) ([]byte, error) {
	body, ok := m.objects[key]
	if !ok {
		return nil, avatars.ErrNotFound
	}
	return body, nil
}

func (m *memStore) Put(_ context.Context, key, _ string, body []byte) error {
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.objects[key] = body
	m.puts = append(m.puts, key)
	return nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}
