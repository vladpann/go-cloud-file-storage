package users_transport_http

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestMigrationPath tests that the migrations path can be located correctly
func TestMigrationPath(t *testing.T) {
	path, err := getMigrationsPath()
	require.NoError(t, err, "Should be able to get migrations path")
	require.NotEmpty(t, path, "Migrations path should not be empty")
	t.Logf("Migrations path: %s", path)
}

// TestDatabaseConnection tests that we can connect to the database after container starts
func TestDatabaseConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tc := &TestContext{
		testID: generateTestID(),
	}

	// Start PostgreSQL container
	container, connString, err := tc.startPostgreSQLContainer(ctx)
	require.NoError(t, err, "PostgreSQL container should start successfully")
	require.NotNil(t, container, "Container instance should not be nil")
	require.NotEmpty(t, connString, "Connection string should not be empty")
	defer container.Terminate(context.Background())

	t.Logf("Connection string: %s", connString)

	// Wait a bit for database to be fully ready
	time.Sleep(2 * time.Second)

	// Try to connect
	pool, err := pgxpool.New(ctx, connString)
	require.NoError(t, err, "Should be able to create connection pool")
	defer pool.Close()

	// Ping database
	err = pool.Ping(ctx)
	require.NoError(t, err, "Should be able to ping database")

	t.Log("Successfully connected to database")
}

// TestMigrationApplication tests applying migrations to a running database
func TestMigrationApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tc := &TestContext{
		testID: generateTestID(),
	}

	// Start PostgreSQL container
	container, connString, err := tc.startPostgreSQLContainer(ctx)
	require.NoError(t, err, "PostgreSQL container should start successfully")
	defer container.Terminate(context.Background())

	t.Logf("Connection string: %s", connString)

	// Wait for database to be fully ready
	time.Sleep(2 * time.Second)

	// Verify we can connect first
	pool, err := pgxpool.New(ctx, connString)
	require.NoError(t, err, "Should be able to create connection pool")
	err = pool.Ping(ctx)
	require.NoError(t, err, "Should be able to ping database before migrations")
	pool.Close()

	// Now try to apply migrations
	err = tc.applyMigrations(ctx, connString)
	if err != nil {
		t.Logf("Migration error: %v", err)
	}
	require.NoError(t, err, "Should be able to apply migrations")

	// Verify we can still connect after migrations
	pool, err = pgxpool.New(ctx, connString)
	require.NoError(t, err, "Should be able to create connection pool after migrations")
	defer pool.Close()

	err = pool.Ping(ctx)
	require.NoError(t, err, "Should be able to ping database after migrations")

	// Verify table was created
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM cloud_file_storage.users").Scan(&count)
	require.NoError(t, err, "Should be able to query users table")
	require.Equal(t, 0, count, "Users table should be empty")

	t.Log("Migrations applied successfully")
}
