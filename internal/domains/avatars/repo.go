package avatars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // registers the postgres dialect
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

func (r *Repo) Upload(ctx context.Context, opt entity.AvatarOpt, body []byte) (entity.Avatar, error) {
	if r.objects == nil {
		return entity.Avatar{}, errors.New("object storage is not configured")
	}
	if err := r.objects.Put(ctx, opt.S3Key, opt.MimeType, body); err != nil {
		return entity.Avatar{}, fmt.Errorf("upload: %w", err)
	}

	sql, args, err := r.builder.Insert(avatarsTable).
		Prepared(true).
		Rows(goqu.Record{
			"id":                opt.ID,
			"user_id":           opt.UserID,
			"file_name":         opt.FileName,
			"mime_type":         opt.MimeType,
			"size_bytes":        opt.Size,
			"s3_key":            opt.S3Key,
			"upload_status":     entity.Uploaded,
			"processing_status": entity.Processing,
			"e_tag":             opt.Etag,
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

func (r *Repo) SelectByID(ctx context.Context, id string) (entity.AvatarObject, error) {
	query, args, err := r.AvatarByID(id).ToSQL()
	if err != nil {
		return entity.AvatarObject{}, fmt.Errorf("select avatar: %w", err)
	}

	var (
		obj    entity.AvatarObject
		thumbs []byte
		etags  []byte
	)
	err = r.db.QueryRow(ctx, query, args...).Scan(&obj.MimeType, &obj.S3Key, &thumbs, &obj.ETag, &etags)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.AvatarObject{}, ErrNotFound
	}
	if err != nil {
		return entity.AvatarObject{}, fmt.Errorf("select avatar: %w", err)
	}

	obj.ID = id
	// ponytail: thumbnail_s3_keys is {"100x100":"s3-key","300x300":"s3-key"}.
	// An array of objects needs a new decoder.
	obj.Thumbnails, err = decodeThumbKeys(thumbs)
	if err != nil {
		return entity.AvatarObject{}, err
	}
	obj.ThumbnailETags, err = decodeThumbKeys(etags)
	if err != nil {
		return entity.AvatarObject{}, err
	}
	return obj, nil
}

func (r *Repo) GetObject(ctx context.Context, key string) ([]byte, error) {
	if r.objects == nil {
		return nil, errors.New("object storage is not configured")
	}
	data, err := r.objects.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	return data, nil
}

func (r *Repo) AvatarByID(id string) *goqu.SelectDataset {
	return r.builder.From(avatarsTable).
		Prepared(true).
		Select("mime_type", "s3_key", "thumbnail_s3_keys", "e_tag", "thumbnail_etags").
		Where(
			goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")),
			goqu.C("deleted_at").IsNull(),
		)
}

func (r *Repo) GetAvatar(ctx context.Context, id string) (StoredAvatar, error) {
	query, args, err := r.storedByID(id).ToSQL()
	if err != nil {
		return StoredAvatar{}, fmt.Errorf("select avatar: %w", err)
	}
	return r.scanStored(ctx, query, args)
}

func (r *Repo) UpdateProcessingStatus(ctx context.Context, id string, status entity.ProcessingStatus, thumbs map[string]string, etags map[string]string) error {
	raw, err := json.Marshal(thumbs)
	if err != nil {
		return fmt.Errorf("encode thumbnail keys: %w", err)
	}
	rawETags, err := json.Marshal(etags)
	if err != nil {
		return fmt.Errorf("encode thumbnail etags: %w", err)
	}
	sql, args, err := r.builder.Update(avatarsTable).
		Prepared(true).
		Set(goqu.Record{
			"processing_status": status,
			"thumbnail_s3_keys": goqu.Cast(goqu.V(string(raw)), "jsonb"),
			"thumbnail_etags":   goqu.Cast(goqu.V(string(rawETags)), "jsonb"),
			"updated_at":        goqu.L("now()"),
		}).
		Where(
			goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")),
			goqu.C("deleted_at").IsNull(),
		).
		ToSQL()
	if err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}
	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteByID(ctx context.Context, avatarID, userID string) (Removal, error) {
	avatar, err := r.GetAvatar(ctx, avatarID)
	if err != nil {
		return Removal{}, fmt.Errorf("delete avatar: %w", err)
	}
	if avatar.Deleted {
		return Removal{}, ErrNotFound
	}
	if avatar.UserID != userID {
		return Removal{}, ErrForbidden
	}
	return r.remove(ctx, avatar)
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

	keys, err := decodeThumbKeys(thumbs)
	if err != nil {
		return entity.AvatarMetadata{}, err
	}
	meta.Thumbnails = ThumbnailsFromKeys(keys)
	return meta, nil
}

func ThumbnailsFromKeys(keys map[string]string) []entity.Thumbnail {
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
		return entity.Avatar{}, fmt.Errorf("select current avatar: %w", err)
	}

	var avatar entity.Avatar
	err = r.db.QueryRow(ctx, sql, args...).Scan(&avatar.ID, &avatar.UserID, &avatar.Status, &avatar.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Avatar{}, ErrNotFound
	}
	if err != nil {
		return entity.Avatar{}, fmt.Errorf("select current avatar: %w", err)
	}
	return avatar, nil
}

func (r *Repo) DeleteCurrent(ctx context.Context, userID string) (Removal, error) {
	query, args, err := r.storedColumns().
		Where(
			goqu.C("user_id").Eq(userID),
			goqu.C("deleted_at").IsNull(),
		).
		Order(goqu.C("created_at").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return Removal{}, fmt.Errorf("delete current avatar: %w", err)
	}
	avatar, err := r.scanStored(ctx, query, args)
	if err != nil {
		return Removal{}, fmt.Errorf("delete current avatar: %w", err)
	}
	removed, err := r.remove(ctx, avatar)
	if err != nil {
		return Removal{}, fmt.Errorf("delete current avatar: %w", err)
	}
	return removed, nil
}

func (r *Repo) remove(ctx context.Context, avatar StoredAvatar) (Removal, error) {
	if err := r.softDelete(ctx, avatar.ID); err != nil {
		return Removal{}, err
	}
	return Removal{ID: avatar.ID, S3Keys: avatar.ObjectKeys()}, nil
}

func (r *Repo) softDelete(ctx context.Context, id string) error {
	sql, args, err := r.builder.Update(avatarsTable).
		Prepared(true).
		Set(goqu.Record{
			"deleted_at": goqu.L("now()"),
			"updated_at": goqu.L("now()"),
		}).
		Where(
			goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")),
			goqu.C("deleted_at").IsNull(),
		).
		ToSQL()
	if err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (a StoredAvatar) ObjectKeys() []string {
	keys := make([]string, 0, 1+len(a.Thumbnails))
	if a.S3Key != "" {
		keys = append(keys, a.S3Key)
	}
	for _, size := range []entity.ThumbnailSize{entity.ThumbnailSmol, entity.ThumbnailMedium} {
		if key := a.Thumbnails[string(size)]; key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func (r *Repo) storedByID(id string) *goqu.SelectDataset {
	return r.storedColumns().Where(goqu.C("id").Eq(goqu.Cast(goqu.V(id), "uuid")))
}

func (r *Repo) storedColumns() *goqu.SelectDataset {
	return r.builder.From(avatarsTable).
		Prepared(true).
		Select("id", "user_id", "s3_key", "thumbnail_s3_keys", "processing_status", "deleted_at")
}

func (r *Repo) scanStored(ctx context.Context, query string, args []any) (StoredAvatar, error) {
	var (
		avatar    StoredAvatar
		thumbs    []byte
		status    *entity.ProcessingStatus
		deletedAt *time.Time
	)
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.S3Key,
		&thumbs,
		&status,
		&deletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredAvatar{}, ErrNotFound
	}
	if err != nil {
		return StoredAvatar{}, fmt.Errorf("select avatar: %w", err)
	}
	if status != nil {
		avatar.ProcessingStatus = *status
	}
	avatar.Deleted = deletedAt != nil
	avatar.Thumbnails, err = decodeThumbKeys(thumbs)
	if err != nil {
		return StoredAvatar{}, err
	}
	return avatar, nil
}

func decodeThumbKeys(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	keys := map[string]string{}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("decode thumbnail keys: %w", err)
	}
	return keys, nil
}

func (r *Repo) SelectUserAvatars(ctx context.Context, userID string) ([]entity.Avatar, error) {
	sql, args, err := r.avatarsByUser(userID).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("select user avatars: %w", err)
	}

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("select user avatars: %w", err)
	}
	defer rows.Close()

	list := make([]entity.Avatar, 0)
	for rows.Next() {
		var avatar entity.Avatar
		if err := rows.Scan(&avatar.ID, &avatar.UserID, &avatar.Status, &avatar.CreatedAt); err != nil {
			return nil, fmt.Errorf("select user avatars: %w", err)
		}
		list = append(list, avatar)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select user avatars: %w", err)
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
