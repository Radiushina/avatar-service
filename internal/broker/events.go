package broker

const (
	Exchange    = "avatars.exchange"
	KeyUploaded = "avatar.uploaded"
	KeyDeleted  = "avatar.deleted"
)

type AvatarUploadEvent struct {
	AvatarID string `json:"avatar_id"`
	UserID   string `json:"user_id"`
	S3Key    string `json:"s3_key"`
}

type ProcessingOp struct {
	Type string `json:"type"`
}

type AvatarProcessEvent struct {
	AvatarID   string         `json:"avatar_id"`
	Operations []ProcessingOp `json:"operations"`
}

type AvatarDeleteEvent struct {
	AvatarID string   `json:"avatar_id"`
	S3Keys   []string `json:"s3_keys"`
}
