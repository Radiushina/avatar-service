package pgmigrator

import (
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // import pg driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"
)

const (
	_attempts = 20
	_timeout  = time.Second
)

func migrateLog(log *zap.Logger, msg string) {
	if log == nil {
		return
	}
	log.Info(msg)
}

func Migrate(dsn, migrationsPath string, log *zap.Logger) (err error) {
	var (
		attempts = _attempts
		m        *migrate.Migrate
	)

	for attempts > 0 {
		m, err = migrate.New("file://"+migrationsPath, dsn)
		if err == nil {
			break
		}
		migrateLog(log, "migrate: postgres is trying to connect")
		time.Sleep(_timeout)
		attempts--
	}
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	err = m.Up()
	defer func() {
		_, closeErr := m.Close()
		err = errors.Join(err, closeErr)
	}()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		migrateLog(log, "migrate: no change")
		return nil
	}

	migrateLog(log, "migrate: up success")
	return nil
}

func MigrateFromEmbeddedFS(fs embed.FS, migrationsPath string, dsn string, log *zap.Logger) (err error) {
	d, err := iofs.New(fs, migrationsPath)
	if err != nil {
		return fmt.Errorf("migrateFromEmbeddedFS: %w", err)
	}

	var (
		attempts = _attempts
		m        *migrate.Migrate
	)
	for attempts > 0 {
		m, err = migrate.NewWithSourceInstance("iofs", d, dsn)
		if err == nil {
			break
		}

		migrateLog(log, "migrate: postgres is trying to connect")
		time.Sleep(_timeout)
		attempts--
	}
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	err = m.Up()
	defer func() {
		_, closeErr := m.Close()
		err = errors.Join(err, closeErr)
	}()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		migrateLog(log, "migrate: no change")
		return nil
	}

	migrateLog(log, "migrate: up success")
	return nil
}
