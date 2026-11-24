package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMe_SuccessfulUserMe validates the complete successful /me flow.
// It verifies that authenticated user can successfully retrieve their username
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user and authenticate (sign-up + sign-in)
//  3. Send me request
//  4. Verify HTTP response status is 200 OK
func TestMe_SuccessfulUserMe(t *testing.T) {
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
	signUpResp := sendSignUpRequest(t, tc.router, signUpReq)
	assertHTTPStatus(t, signUpResp, http.StatusCreated)

	signInReq := buildSignInRequest(username, password)
	signInResp := sendSignInRequest(t, tc.router, signInReq)
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err, "session cookie should be present in response")

	// Step 1: Send me request
	response := sendMeRequest(t, tc.router, sessionCookie)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusOK)

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestMe_Unauthorized validates that an unauthenticated request to /me is rejected.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send me request without session cookies
//  3. Verify HTTP response status is 401 Unauthorized
func TestMe_Unauthorized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Step 1: Send me request without authentication cookies
	response := sendMeRequest(t, tc.router, nil)

	// Step 2: Verify HTTP status code is 401 Unauthorized
	assertHTTPStatus(t, response, http.StatusUnauthorized)
}

// TestMe_AfterSignOut validates that /me rejects requests with an invalidated session after sign-out.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user and authenticate (sign-up + sign-in)
//  3. Sign out using the received session cookies
//  4. Send me request with the old session cookies
//  5. Verify HTTP response status is 401 Unauthorized
func TestMe_AfterSignOut(t *testing.T) {
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
	signUpResp := sendSignUpRequest(t, tc.router, signUpReq)
	assertHTTPStatus(t, signUpResp, http.StatusCreated)

	signInReq := buildSignInRequest(username, password)
	signInResp := sendSignInRequest(t, tc.router, signInReq)
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err, "failed to extract session cookie")

	// Step 1: Sign out to invalidate the session in Redis
	signOutResp := sendSignOutRequest(t, tc.router, sessionCookie)
	assertHTTPStatus(t, signOutResp, http.StatusNoContent)

	// Step 2: Send me request with the invalidated session cookies
	response := sendMeRequest(t, tc.router, sessionCookie)

	// Step 3: Verify HTTP status code is 401 Unauthorized
	assertHTTPStatus(t, response, http.StatusUnauthorized)

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}
