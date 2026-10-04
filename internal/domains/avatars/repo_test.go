package avatars

import (
	"testing"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/stretchr/testify/require"
)

func TestRepoSQL(t *testing.T) {
	t.Parallel()

	repo := NewAvatarRepo(nil, nil)
	const (
		id     = "1ca79251-f6e3-45a4-992f-404f6e4e13ed"
		userID = "user-1"
	)

	tests := []struct {
		name     string
		query    func() (string, []any, error)
		contains []string
		args     []any
	}{
		{
			name: "uploads to publish",
			query: func() (string, []any, error) {
				return repo.uploadsToPublish().ToSQL()
			},
			contains: []string{
				`"deleted_at" IS NULL`,
				`"processing_status"`,
				`"upload_published_at" IS NULL`,
				"interval '15 seconds'",
				"interval '5 minutes'",
				`ORDER BY "created_at" ASC`,
				"LIMIT",
			},
			args: []any{string(entity.Processing), int64(50)},
		},
		{
			name: "avatar by id",
			query: func() (string, []any, error) {
				return repo.AvatarByID(id).ToSQL()
			},
			contains: []string{
				`"mime_type"`,
				`"s3_key"`,
				`"thumbnail_s3_keys"`,
				`"e_tag"`,
				`"thumbnail_etags"`,
				`"deleted_at" IS NULL`,
				"CAST(",
				"AS uuid)",
			},
			args: []any{id},
		},
		{
			name: "stored by id",
			query: func() (string, []any, error) {
				return repo.storedByID(id).ToSQL()
			},
			contains: []string{
				`"id"`,
				`"user_id"`,
				`"s3_key"`,
				`"thumbnail_s3_keys"`,
				`"processing_status"`,
				`"deleted_at"`,
				"CAST(",
				"AS uuid)",
			},
			args: []any{id},
		},
		{
			name: "avatars by user",
			query: func() (string, []any, error) {
				return repo.avatarsByUser(userID).ToSQL()
			},
			contains: []string{
				`"processing_status"`,
				`"created_at"`,
				`"user_id"`,
				`"deleted_at" IS NULL`,
				`ORDER BY "created_at" DESC`,
			},
			args: []any{userID},
		},
		{
			name: "current avatar",
			query: func() (string, []any, error) {
				return repo.avatarsByUser(userID).Limit(1).ToSQL()
			},
			contains: []string{
				`"deleted_at" IS NULL`,
				`ORDER BY "created_at" DESC`,
				"LIMIT",
			},
			args: []any{userID, int64(1)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sql, args, err := tt.query()
			require.NoError(t, err)
			for _, part := range tt.contains {
				require.Contains(t, sql, part)
			}
			require.Equal(t, tt.args, args)
		})
	}
}

func TestDecodeThumbKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     []byte
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", raw: nil, want: map[string]string{}},
		{name: "blank", raw: []byte{}, want: map[string]string{}},
		{
			name: "object",
			raw:  []byte(`{"100x100":"a","300x300":"b"}`),
			want: map[string]string{"100x100": "a", "300x300": "b"},
		},
		{name: "broken json", raw: []byte(`{`), wantErr: true},
		{name: "array", raw: []byte(`[]`), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeThumbKeys(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestObjectKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		avatar StoredAvatar
		want   []string
	}{
		{
			name: "original and known sizes",
			avatar: StoredAvatar{
				S3Key: "orig",
				Thumbnails: map[string]string{
					"300x300": "med",
					"100x100": "smol",
					"other":   "skip",
				},
			},
			want: []string{"orig", "smol", "med"},
		},
		{
			name:   "original only",
			avatar: StoredAvatar{S3Key: "orig"},
			want:   []string{"orig"},
		},
		{
			name: "thumbnails only",
			avatar: StoredAvatar{
				Thumbnails: map[string]string{"100x100": "smol"},
			},
			want: []string{"smol"},
		},
		{
			name:   "nothing",
			avatar: StoredAvatar{},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.avatar.ObjectKeys())
		})
	}
}

func TestThumbnailsFromKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys map[string]string
		want []entity.Thumbnail
	}{
		{
			name: "known sizes in order",
			keys: map[string]string{
				"300x300": "k3",
				"100x100": "k1",
				"other":   "kx",
			},
			want: []entity.Thumbnail{
				{Size: entity.ThumbnailSmol},
				{Size: entity.ThumbnailMedium},
			},
		},
		{
			name: "skip empty",
			keys: map[string]string{"100x100": "", "300x300": "k3"},
			want: []entity.Thumbnail{{Size: entity.ThumbnailMedium}},
		},
		{
			name: "none",
			keys: nil,
			want: []entity.Thumbnail{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ThumbnailsFromKeys(tt.keys))
		})
	}
}
