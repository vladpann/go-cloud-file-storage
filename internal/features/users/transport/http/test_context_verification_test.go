package users_transport_http

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTestContext_PreTestStateVerification_CleanState validates that VerifyCleanState
// passes when the database and Redis are empty (no test data).
//
// This test verifies the pre-test state verification mechanism works correctly
// for the normal case where no data exists.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Call VerifyCleanState (should succeed - database and Redis are empty)
//  3. Verify no error is returned
func TestTestContext_PreTestStateVerification_CleanState(t *testing.T) {
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Call VerifyCleanState - should succeed because database and Redis are empty
	err = tc.VerifyCleanState(ctx)
	assert.NoError(t, err, "VerifyCleanState should pass when database and Redis are empty")
}

// TestTestContext_PreTestStateVerification_DatabaseNotEmpty validates that VerifyCleanState
// fails when the database contains users at test start.
//
// This test verifies the pre-test state verification mechanism correctly detects
// when the database is not in a clean state and aborts test execution.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Manually insert a user record into the database (simulating leftover data)
//  3. Call VerifyCleanState (should fail - database contains users)
//  4. Verify error is returned indicating verification failure
//  5. Verify error message mentions database contains users
func TestTestContext_PreTestStateVerification_DatabaseNotEmpty(t *testing.T) {
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Step 1: Manually insert a user into database to simulate leftover data
	baseUsername := "leftoveruser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	// Hash the password
	passwordHash, err := tc.passwordHasher.Hash(password)
	require.NoError(t, err, "failed to hash password")

	// Insert user directly into database
	query := `
		INSERT INTO cloud_file_storage.users (username, password_hash)
		VALUES ($1, $2)
	`
	_, err = tc.pgPool.Exec(ctx, query, username, passwordHash)
	require.NoError(t, err, "failed to insert test user")

	// Step 2: Call VerifyCleanState - should fail because database contains a user
	err = tc.VerifyCleanState(ctx)

	// Step 3: Verify error is returned
	assert.Error(t, err, "VerifyCleanState should fail when database contains users")

	// Step 4: Verify error message indicates database contains users
	if err != nil {
		errorMsg := err.Error()
		containsRelevantError :=
			containsIgnoreCase(errorMsg, "user") ||
				containsIgnoreCase(errorMsg, "database") ||
				containsIgnoreCase(errorMsg, "verification")

		assert.True(t, containsRelevantError,
			"error message should indicate database contains users, got: %s", errorMsg)
	}

	// Cleanup: Delete the test user
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")
}

// TestTestContext_PreTestStateVerification_RedisNotEmpty validates that VerifyCleanState
// fails when Redis contains sessions at test start.
//
// This test verifies the pre-test state verification mechanism correctly detects
// when Redis is not in a clean state and aborts test execution.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Manually insert a session record into Redis (simulating leftover data)
//  3. Call VerifyCleanState (should fail - Redis contains sessions)
//  4. Verify error is returned indicating verification failure
//  5. Verify error message mentions Redis contains sessions
func TestTestContext_PreTestStateVerification_RedisNotEmpty(t *testing.T) {
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Step 1: Manually insert a session into Redis to simulate leftover data
	// Use the session key format: "session:UUID"
	sessionKey := "session:00000000-0000-0000-0000-000000000001"
	sessionData := `{"user_id":"00000000-0000-0000-0000-000000000002","expires_at":"2025-01-01T00:00:00Z"}`

	err = tc.redisClient.Set(ctx, sessionKey, sessionData, 0).Err()
	require.NoError(t, err, "failed to insert test session")

	// Step 2: Call VerifyCleanState - should fail because Redis contains a session
	err = tc.VerifyCleanState(ctx)

	// Step 3: Verify error is returned
	assert.Error(t, err, "VerifyCleanState should fail when Redis contains sessions")

	// Step 4: Verify error message indicates Redis contains sessions
	if err != nil {
		errorMsg := err.Error()
		containsRelevantError :=
			containsIgnoreCase(errorMsg, "session") ||
				containsIgnoreCase(errorMsg, "redis") ||
				containsIgnoreCase(errorMsg, "verification")

		assert.True(t, containsRelevantError,
			"error message should indicate Redis contains sessions, got: %s", errorMsg)
	}

	// Cleanup: Delete the test session
	err = tc.redisClient.Del(ctx, sessionKey).Err()
	require.NoError(t, err, "failed to cleanup session")
}

// TestTestContext_PreTestStateVerification_BothNotEmpty validates that VerifyCleanState
// fails when both database and Redis contain data at test start.
//
// This test verifies the pre-test state verification mechanism correctly detects
// when both database and Redis are not in a clean state.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Manually insert a user into database and session into Redis
//  3. Call VerifyCleanState (should fail - both contain data)
//  4. Verify error is returned indicating verification failure
func TestTestContext_PreTestStateVerification_BothNotEmpty(t *testing.T) {
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Step 1: Insert user into database
	baseUsername := "leftoveruser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	passwordHash, err := tc.passwordHasher.Hash(password)
	require.NoError(t, err, "failed to hash password")

	query := `
		INSERT INTO cloud_file_storage.users (username, password_hash)
		VALUES ($1, $2)
	`
	_, err = tc.pgPool.Exec(ctx, query, username, passwordHash)
	require.NoError(t, err, "failed to insert test user")

	// Step 2: Insert session into Redis
	sessionKey := "session:00000000-0000-0000-0000-000000000001"
	sessionData := `{"user_id":"00000000-0000-0000-0000-000000000002","expires_at":"2025-01-01T00:00:00Z"}`

	err = tc.redisClient.Set(ctx, sessionKey, sessionData, 0).Err()
	require.NoError(t, err, "failed to insert test session")

	// Step 3: Call VerifyCleanState - should fail
	err = tc.VerifyCleanState(ctx)

	// Step 4: Verify error is returned
	// Note: The error should be from the first check (database), since we check in order
	assert.Error(t, err, "VerifyCleanState should fail when both database and Redis contain data")

	if err != nil {
		errorMsg := err.Error()
		// Error could be from database check or Redis check
		containsRelevantError :=
			containsIgnoreCase(errorMsg, "user") ||
				containsIgnoreCase(errorMsg, "session") ||
				containsIgnoreCase(errorMsg, "database") ||
				containsIgnoreCase(errorMsg, "redis") ||
				containsIgnoreCase(errorMsg, "verification")

		assert.True(t, containsRelevantError,
			"error message should indicate verification failed, got: %s", errorMsg)
	}

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = tc.redisClient.Del(ctx, sessionKey).Err()
	require.NoError(t, err, "failed to cleanup session")
}
