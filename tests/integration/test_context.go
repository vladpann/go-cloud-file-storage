package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/vladpann/go-cloud-file-storage/config"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	resources_repository_minio "github.com/vladpann/go-cloud-file-storage/internal/features/resources/repository/minio"
	resources_service "github.com/vladpann/go-cloud-file-storage/internal/features/resources/service"
	sessions_repository_redis "github.com/vladpann/go-cloud-file-storage/internal/features/sessions/repository/redis"
	sessions_service "github.com/vladpann/go-cloud-file-storage/internal/features/sessions/service"
	users_repository_postgres "github.com/vladpann/go-cloud-file-storage/internal/features/users/repository/postgres"
	user_service "github.com/vladpann/go-cloud-file-storage/internal/features/users/service"
	users_transport_http "github.com/vladpann/go-cloud-file-storage/internal/features/users/transport/http"
	"github.com/vladpann/go-cloud-file-storage/internal/testutil"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
	"github.com/vladpann/go-cloud-file-storage/pkg/minio"
	"github.com/vladpann/go-cloud-file-storage/pkg/postgres"
	pkg_redis "github.com/vladpann/go-cloud-file-storage/pkg/redis"
)

// migrationMutex ensures that goose migrations are applied sequentially
// to avoid data races in goose's SetDialect() function during parallel test execution
var migrationMutex sync.Mutex

// TestContext encapsulates all infrastructure and application dependencies needed for integration testing.
// It manages PostgreSQL and Redis test containers, database connections, and provides isolated test environments
// with unique test IDs for parallel test execution.
type TestContext struct {
	// Infrastructure
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container
	minioContainer    testcontainers.Container

	// Database connections
	pgPool      *pgxpool.Pool
	redisClient *redis.Client
	minioClient *minio.Client

	// Application dependencies
	config         *config.Config
	logger         logger.Interface
	validator      *validator.Validate
	passwordHasher user_service.PasswordHasher

	// Repositories
	usersRepo     *users_repository_postgres.UsersRepository
	sessionsRepo  *sessions_repository_redis.SessionsRepository
	resourcesRepo *resources_repository_minio.ResourcesRepository

	// Services
	usersService     *user_service.UsersService
	sessionsService  *sessions_service.SessionsService
	resourcesService *resources_service.ResourcesService

	// Handler and Router
	httpHandler *users_transport_http.UsersHTTPHandler
	router      http.Handler

	// Test metadata
	testID     string
	cleanupFns []func() error
}

