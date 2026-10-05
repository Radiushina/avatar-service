package entity

import (
	"errors"
	"time"
)

type MimeType string
type ThumbnailSize string

type UploadStatus string
type ProcessingStatus string

const (
	ImageJpeg MimeType = "image/jpeg"
	ImagePng  MimeType = "image/png"
	ImageWebp MimeType = "image/webp"

	ThumbnailOriginal ThumbnailSize = "original"
	ThumbnailSmol     ThumbnailSize = "100x100"
	ThumbnailMedium   ThumbnailSize = "300x300"

	Uploaded UploadStatus = "uploaded"

	Processing ProcessingStatus = "processing"
	Completed  ProcessingStatus = "completed"
)

type Avatar struct {
	ID        string    `json:"id" db:"avatars.id"`
	UserID    string    `json:"user_id" db:"avatar.user_id"`
	URL       string    `json:"url" db:"avatars.url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at" db:"avatars.created_at"`
}

type AvatarOpt struct {
	ID       string
	UserID   string
	FileName string
	MimeType string
	S3Key    string
	ETag     string
	Size     int64
}

type AvatarMetadata struct {
	ID         string      `json:"id"`
	UserID     string      `json:"user_id"`
	FileName   string      `json:"file_name"`
	MimeType   MimeType    `json:"mime_type"`
	Size       int64       `json:"size"`
	Dimensions Dimensions  `json:"dimensions"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type Dimensions struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

type Thumbnail struct {
	Size ThumbnailSize `json:"size"`
	URL  string        `json:"url"`
}

type AvatarReq struct {
	AvatarID string `param:"avatar_id" validate:"required"`
	Size     string `query:"size" validate:"omitempty,oneof=original 100x100 300x300"`
	Format   string `query:"format" validate:"omitempty,oneof=jpeg png webp"`
}

type AvatarObject struct {
	ID             string
	MimeType       string
	S3Key          string
	Thumbnails     map[string]string
	ETag           string
	ThumbnailETags map[string]string
}

type S3AvatarFile struct {
	ContentType string
	Key         string
	ETag        string
}

func (r AvatarReq) Validate() error {
	switch r.Size {
	case "", string(ThumbnailOriginal), string(ThumbnailSmol), string(ThumbnailMedium):
	default:
		return errors.New("invalid size")
	}
	switch r.Format {
	case "", "jpeg", "png", "webp":
	default:
		return errors.New("invalid format")
	}
	return nil
}
