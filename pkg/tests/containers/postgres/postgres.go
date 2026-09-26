package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Radiushina/avatar-service/pkg/pgmigrator"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	initialDatabase = "initial_db"
	migrationsDir   = "internal/migrations/postgres"
)

var (
	singleton   sync.Once
	adminPool   *pgxpool.Pool
	baseConnStr string
	containerIP string
)

// New поднимает singleton Postgres (testcontainers), создаёт отдельную БД для теста,
// накатывает миграции и возвращает пул подключений к этой БД.
func New(t *testing.T) (*pgxpool.Pool, string, string) {
	t.Helper()

	singleton.Do(func() {
		ctx := context.WithoutCancel(t.Context())

		imageName := "postgres:16-alpine"
		if proxy := os.Getenv("CI_DEPENDENCY_PROXY_DIRECT_GROUP_IMAGE_PREFIX"); proxy != "" {
			imageName = proxy + "/" + imageName
		}

		pgContainer, err := postgres.Run(
			ctx,
			imageName,
			postgres.WithDatabase(initialDatabase),
			postgres.WithUsername("root"),
			postgres.WithPassword("qwerty"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(5*time.Minute),
				wait.ForListeningPort("5432/tcp"),
			),
			testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
				hostConfig.SecurityOpt = []string{"seccomp=unconfined", "apparmor=unconfined"} // for gitlab ci
			}),
			testcontainers.CustomizeRequestOption(func(req *testcontainers.GenericContainerRequest) error {
				req.Cmd = append(req.Cmd, "-c", "max_connections=500")
				return nil
			}),
		)
		require.NoError(t, err)

		t.Cleanup(func() {
			require.NoError(t, testcontainers.TerminateContainer(pgContainer))
		})

		containerIP, err = pgContainer.ContainerIP(ctx)
		require.NoError(t, err)

		baseConnStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
		require.NoError(t, err)

		adminPool, err = pgxpool.New(ctx, baseConnStr)
		require.NoError(t, err)

		require.NoError(t, adminPool.Ping(ctx))
	})

	dbName := "test_" + lo.RandomString(8, []rune("abcdefghijklmnopqrstuvwxyz"))
	ctx := t.Context()

	_, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q", dbName))
	require.NoError(t, err)

	testConnStr := replaceDatabaseName(baseConnStr, dbName)

	err = pgmigrator.Migrate(testConnStr, filepath.Join(projectRoot(t), migrationsDir), nil)
	require.NoError(t, err)

	testPool, err := pgxpool.New(ctx, testConnStr)
	require.NoError(t, err)
	require.NoError(t, testPool.Ping(ctx))

	t.Cleanup(func() {
		testPool.Close()

		cleanupCtx := context.WithoutCancel(ctx)
		_, err := adminPool.Exec(cleanupCtx, fmt.Sprintf("DROP DATABASE %q", dbName))
		require.NoError(t, err)
	})

	return testPool, testConnStr, containerIP
}

func replaceDatabaseName(connStr, dbName string) string {
	return strings.Replace(connStr, "/"+initialDatabase+"?", "/"+dbName+"?", 1)
}

func projectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("project root with go.mod not found")
		}
		dir = parent
	}
}
