package avatars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key, contentType string, body []byte) error
}

type Repo struct {
	db      *pgxpool.Pool
	objects ObjectStore
	builder goqu.DialectWrapper
}

func NewAvatarRepo(db *pgxpool.Pool, objects ObjectStore) *Repo {
	return &Repo{
		db:      db,
		objects: objects,
		builder: goqu.Dialect("postgres"),
	}
}

var avatarsTable = goqu.T("avatars")

func (r *Repo) Upload(ctx context.Context, id, userID, fileName, mimeType, s3Key string, size int64, body []byte) (entity.Avatar, error) {
	if r.objects == nil {
		return entity.Avatar{}, errors.New("object storage is not configured")
	}
	if err := r.objects.Put(ctx, s3Key, mimeType, body); err != nil {
		return entity.Avatar{}, err
	}

	sql, args, err := r.builder.Insert(avatarsTable).
		Prepared(true).
		Rows(goqu.Record{
			"id":                id,
			"user_id":           userID,
			"file_name":         fileName,
			"mime_type":         mimeType,
			"size_bytes":        size,
			"s3_key":            s3Key,
			"upload_status":     "uploaded",
			"processing_status": "processing",
		}).
		Returning("id", "user_id", "processing_status", "created_at").
		ToSQL()
	if err != nil {
		return entity.Avatar{}, fmt.Errorf("insert avatar: %w", err)
	}

	var avatar entity.Avatar
	if err := r.db.QueryRow(ctx, sql, args...).Scan(&avatar.ID, &avatar.UserID, &avatar.Status, &avatar.CreatedAt); err != nil {
		return entity.Avatar{}, fmt.Errorf("insert avatar: %w", err)
	}
	return avatar, nil
}

func (r *Repo) SelectById(ctx context.Context, id string) (entity.AvatarObject, error) {
	query, args, err := r.avatarByID(id).ToSQL()
	if err != nil {
		return entity.AvatarObject{}, fmt.Errorf("select avatar: %w", err)
	}

	var (
		obj    entity.AvatarObject
		thumbs []byte
	)
	err = r.db.QueryRow(ctx, query, args...).Scan(&obj.MimeType, &obj.S3Key, &thumbs)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.AvatarObject{}, ErrNotFound
	}
	if err != nil {
		return entity.AvatarObject{}, fmt.Errorf("select avatar: %w", err)
	}

	obj.ID = id
	// ponytail: thumbnail_s3_keys is {"100x100":"s3-key","300x300":"s3-key"}.
	// An array of objects needs a new decoder.
	if len(thumbs) > 0 {
		if err := json.Unmarshal(thumbs, &obj.Thumbnails); err != nil {
			return entity.AvatarObject{}, fmt.Errorf("decode thumbnail keys: %w", err)
		}
	}
	return obj, nil
}

func (r *Repo) GetObject(ctx context.Context, key string) ([]byte, error) {
	if r.objects == nil {
		return nil, errors.New("object storage is not configured")
	}
	return r.objects.Get(ctx, key)
}

func (r *Repo) avatarByID(id string) *goqu.SelectDataset {
	return r.builder.From(avatarsTable).
		Prepared(true).
		Select("mime_type", "s3_key", "thumbnail_s3_keys").
		Where(
			goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")),
			goqu.C("deleted_at").IsNull(),
		)
}

func (r *Repo) DeleteById() error {

	return nil
}

func (r *Repo) SelectAvatarMeta(ctx context.Context, id string) (entity.AvatarMetadata, error) {
	query, args, err := r.builder.From(avatarsTable).
		Prepared(true).
		Select(
			"id",
			"user_id",
			"file_name",
			"mime_type",
			"size_bytes",
			"thumbnail_s3_keys",
			"created_at",
			"updated_at",
		).
		Where(
			goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")),
			goqu.C("deleted_at").IsNull(),
		).
		ToSQL()
	if err != nil {
		return entity.AvatarMetadata{}, fmt.Errorf("select avatar meta: %w", err)
	}

	var (
		meta   entity.AvatarMetadata
		thumbs []byte
	)
	err = r.db.QueryRow(ctx, query, args...).Scan(
		&meta.ID,
		&meta.UserID,
		&meta.FileName,
		&meta.MimeType,
		&meta.Size,
		&thumbs,
		&meta.CreatedAt,
		&meta.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.AvatarMetadata{}, ErrNotFound
	}
	if err != nil {
		return entity.AvatarMetadata{}, fmt.Errorf("select avatar meta: %w", err)
	}

	keys := map[string]string{}
	if len(thumbs) > 0 {
		if err := json.Unmarshal(thumbs, &keys); err != nil {
			return entity.AvatarMetadata{}, fmt.Errorf("decode thumbnail keys: %w", err)
		}
	}
	meta.Thumbnails = thumbnailsFromKeys(keys)
	return meta, nil
}

func thumbnailsFromKeys(keys map[string]string) []entity.Thumbnail {
	order := []entity.ThumbnailSize{entity.ThumbnailSmol, entity.ThumbnailMedium}
	out := make([]entity.Thumbnail, 0, len(order))
	for _, size := range order {
		if keys[string(size)] == "" {
			continue
		}
		out = append(out, entity.Thumbnail{Size: size})
	}
	return out
}

func (r *Repo) SelectCurrent(ctx context.Context, userID string) (entity.Avatar, error) {
	sql, args, err := r.avatarsByUser(userID).Limit(1).ToSQL()
	if err != nil {
		return entity.Avatar{}, err
	}

	var avatar entity.Avatar
	err = r.db.QueryRow(ctx, sql, args...).Scan(&avatar.ID, &avatar.UserID, &avatar.Status, &avatar.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Avatar{}, ErrNotFound
	}
	if err != nil {
		return entity.Avatar{}, err
	}
	return avatar, nil
}

func (r *Repo) DeleteCurrent(ctx context.Context, userID string) error {
	current, err := r.SelectCurrent(ctx, userID)
	if err != nil {
		return err
	}

	sql, args, err := r.builder.Update(avatarsTable).
		Prepared(true).
		Set(goqu.Record{
			"deleted_at": goqu.L("now()"),
			"updated_at": goqu.L("now()"),
		}).
		Where(
			goqu.C("id").Eq(current.ID),
			goqu.C("deleted_at").IsNull(),
		).
		ToSQL()
	if err != nil {
		return err
	}

	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error) {
	sql, args, err := r.avatarsByUser(userID).ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]entity.Avatar, 0)
	for rows.Next() {
		var avatar entity.Avatar
		if err := rows.Scan(&avatar.ID, &avatar.UserID, &avatar.Status, &avatar.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, avatar)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *Repo) avatarsByUser(userID string) *goqu.SelectDataset {
	return r.builder.From(avatarsTable).
		Prepared(true).
		Select("id", "user_id", "processing_status", "created_at").
		Where(
			goqu.C("user_id").Eq(userID),
			goqu.C("deleted_at").IsNull(),
		).
		Order(goqu.C("created_at").Desc())
}
