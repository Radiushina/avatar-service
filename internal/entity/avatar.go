package entity

import "time"

type MimeType string
type ThumbnailSize string

const (
	ImageJpeg MimeType = "image/jpeg"
	ImagePng  MimeType = "image/png"
	ImageWebp MimeType = "image/webp"

	ThumbnailSmol   ThumbnailSize = "100x100"
	ThumbnailMedium ThumbnailSize = "300x300"
)

type Avatar struct {
	ID        string    `json:"id" db:"avatars.id"`
	UserID    string    `json:"user_id" db:"avatar.user_id"`
	URL       string    `json:"url" db:"avatars.url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at" db:"avatars.created_at"`
}

type AvatarMetadata struct {
	ID         string      `json:"id" db:"avatars.id"`
	UserID     string      `json:"user_id" db:"avatars.user_id"`
	FileName   string      `json:"file_name" db:"avatars.file_name"`
	MimeType   MimeType    `json:"mime_type" db:"avatars.mime_type"`
	Size       int64       `json:"size_bytes" db:"size_bytes"`
	Dimensions Dimensions  `json:"dimensions"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	CreatedAt  string      `json:"created_at" db:"created_at"`
	UpdatedAt  string      `json:"updated_at" db:"updated_at"`
}

type Dimensions struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

type Thumbnail struct {
	Size ThumbnailSize `json:"size"`
	URL  string        `json:"url"`
}
