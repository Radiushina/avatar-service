package di

import (
	"github.com/Radiushina/avatar-service/cmd/server/di/providers"
	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/domains/health"
	"github.com/google/wire"
)

var (
	ConfigSet = wire.NewSet(
		providers.NewConfig,
		providers.NewLogger,
	)

	InfraSet = wire.NewSet(
		providers.NewPostgres,

		providers.NewS3Store,
		wire.Bind(new(avatars.ObjectStore), new(*providers.S3Store)),
		wire.Bind(new(health.S3HealthCheckProvider), new(*providers.S3Store)),

		providers.NewRabbit,
		wire.Bind(new(health.BrokerHealthCheckProvider), new(*broker.Client)),

		providers.NewPublisher,
		wire.Bind(new(avatars.Publisher), new(*broker.Publisher)),

		avatars.NewAvatarRepo,
		wire.Bind(new(avatars.RepoProvider), new(*avatars.Repo)),

		providers.NewDBHealth,
		wire.Bind(new(health.DBHealthCheckProvider), new(*providers.Postgres)),
	)

	ServicesSet = wire.NewSet(
		avatars.NewService,
		wire.Bind(new(avatars.ServiceProvider), new(*avatars.Service)),
	)

	ServerSet = wire.NewSet(
		providers.NewHTTPServer,
		providers.NewServers,
	)
)

var AllSets = wire.NewSet(
	ConfigSet,
	InfraSet,
	ServicesSet,
	ServerSet,
)