// NewTestContext initializes a complete isolated test environment with PostgreSQL and Redis containers.
// It performs the following steps:
//  1. Generates unique test ID for data isolation
//  2. Starts PostgreSQL container (postgres:16-alpine)
//  3. Starts Redis container (redis:7-alpine)
//  4. Applies database migrations to test database
//  5. Creates database connection pool with test configuration
//  6. Creates Redis client with test configuration
//  7. Initializes logger and configuration
//  8. Registers cleanup functions
//
// The test context ensures complete isolation between parallel tests through unique test IDs
// and prefix-based data segregation.
//
// Container startup timeout: 30 seconds per container
// Returns error if container startup, migration, or connection fails.
func NewTestContext(ctx context.Context) (*TestContext, error) {
	tc := &TestContext{
		testID: generateTestID(),
	}

	// Start PostgreSQL container
	pgContainer, pgConnString, err := tc.startPostgreSQLContainer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}
	tc.postgresContainer = pgContainer
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return pgContainer.Terminate(terminateCtx)
	})

	// Start Redis container
	redisContainer, redisAddr, redisPassword, err := tc.startRedisContainer(ctx)
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to start Redis container: %w", err)
	}
	tc.redisContainer = redisContainer
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return redisContainer.Terminate(terminateCtx)
	})

	// Start MinIO container
	minioContainer, minioEndPoint, minioAccessKey, minioSecretKey, err := tc.startMinioContainer(ctx)
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to start MinIO container: %w", err)
	}
	tc.minioContainer = minioContainer
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return minioContainer.Terminate(terminateCtx)
	})

	// Apply database migrations
	if err := tc.applyMigrations(ctx, pgConnString); err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Create database connection pool
	poolConfig, err := pgxpool.ParseConfig(pgConnString)
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to parse PostgreSQL connection string: %w", err)
	}
	poolConfig.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to connect to test database: %w", err)
	}
	tc.pgPool = pool
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		pool.Close()
		return nil
	})

	// Verify database connection
	if err := pool.Ping(ctx); err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	// Create Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
	})
	tc.redisClient = redisClient
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		return redisClient.Close()
	})

	// Verify Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to ping test Redis: %w", err)
	}

	minioClient, err := minio.New(ctx, minio.Config{
		Endpoint:        minioEndPoint,
		AccessKeyID:     minioAccessKey,
		SecretAccessKey: minioSecretKey,
		UseSSL:          false,
		BucketName:      "test-files",
	})
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}
	tc.minioClient = minioClient

	// Create test configuration
	tc.config = testutil.NewTestConfig(pgConnString, redisAddr, redisPassword, minioAccessKey, minioSecretKey)

	// Initialize logger
	testLogger, err := logger.New(tc.config.Log.Level, tc.config.Log.Folder)
	if err != nil {
		tc.Cleanup()
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	tc.logger = testLogger

	// Initialize validator
	tc.validator = validator.New(validator.WithRequiredStructEnabled())

	// Initialize password hasher
	tc.passwordHasher = user_service.NewPasswordHasher()

	// Initialize repositories
	// Users repository - create Postgres wrapper with test pool
	pgWrapper := &postgres.Postgres{
		Builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
		Pool:    pool,
	}
	tc.usersRepo = users_repository_postgres.New(pgWrapper)

	// Sessions repository - create Redis wrapper with test client
	redisWrapper := &pkg_redis.Redis{
		Client: redisClient,
	}
	tc.sessionsRepo = sessions_repository_redis.New(redisWrapper)

	// Resources repository - use the ready-made *pkg_minio.Client
	tc.resourcesRepo = resources_repository_minio.New(minioClient)

	// Initialize services
	tc.usersService = user_service.New(tc.usersRepo, tc.passwordHasher)
	tc.sessionsService = sessions_service.New(tc.config, tc.sessionsRepo)
	tc.resourcesService = resources_service.New(tc.config, tc.resourcesRepo)

	// Initialize auth middleware
	authMiddleware := core_http_middleware.Auth(tc.logger, tc.sessionsService)

	// Initialize HTTP handler
	tc.httpHandler = users_transport_http.NewUsersHTTPHandler(
		tc.usersService,
		tc.sessionsService,
		tc.resourcesService,
		authMiddleware,
		tc.config,
		tc.logger,
		tc.validator,
	)

	// Initialize router for testing
	tc.router = tc.setupTestRouter()

	return tc, nil
}

// VerifyCleanState verifies that the database and Redis are in a clean state (no test data).
// This method should be called at the beginning of each test to ensure data isolation.
// It checks:
//  1. PostgreSQL contains zero users
//  2. Redis contains zero sessions
//
// Returns error if:
//   - Database query fails
//   - Redis query fails
//   - Database contains one or more users (verification failure)
//   - Redis contains one or more sessions (verification failure)
func (tc *TestContext) VerifyCleanState(ctx context.Context) error {
	// Check PostgreSQL for users
	userCount, err := countUsers(ctx, tc.pgPool, tc.testID)
	if err != nil {
		return fmt.Errorf("failed to count users during pre-test verification: %w", err)
	}

	if userCount > 0 {
		return fmt.Errorf("pre-test verification failed: database contains %d user(s), expected 0", userCount)
	}

	// Check Redis for sessions
	sessionCount, err := countSessions(ctx, tc.redisClient, tc.testID)
	if err != nil {
		return fmt.Errorf("failed to count sessions during pre-test verification: %w", err)
	}

	if sessionCount > 0 {
		return fmt.Errorf("pre-test verification failed: Redis contains %d session(s), expected 0", sessionCount)
	}

	return nil
}

