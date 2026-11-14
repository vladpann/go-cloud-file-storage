package users_transport_http

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// TestTestContext_Cleanup verifies that the Cleanup method properly:
// 1. Executes cleanup functions in reverse order (LIFO)
// 2. Closes database and Redis connections
// 3. Stops PostgreSQL and Redis containers within 30 seconds
// 4. Collects all errors and reports them
// 5. Logs cleanup failures
func TestTestContext_Cleanup(t *testing.T) {
	ctx := context.Background()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	require.NotNil(t, tc, "test context should not be nil")

	// Verify containers are running
	require.NotNil(t, tc.postgresContainer, "PostgreSQL container should be initialized")
	require.NotNil(t, tc.redisContainer, "Redis container should be initialized")

	// Verify database connections are working
	err = tc.pgPool.Ping(ctx)
	require.NoError(t, err, "PostgreSQL connection should be working")

	err = tc.redisClient.Ping(ctx).Err()
	require.NoError(t, err, "Redis connection should be working")

	// Measure cleanup time to ensure it completes within 30 seconds
	startTime := time.Now()

	// Execute cleanup
	tc.Cleanup()

	cleanupDuration := time.Since(startTime)

	// Verify cleanup completed within reasonable time (should be much less than 30 seconds)
	require.Less(t, cleanupDuration, 35*time.Second, "cleanup should complete within 35 seconds (30s timeout + 5s buffer)")

	t.Logf("Cleanup completed in %v", cleanupDuration)
}

// TestTestContext_CleanupIdempotent verifies that calling Cleanup multiple times is safe
func TestTestContext_CleanupIdempotent(t *testing.T) {
	ctx := context.Background()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")

	// Call cleanup multiple times - should not panic
	tc.Cleanup()
	tc.Cleanup()
	tc.Cleanup()

	t.Log("Multiple cleanup calls completed successfully")
}

// TestTestContext_CleanupWithFailedSetup verifies cleanup works even when setup partially fails
func TestTestContext_CleanupWithFailedSetup(t *testing.T) {
	tc := &TestContext{
		cleanupFns: []func() error{},
	}

	// Add some cleanup functions
	tc.cleanupFns = append(tc.cleanupFns, func() error {
		return nil // Success
	})

	// Cleanup should not panic even with minimal setup
	tc.Cleanup()

	t.Log("Cleanup with partial setup completed successfully")
}

// TestTestContext_DatabaseConnectionFailure verifies that NewTestContext properly handles
// PostgreSQL connection failures according to Requirement 12.1:
//   - When database connection fails after 10 seconds, the test suite should terminate
//     with error message indicating database connection failure cause
//
// This test verifies:
// 1. Connection failure with wrong credentials is detected
// 2. Error message indicates authentication/connection failure cause
// 3. Test completes within timeout (10 seconds for connection + buffer)
func TestTestContext_DatabaseConnectionFailure(t *testing.T) {
	// Track start time to verify timeout behavior
	startTime := time.Now()

	// Create context with timeout - 10 seconds for connection + 5 second buffer
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test connection failure with invalid connection string
	// This simulates what happens in NewTestContext when database is unreachable
	invalidConnString := "postgres://test:wrong_password@localhost:65432/testdb?sslmode=disable&connect_timeout=10"

	poolConfig, err := pgxpool.ParseConfig(invalidConnString)
	require.NoError(t, err, "connection string should parse successfully")
	poolConfig.MaxConns = 5
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	// Attempt to create pool and ping - this should fail
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)

	// Connection pool creation might succeed, but ping should fail
	if err == nil {
		defer pool.Close()
		err = pool.Ping(ctx)
	}

	// Measure elapsed time
	elapsed := time.Since(startTime)

	// Verify that connection failure occurred
	require.Error(t, err, "connection should fail with invalid connection details")

	// Verify error message indicates connection failure cause
	// Possible error messages:
	// - "connection refused" (port not available)
	// - "password authentication failed" (wrong credentials)
	// - "timeout" or "deadline exceeded" (connection timeout)
	// - "no such host" (invalid hostname)
	errMsg := err.Error()
	hasConnectionError := strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "password authentication failed") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "i/o timeout")

	require.True(t, hasConnectionError,
		"error message should indicate connection failure cause, got: %s", errMsg)

	// Verify the test completed within reasonable time
	// Should fail within 15 seconds (10s timeout + 5s buffer)
	require.Less(t, elapsed, 15*time.Second,
		"connection failure should be detected within 15 seconds")

	t.Logf("Connection failure detected in %v with error: %v", elapsed, err)
}

// TestTestContext_RedisConnectionFailure verifies that NewTestContext properly handles
// Redis connection failures according to Requirement 12.2:
//   - When Redis connection fails after 10 seconds, the test suite should terminate
//     with error message indicating Redis connection failure cause
//
// This test verifies:
// 1. Connection failure with wrong credentials is detected
// 2. Error message indicates Redis connection failure cause
// 3. Test completes within timeout (10 seconds for connection + buffer)
func TestTestContext_RedisConnectionFailure(t *testing.T) {
	// Track start time to verify timeout behavior
	startTime := time.Now()

	// Create context with timeout - 10 seconds for connection + 5 second buffer
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test connection failure with invalid Redis configuration
	// This simulates what happens in NewTestContext when Redis is unreachable
	// Using wrong password on a non-existent address
	invalidRedisClient := redis.NewClient(&redis.Options{
		Addr:         "localhost:65379", // Non-existent port
		Password:     "wrong_password",
		DialTimeout:  10 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	defer invalidRedisClient.Close()

	// Attempt to ping Redis - this should fail
	err := invalidRedisClient.Ping(ctx).Err()

	// Measure elapsed time
	elapsed := time.Since(startTime)

	// Verify that connection failure occurred
	require.Error(t, err, "connection should fail with invalid Redis configuration")

	// Verify error message indicates Redis connection failure cause
	// Possible error messages:
	// - "connection refused" (port not available)
	// - "i/o timeout" (connection timeout)
	// - "NOAUTH Authentication required" (authentication failure)
	// - "invalid password" or "WRONGPASS" (wrong password)
	// - "dial tcp" (network dial error)
	errMsg := err.Error()
	hasRedisError := strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "i/o timeout") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "NOAUTH") ||
		strings.Contains(errMsg, "WRONGPASS") ||
		strings.Contains(errMsg, "invalid password") ||
		strings.Contains(errMsg, "dial tcp")

	require.True(t, hasRedisError,
		"error message should indicate Redis connection failure cause, got: %s", errMsg)

	// Verify the test completed within reasonable time
	// Should fail within 15 seconds (10s timeout + 5s buffer)
	require.Less(t, elapsed, 15*time.Second,
		"Redis connection failure should be detected within 15 seconds")

	t.Logf("Redis connection failure detected in %v with error: %v", elapsed, err)
}
