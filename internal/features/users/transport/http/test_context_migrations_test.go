package users_transport_http

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetMigrationsPath verifies that getMigrationsPath correctly locates the migrations directory
// using runtime.Caller and returns a valid path.
func TestGetMigrationsPath(t *testing.T) {
	path, err := getMigrationsPath()
	require.NoError(t, err, "getMigrationsPath should successfully locate migrations directory")
	assert.NotEmpty(t, path, "migrations path should not be empty")
	assert.Contains(t, path, "migrations", "path should contain 'migrations'")
}

// TestApplyMigrations verifies that applyMigrations successfully applies all database migrations
// to a test database and handles ErrNoChange gracefully when re-applied.
func TestApplyMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context with PostgreSQL container
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "NewTestContext should initialize without error")
	defer tc.Cleanup()

	// Migrations are already applied in NewTestContext, so we verify the database state
	// by checking if the users table exists in cloud_file_storage schema
	var tableExists bool
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'cloud_file_storage' 
			AND table_name = 'users'
		);
	`
	err = tc.pgPool.QueryRow(ctx, query).Scan(&tableExists)
	require.NoError(t, err, "should be able to query table existence")
	assert.True(t, tableExists, "users table should exist in cloud_file_storage schema after migrations")

	// Test applying migrations again - should handle ErrNoChange gracefully
	host, err := tc.postgresContainer.Host(ctx)
	require.NoError(t, err)
	port, err := tc.postgresContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	connString := "postgres://test:test@" + host + ":" + port.Port() + "/testdb?sslmode=disable"
	err = tc.applyMigrations(ctx, connString)
	assert.NoError(t, err, "applyMigrations should handle ErrNoChange gracefully")
}
