package migrations

import "embed"

// Postgres содержит SQL-миграции, вшитые в бинарник.
//
//go:embed postgres/*.sql
var Postgres embed.FS
