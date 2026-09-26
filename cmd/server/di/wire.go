//go:build wireinject
// +build wireinject

package di

import (
	"context"

	"github.com/google/wire"
)

// InjectApp uses Wire to construct the application with all dependencies.
// This function is implemented by Wire during code generation (wire_gen.go).
// It returns the initialized App, a cleanup function, and any initialization errors.
func InjectApp(ctx context.Context) (*App, func(), error) {
	wire.Build(
		AllSets,
		wire.Struct(new(App), "*"),
	)
	return &App{}, nil, nil
}