// Cleanup performs complete cleanup including data deletion and container shutdown.
// It executes all registered cleanup functions in reverse order (LIFO).
// All errors are collected and logged, but cleanup continues even if individual steps fail.
// This ensures maximum cleanup even in failure scenarios.
// Container termination operations use a 30-second timeout.
//
// The method is idempotent - calling it multiple times is safe and will not cause panics.
// Expected errors from already-closed resources (like "client is closed" or "No such container")
// are filtered out to avoid noise in logs.
func (tc *TestContext) Cleanup() {
	var errs []error

	// Execute cleanup functions in reverse order (LIFO)
	for i := len(tc.cleanupFns) - 1; i >= 0; i-- {
		if err := tc.cleanupFns[i](); err != nil {
			// Filter out expected errors from already-closed resources
			if !isExpectedCleanupError(err) {
				errs = append(errs, err)
			}
		}
	}

	// Log all cleanup errors
	if len(errs) > 0 && tc.logger != nil {
		tc.logger.Error("cleanup errors occurred: %v", errs)
	}
}

// isExpectedCleanupError checks if an error is expected during cleanup
// (e.g., from already-closed connections or terminated containers)
func isExpectedCleanupError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// Redis client already closed
	if errMsg == "redis: client is closed" {
		return true
	}
	// Container already terminated
	if strings.Contains(errMsg, "No such container") {
		return true
	}
	return false
}

// startPostgreSQLContainer starts a PostgreSQL test container and returns the container instance
// and connection string. The container uses postgres:16-alpine image with test credentials.
// Wait strategy: waits for "database system is ready to accept connections" log message.
// Timeout: 30 seconds.
func (tc *TestContext) startPostgreSQLContainer(ctx context.Context) (testcontainers.Container, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", fmt.Errorf("failed to get PostgreSQL host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", fmt.Errorf("failed to get PostgreSQL port: %w", err)
	}

	connString := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Wait an additional moment for database to be fully ready for connections
	// The wait strategy ensures the log message appears, but the database may still
	// need a brief moment to accept connections reliably
	time.Sleep(1 * time.Second)

	return container, connString, nil
}

// startRedisContainer starts a Redis test container and returns the container instance,
// address, and password. The container uses redis:7-alpine image with test password.
// Wait strategy: waits for "Ready to accept connections" log message.
// Timeout: 30 seconds.
func (tc *TestContext) startRedisContainer(ctx context.Context) (container testcontainers.Container, addr, password string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	password = "testpass"
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		Cmd:          []string{"redis-server", "--requirepass", password},
		WaitingFor: wait.ForLog("Ready to accept connections").
			WithStartupTimeout(30 * time.Second),
	}

	container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to start Redis container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", "", fmt.Errorf("failed to get Redis host: %w", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", "", fmt.Errorf("failed to get Redis port: %w", err)
	}

	addr = fmt.Sprintf("%s:%s", host, port.Port())
	return container, addr, password, nil
}

// startMinioContainer starts a MinIO test container and returns the container instance,
// endpoint, access key, and secret key. The container uses cgr.dev/chainguard/minio:latest image.
// Wait strategy: waits for HTTP 200 OK on /minio/health/live endpoint.
// Timeout: 30 seconds.
func (tc *TestContext) startMinioContainer(ctx context.Context) (container testcontainers.Container, endpoint, accessKey, secretKey string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	accessKey = "minioadmin"
	secretKey = "minioadmin"

	req := testcontainers.ContainerRequest{
		Image:        "cgr.dev/chainguard/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     accessKey,
			"MINIO_ROOT_PASSWORD": secretKey,
		},
		WaitingFor: wait.ForHTTP("/minio/health/live").
			WithPort("9000/tcp").
			WithStartupTimeout(30 * time.Second),
	}

	container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to start MinIO container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", "", "", fmt.Errorf("failed to get MinIO host: %w", err)
	}

	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			err = errors.Join(err, termErr)
		}
		return nil, "", "", "", fmt.Errorf("failed to get MinIO port: %w", err)
	}

	endpoint = fmt.Sprintf("%s:%s", host, port.Port())
	return container, endpoint, accessKey, secretKey, nil
}

