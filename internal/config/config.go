package config

type Config struct {
	Server   ServerConfig   `koanf:"server" yaml:"server"`
	Database DatabaseConfig `koanf:"database" yaml:"database"`
	Log      LogConfig      `koanf:"log" yaml:"log"`
	S3       S3Config       `koanf:"s3" yaml:"s3"`
	RabbitMQ RabbitConfig   `koanf:"rabbitmq" yaml:"rabbitmq"`
}

type ServerConfig struct {
	HTTP HTTPServerConfig `koanf:"http" yaml:"http"`
}

type HTTPServerConfig struct {
	Address string `koanf:"address" yaml:"address" env:"SERVER_HTTP_ADDRESS" flag:"server-http-address"`
}

type DatabaseConfig struct {
	DSN string `koanf:"dsn" yaml:"dsn" env:"DATABASE_DSN" flag:"database-dsn"`
}

type LogConfig struct {
	Level string `koanf:"level" yaml:"level" env:"LOG_LEVEL" flag:"log-level"`
}

type S3Config struct {
	Endpoint  string `koanf:"endpoint" yaml:"endpoint" env:"S3_ENDPOINT" flag:"s3-endpoint"`
	Region    string `koanf:"region" yaml:"region" env:"S3_REGION" flag:"s3-region"`
	Bucket    string `koanf:"bucket" yaml:"bucket" env:"S3_BUCKET" flag:"s3-bucket"`
	AccessKey string `koanf:"access_key" yaml:"access_key" env:"S3_ACCESS_KEY" flag:"s3-access-key"`
	SecretKey string `koanf:"secret_key" yaml:"secret_key" env:"S3_SECRET_KEY" flag:"s3-secret-key"`
	PathStyle bool   `koanf:"path_style" yaml:"path_style" env:"S3_PATH_STYLE" flag:"s3-path-style"`
}

type RabbitConfig struct {
	URL string `koanf:"url" yaml:"url" env:"RABBITMQ_URL" flag:"rabbitmq-url"`
}
