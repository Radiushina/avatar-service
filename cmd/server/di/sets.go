package di

import (
	"github.com/Radiushina/avatar-service/cmd/server/di/providers"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/google/wire"
)

var (
	ConfigSet = wire.NewSet(
		providers.NewConfig,
		providers.NewLogger,
	)

	InfraSet = wire.NewSet(
		providers.NewPostgres,
		avatars.NewAvatarRepo,
		wire.Bind(new(avatars.RepoProvider), new(*avatars.Repo)),
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