// getMigrationsPath locates the migrations directory relative to the test file location.
// It uses runtime.Caller to determine the current file path and navigates up to the project root.
// The migrations directory is expected at: <project_root>/migrations
// Returns error if migrations directory does not exist.
func getMigrationsPath() (string, error) {
	// Get the current file path using runtime.Caller
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("failed to determine test file location")
	}

	// Navigate from test file to project root
	// Current location: internal/features/users/transport/http/test_context.go
	// Navigate up 5 levels to reach project root
	testDir := filepath.Dir(filename)
	migrationsPath := filepath.Join(testDir, "..", "..", "..", "..", "..", "migrations")

	// Clean the path to resolve .. components
	migrationsPath = filepath.Clean(migrationsPath)

	// Verify migrations directory exists
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("migrations directory not found at %s", migrationsPath)
	}

	return migrationsPath, nil
}

// applyMigrations applies all database migrations to the test database using goose.
// It locates the migrations directory using getMigrationsPath and runs all up migrations.
// Returns detailed error messages indicating which migration failed.
//
// This function uses a mutex to ensure migrations are applied sequentially,
// avoiding data races in goose's global dialect state during parallel test execution.
//
// Migration application steps:
//  1. Locate migrations directory using runtime.Caller
//  2. Open database connection using standard library sql package
//  3. Run goose.Up() to apply all pending migrations (synchronized via mutex)
//
// Returns error if:
//   - Migrations directory cannot be located
//   - Database connection fails
//   - Migration application fails
func (tc *TestContext) applyMigrations(_ context.Context, databaseURL string) error {
	// Locate migrations directory
	migrationsPath, err := getMigrationsPath()
	if err != nil {
		return fmt.Errorf("failed to get migrations path: %w", err)
	}

	// Lock to prevent concurrent goose operations (goose has global state)
	migrationMutex.Lock()
	defer migrationMutex.Unlock()

	// Open database connection using standard library sql package
	// goose requires database/sql connection
	db, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close database connection: %w", closeErr))
		}
	}()

	// Apply all up migrations
	if err := goose.Up(db, migrationsPath); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// generateTestID creates a unique test identifier for data isolation in parallel test execution.
// Format: test_[timestamp_nanoseconds]_[random_uuid]
// Example: test_1234567890123456789_a1b2c3d4-e5f6-4a5b-8c7d-9e8f7a6b5c4d
//
// This test ID is used to:
//   - Prefix all usernames created during the test
//   - Prefix all session keys created during the test
//   - Filter data for cleanup operations
//
// The combination of timestamp and UUID ensures uniqueness even in high-concurrency scenarios.
func generateTestID() string {
	timestamp := time.Now().UnixNano()
	uuidStr := uuid.New().String()
	return fmt.Sprintf("test_%d_%s", timestamp, uuidStr)
}

// setupTestRouter creates a chi router with test routes registered.
// It mirrors the production routing structure with middleware.
// Routes registered:
//   - POST /api/v1/auth/sign-up (public)
//   - POST /api/v1/auth/sign-in (public)
//   - POST /api/v1/auth/sign-out (protected)
func (tc *TestContext) setupTestRouter() http.Handler {
	r := chi.NewRouter()

	// Register API v1 routes to match production structure
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			// Public routes
			r.Post("/sign-up", tc.httpHandler.SignUp)
			r.Post("/sign-in", tc.httpHandler.SignIn)

			// Protected route with auth middleware
			r.With(tc.httpHandler.AuthMiddleware()).Post("/sign-out", tc.httpHandler.SignOut)
		})

		r.Route("/user", func(r chi.Router) {
			r.With(tc.httpHandler.AuthMiddleware()).Get("/me", tc.httpHandler.Me)
		})
	})

	return r
}
