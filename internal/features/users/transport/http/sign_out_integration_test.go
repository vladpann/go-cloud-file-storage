package users_transport_http

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignOut_SuccessfulLogout validates the complete successful logout flow.
// It verifies that authenticated user can successfully sign out, session is deleted,
// and session cookie is cleared.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user and authenticate (sign-up + sign-in)
//  3. Send sign-out request with valid session cookie
//  4. Verify HTTP response status is 204 No Content
//  5. Verify session is deleted from Redis
//  6. Verify session cookie is cleared (MaxAge=-1)
func TestSignOut_SuccessfulLogout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - prefix with test ID for isolation
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := "testpass123"

	// Prerequisite: Create user and authenticate
	signUpReq := buildSignUpRequest(username, password, repeatPassword)
	_ = sendSignUpRequest(t, tc.router, signUpReq)

	signInReq := buildSignInRequest(username, password)
	signInResponse := sendSignInRequest(t, tc.router, signInReq)

	// Extract session cookie from sign-in response
	sessionCookie, err := extractSessionCookie(t, signInResponse)
	require.NoError(t, err, "session cookie should be present after sign-in")

	sessionID, err := parseUUID(sessionCookie.Value)
	require.NoError(t, err, "session cookie value should be valid UUID")

	// Verify session exists before sign-out
	session, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
	require.NoError(t, err, "session should exist in Redis before sign-out")
	require.NotNil(t, session, "session should not be nil")

	// Step 1: Send sign-out request
	response := sendSignOutRequest(t, tc.router, sessionCookie)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusNoContent)

	// Step 3: Verify session is deleted from Redis
	deletedSession, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
	require.NoError(t, err, "query should not return error")
	assert.Nil(t, deletedSession, "session should not exist in Redis after sign-out")

	// Step 4: Verify session cookie is cleared
	cookies := response.Result().Cookies()
	var clearedCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			clearedCookie = cookie
			break
		}
	}

	require.NotNil(t, clearedCookie, "response should contain session cookie")
	assert.Equal(t, "", clearedCookie.Value, "cleared cookie should have empty value")
	assert.Equal(t, -1, clearedCookie.MaxAge, "cleared cookie should have MaxAge=-1")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignOut_MissingSessionCookie validates that sign-out without session cookie is rejected by auth middleware.
// It verifies the system correctly handles logout attempt without authentication,
// returning 401 Unauthorized status from the auth middleware.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-out request without session cookie
//  3. Verify HTTP response status is 401 Unauthorized (from auth middleware)
func TestSignOut_MissingSessionCookie(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Step 1: Send sign-out request without session cookie
	response := sendSignOutRequest(t, tc.router, nil)

	// Step 2: Verify HTTP status code
	// Note: Auth middleware returns 401 when cookie is missing
	assertHTTPStatus(t, response, http.StatusUnauthorized)

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignOut_InvalidSessionID validates that sign-out with invalid session ID is rejected by auth middleware.
// It verifies the system correctly handles logout attempt with malformed session ID,
// returning 401 Unauthorized status from the auth middleware.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-out request with invalid (non-UUID) session cookie value
//  3. Verify HTTP response status is 401 Unauthorized (from auth middleware)
func TestSignOut_InvalidSessionID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - invalid session ID (not a UUID)
	invalidSessionID := "invalid-session-id-not-uuid"

	// Step 1: Send sign-out request with invalid session ID
	response := sendSignOutRequestWithRawCookie(t, tc.router, sessionCookieName, invalidSessionID)

	// Step 2: Verify HTTP status code
	// Note: Auth middleware returns 401 when session ID format is invalid
	assertHTTPStatus(t, response, http.StatusUnauthorized)

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignOut_NonExistentSession validates that sign-out with non-existent session is rejected by auth middleware.
// It verifies the system correctly handles logout attempt with valid UUID but non-existent session,
// returning 401 Unauthorized status from the auth middleware (session not found in Redis).
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-out request with valid UUID but non-existent session
//  3. Verify HTTP response status is 401 Unauthorized (from auth middleware)
func TestSignOut_NonExistentSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - valid UUID but non-existent session
	nonExistentSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Step 1: Send sign-out request with non-existent session ID
	response := sendSignOutRequestWithRawCookie(t, tc.router, sessionCookieName, nonExistentSessionID)

	// Step 2: Verify HTTP status code
	// Note: Auth middleware returns 401 when session doesn't exist in Redis
	assertHTTPStatus(t, response, http.StatusUnauthorized)

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignOut_AlreadySignedOut validates that signing out twice is rejected by auth middleware.
// It verifies that attempting to sign out with an already deleted session returns 401 Unauthorized
// from the auth middleware (session no longer exists in Redis).
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user and authenticate (sign-up + sign-in)
//  3. Sign out successfully (first time)
//  4. Attempt to sign out again with same session cookie
//  5. Verify HTTP response status is 401 Unauthorized (from auth middleware)
func TestSignOut_AlreadySignedOut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - prefix with test ID for isolation
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := "testpass123"

	// Prerequisite: Create user and authenticate
	signUpReq := buildSignUpRequest(username, password, repeatPassword)
	_ = sendSignUpRequest(t, tc.router, signUpReq)

	signInReq := buildSignInRequest(username, password)
	signInResponse := sendSignInRequest(t, tc.router, signInReq)

	// Extract session cookie from sign-in response
	sessionCookie, err := extractSessionCookie(t, signInResponse)
	require.NoError(t, err, "session cookie should be present after sign-in")

	// Step 1: Sign out successfully (first time)
	firstSignOutResponse := sendSignOutRequest(t, tc.router, sessionCookie)
	assertHTTPStatus(t, firstSignOutResponse, http.StatusNoContent)

	// Step 2: Attempt to sign out again with same session cookie
	secondSignOutResponse := sendSignOutRequest(t, tc.router, sessionCookie)

	// Step 3: Verify HTTP status code
	// Note: Auth middleware returns 401 when session is already deleted
	assertHTTPStatus(t, secondSignOutResponse, http.StatusUnauthorized)

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}
