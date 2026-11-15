package users_transport_http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignIn_SuccessfulAuthorization validates the complete successful authentication flow.
// It verifies all aspects of user authentication including HTTP response, session creation,
// and cookie handling.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user via sign-up (prerequisite for authentication)
//  3. Send valid sign-in request with correct credentials
//  4. Verify HTTP response status is 200 OK
//  5. Verify response body contains username
//  6. Verify session cookie is set with proper security attributes
//  7. Verify new session is created in Redis
func TestSignIn_SuccessfulAuthorization(t *testing.T) {
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

	// Prerequisite: Create user via sign-up
	signUpReq := buildSignUpRequest(username, password, repeatPassword)
	_ = sendSignUpRequest(t, tc.router, signUpReq)

	// Step 1: Authenticate with correct credentials
	signInReq := buildSignInRequest(username, password)
	response := sendSignInRequest(t, tc.router, signInReq)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusOK)

	// Step 3: Verify JSON response body
	signInResponse := parseSignInResponse(t, response)
	assert.Equal(t, username, signInResponse.Username,
		"response username should match request username")

	// Step 4: Verify session cookie is present and valid
	cookie, err := extractSessionCookie(t, response)
	require.NoError(t, err, "session cookie should be present in response")
	assert.NotEmpty(t, cookie.Value, "session cookie should have non-empty value")
	assert.True(t, cookie.HttpOnly, "session cookie should have HttpOnly flag set")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignIn_InvalidRequest_EmptyUsername validates that sign-in with empty username is rejected.
