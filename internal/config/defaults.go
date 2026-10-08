package config

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			HTTP: HTTPServerConfig{
				Address: ":8080",
			},
		},
		Log: LogConfig{
			Level: "info",
		},
		S3: S3Config{
			Region:    "us-east-1",
			PathStyle: true,
		},
		RabbitMQ: RabbitConfig{},
	}
}
