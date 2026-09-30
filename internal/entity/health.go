package entity

type HealthStatus string

const (
	Up   HealthStatus = "up"
	Down HealthStatus = "down"
)

type HealthCheck struct {
	Status     string     `json:"status"`
	Components Components `json:"components"`
}

type Components struct {
	Database string `json:"database"`
	S3       string `json:"s3"`
	Broker   string `json:"broker"`
}