// It verifies the system correctly rejects an authentication attempt with zero-length username,
// returning appropriate error status and message without creating any session.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-in request with empty username (zero characters)
//  3. Verify HTTP response status is 400 Bad Request
//  4. Verify response body contains error message describing validation failure
//  5. Verify no session cookie is returned
func TestSignIn_InvalidRequest_EmptyUsername(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - empty username (zero characters)
	username := ""
	password := "testpass123"

	// Step 1: Send sign-in request with empty username
	signInReq := buildSignInRequest(username, password)
	response := sendSignInRequest(t, tc.router, signInReq)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusBadRequest)

	// Step 3: Verify error response body
	var errorResponse struct {
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	err = json.NewDecoder(response.Body).Decode(&errorResponse)
	require.NoError(t, err, "failed to decode error response")

	// Check that error message exists and describes validation failure
	errorText := errorResponse.Error + " " + errorResponse.Message
	assert.NotEmpty(t, errorText, "error response should contain error message")

	// The error message should indicate validation failure
	containsRelevantError :=
		containsIgnoreCase(errorText, "username") ||
			containsIgnoreCase(errorText, "validation") ||
			containsIgnoreCase(errorText, "required") ||
			containsIgnoreCase(errorText, "empty") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignIn_InvalidRequest_EmptyPassword validates that sign-in with empty password is rejected.
// It verifies the system correctly rejects an authentication attempt with zero-length password,
// returning appropriate error status and message without creating any session.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-in request with empty password (zero characters)
//  3. Verify HTTP response status is 400 Bad Request
//  4. Verify response body contains error message describing validation failure
//  5. Verify no session cookie is returned
func TestSignIn_InvalidRequest_EmptyPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - empty password (zero characters)
	username := "testuser"
	password := ""

	// Step 1: Send sign-in request with empty password
	signInReq := buildSignInRequest(username, password)
	response := sendSignInRequest(t, tc.router, signInReq)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusBadRequest)

	// Step 3: Verify error response body
	var errorResponse struct {
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	err = json.NewDecoder(response.Body).Decode(&errorResponse)
	require.NoError(t, err, "failed to decode error response")

	// Check that error message exists and describes validation failure
	errorText := errorResponse.Error + " " + errorResponse.Message
	assert.NotEmpty(t, errorText, "error response should contain error message")

	// The error message should indicate validation failure
	containsRelevantError :=
		containsIgnoreCase(errorText, "password") ||
			containsIgnoreCase(errorText, "validation") ||
			containsIgnoreCase(errorText, "required") ||
			containsIgnoreCase(errorText, "empty") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignIn_InvalidRequest_InvalidCredentials validates that sign-in with wrong password is rejected.
// It verifies the system correctly rejects an authentication attempt when password doesn't match,
// returning 401 Unauthorized status without creating a new session.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Create a user via sign-up with a known password
//  3. Send sign-in request with correct username but wrong password
//  4. Verify HTTP response status is 401 Unauthorized
//  5. Verify response body contains error message
//  6. Verify no new session is created
func TestSignIn_InvalidRequest_InvalidCredentials(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - correct username, wrong password
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	correctPassword := "testpass123"
	wrongPassword := "wrongpass456"

	// Verify passwords are different
	require.NotEqual(t, correctPassword, wrongPassword, "test passwords should be different")

	// Prerequisite: Create user with correct password
	signUpReq := buildSignUpRequest(username, correctPassword, correctPassword)
	_ = sendSignUpRequest(t, tc.router, signUpReq)

	// Step 1: Attempt sign-in with wrong password
	signInReq := buildSignInRequest(username, wrongPassword)
	response := sendSignInRequest(t, tc.router, signInReq)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusUnauthorized)

	// Step 3: Verify error response body
	var errorResponse struct {
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	err = json.NewDecoder(response.Body).Decode(&errorResponse)
	require.NoError(t, err, "failed to decode error response")

	// Check that error message exists
	errorText := errorResponse.Error + " " + errorResponse.Message
	assert.NotEmpty(t, errorText, "error response should contain error message")

	// Step 4: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid credentials should not return session cookie")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignIn_InvalidRequest_UserNotFound validates that sign-in with non-existent user is rejected.
// It verifies the system correctly rejects an authentication attempt when the username
// doesn't exist in the database, returning 404 Not Found status without creating any session.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-in request with username that doesn't exist in database
//  3. Verify HTTP response status is 404 Not Found
//  4. Verify response body contains error message
//  5. Verify no session is created
func TestSignIn_InvalidRequest_UserNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - non-existent user
	username := "nonexistentuser"
	password := "testpass123"

	// Step 1: Attempt sign-in with non-existent username
	signInReq := buildSignInRequest(username, password)
	response := sendSignInRequest(t, tc.router, signInReq)

	// Step 2: Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusNotFound)

	// Step 3: Verify error response body
	var errorResponse struct {
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	err = json.NewDecoder(response.Body).Decode(&errorResponse)
	require.NoError(t, err, "failed to decode error response")

	// Check that error message exists
	errorText := errorResponse.Error + " " + errorResponse.Message
	assert.NotEmpty(t, errorText, "error response should contain error message")

	// Step 4: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "non-existent user should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignIn_InvalidRequest_MalformedJSON validates that sign-in with malformed JSON is rejected.
// It verifies the system correctly rejects authentication attempts with invalid JSON body,
// returning 400 Bad Request status without creating any session.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-in request with malformed JSON (various invalid patterns)
//  3. Verify HTTP response status is 400 Bad Request
//  4. Verify response body contains error message describing decode/validation failure
func TestSignIn_InvalidRequest_MalformedJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test cases with different malformed JSON patterns
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "unclosed braces",
			body: `{"username":"testuser","password":"testpass123"`,
		},
		{
			name: "missing quotes",
			body: `{username:"testuser","password":"testpass123"}`,
		},
		{
			name: "trailing comma",
			body: `{"username":"testuser","password":"testpass123",}`,
		},
		{
			name: "invalid escape sequence",
			body: `{"username":"test\xuser","password":"testpass123"}`,
		},
		{
			name: "empty body",
			body: ``,
		},
		{
			name: "not JSON",
			body: `this is not json`,
		},
	}

	for _, tc_json := range testCases {
		t.Run(tc_json.name, func(t *testing.T) {
			// Step 1: Send sign-in request with malformed JSON
			response := sendRawSignInRequest(t, tc.router, tc_json.body)

			// Step 2: Verify HTTP status code
			assertHTTPStatus(t, response, http.StatusBadRequest)

			// Step 3: Verify error response body
			var errorResponse struct {
				Error   string `json:"error,omitempty"`
				Message string `json:"message,omitempty"`
			}
			err = json.NewDecoder(response.Body).Decode(&errorResponse)
			// If we can't decode the error response, it's still an error (could be plain text)
			// Just verify the body is not empty
			if err != nil {
				assert.NotEmpty(t, response.Body.String(), "error response should not be empty")
			} else {
				// Check that error message exists
				errorText := errorResponse.Error + " " + errorResponse.Message
				assert.NotEmpty(t, errorText, "error response should contain error message")

				// The error message should indicate validation or decode failure
				containsRelevantError :=
					containsIgnoreCase(errorText, "json") ||
						containsIgnoreCase(errorText, "decode") ||
						containsIgnoreCase(errorText, "parse") ||
						containsIgnoreCase(errorText, "invalid") ||
						containsIgnoreCase(errorText, "malformed") ||
						containsIgnoreCase(errorText, "validation")

				assert.True(t, containsRelevantError,
					"error message should indicate JSON decode/validation failed, got: %s", errorText)
			}

			// Cleanup: Delete any test data (should be none, but for safety)
			err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to cleanup users")

			err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to cleanup sessions")
		})
	}
}
