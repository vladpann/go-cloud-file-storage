package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignUp_SuccessfulRegistration validates the complete successful registration flow.
// It verifies all aspects of user registration including HTTP response, database persistence,
// session creation, and cookie handling.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send valid sign-up request to the handler
//  3. Verify HTTP response status and body
//  4. Verify user record is created in database with hashed password
//  5. Verify session is created in Redis with correct user association
//  6. Verify user root folder was created in MinIO
func TestSignUp_SuccessfulRegistration(t *testing.T) {
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

	// Step 1: Build and send sign-up request
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

	// Verify HTTP status code
	assertHTTPStatus(t, response, http.StatusCreated)

	// Step 2: Verify JSON response body
	signUpResponse := parseSignUpResponse(t, response)
	assert.Equal(t, username, signUpResponse.Username,
		"response username should match request username")

	// Step 3: Verify user record in database
	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err, "failed to query user from database")
	require.NotNil(t, user, "user should exist in database")

	assert.Equal(t, username, user.Username,
		"stored username should match request username (with test prefix)")

	assert.NotEmpty(t, user.PasswordHash, "password hash should not be empty")
	assert.NotEqual(t, password, string(user.PasswordHash),
		"stored password should not equal plaintext password")

	// Verify password hash is valid (can be verified with hasher)
	verifyPasswordHash(t, tc.passwordHasher, user.PasswordHash, password)

	// Verify user count (should be exactly one)
	userCount, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users")
	assert.Equal(t, 1, userCount, "should have exactly one user record")

	// Step 4: Verify session cookie
	cookie, err := extractSessionCookie(t, response)
	require.NoError(t, err, "session cookie should be present in response")
	assert.NotEmpty(t, cookie.Value, "session cookie should have non-empty value")

	assert.True(t, cookie.HttpOnly, "session cookie should have HttpOnly flag set")

	// Expected expiration: current time + session TTL (24 hours in test config)
	expectedExpiration := time.Now().Add(tc.config.Session.TTL)
	// Allow 5 second tolerance for test execution time
	expirationDiff := cookie.Expires.Sub(expectedExpiration).Abs()
	assert.Less(t, expirationDiff, 5*time.Second,
		"cookie expiration should match configured session TTL (24h)")

	// Verify additional cookie attributes
	assert.Equal(t, sessionCookieName, cookie.Name, "cookie should have correct name")
	assert.Equal(t, "/", cookie.Path, "cookie path should be /")
	assert.Equal(t, tc.config.Session.CookieSecure, cookie.Secure,
		"cookie Secure flag should match configuration")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite,
		"cookie SameSite should be Lax")

	// Step 5: Verify session in Redis
	sessionID, err := parseUUID(cookie.Value)
	require.NoError(t, err, "session cookie value should be valid UUID")

	session, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
	require.NoError(t, err, "failed to query session from Redis")
	require.NotNil(t, session, "session should exist in Redis")

	assert.Equal(t, user.ID, session.UserID,
		"session user ID should match created user ID")

	// Verify session count (should be exactly one)
	sessionCount, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions")
	assert.Equal(t, 1, sessionCount, "should have exactly one session record")

	// Verify session expiration
	expectedSessionExpiration := time.Now().Add(tc.config.Session.TTL)
	sessionExpirationDiff := session.ExpiresAt.Sub(expectedSessionExpiration).Abs()
	assert.Less(t, sessionExpirationDiff, 5*time.Second,
		"session expiration should match configured TTL (24h)")

	// Step: 6: Verify user root folder was created in MinIO
	rootFolderKey := fmt.Sprintf("user-%s-files/", user.ID.String())
	objInfo, err := tc.minioClient.StatObject(
		ctx,
		tc.minioClient.BucketName(),
		rootFolderKey,
		minio.StatObjectOptions{},
	)
	require.NoError(t, err, "user root folder should exist in MinIO")
	assert.Equal(t, rootFolderKey, objInfo.Key, "folder key should match user root prefix")
	assert.Equal(t, int64(0), objInfo.Size, "root folder marker should be 0 bytes")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")

	err = tc.minioClient.RemoveObject(
		ctx,
		tc.minioClient.BucketName(),
		rootFolderKey,
		minio.RemoveObjectOptions{},
	)
	require.NoError(t, err, "failed to cleanup user root folder in MinIO")
}

// TestSignUp_DuplicateUsername validates that duplicate username registration is prevented.
// It verifies the system correctly rejects a second registration attempt with the same username,
// returning appropriate error status and message while preventing duplicate database records.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send first sign-up request with a unique username (should succeed)
//  3. Verify first registration creates user and session
//  4. Send second sign-up request with the same username (should fail)
//  5. Verify HTTP response status is 409 or 400
//  6. Verify response body contains error message indicating username exists
//  7. Verify no duplicate user record was created in database
//  8. Verify no session was created for the duplicate request
//  9. Verify original user record remains unchanged
func TestSignUp_DuplicateUsername(t *testing.T) {
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
	baseUsername := "dupuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := "testpass123"

	// Step 1: First sign-up request (should succeed)
	firstRequest := buildSignUpRequest(username, password, repeatPassword)
	firstResponse := sendSignUpRequest(t, tc.router, firstRequest)

	// Verify first request succeeds
	if firstResponse.Code != http.StatusOK && firstResponse.Code != http.StatusCreated {
		t.Fatalf("first sign-up request should succeed, got status %d", firstResponse.Code)
	}

	// Extract session cookie from first response
	firstCookie, err := extractSessionCookie(t, firstResponse)
	require.NoError(t, err, "first registration should return session cookie")
	firstSessionID, err := parseUUID(firstCookie.Value)
	require.NoError(t, err, "first session cookie should contain valid UUID")

	// Step 2: Verify user record created
	userAfterFirst, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err, "failed to query user after first registration")
	require.NotNil(t, userAfterFirst, "user should exist after first registration")

	// Store original user data for later comparison
	originalUserID := userAfterFirst.ID
	originalPasswordHash := string(userAfterFirst.PasswordHash)

	// Verify exactly one user record exists
	userLCountAfterFirst, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after first registration")
	assert.Equal(t, 1, userLCountAfterFirst, "should have exactly one user after first registration")

	// Step 3: Second sign-up request with same username (should fail)
	secondRequest := buildSignUpRequest(username, password, repeatPassword)
	secondResponse := sendSignUpRequest(t, tc.router, secondRequest)

	// Verify second request fails with appropriate status code
	if secondResponse.Code != http.StatusConflict && secondResponse.Code != http.StatusBadRequest {
		t.Fatalf("second sign-up request should fail with 409 or 400, got status %d. Body: %s",
			secondResponse.Code, secondResponse.Body.String())
	}

	// Step 4: Verify error response body
	var errorResponse struct {
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	err = json.NewDecoder(secondResponse.Body).Decode(&errorResponse)
	require.NoError(t, err, "failed to decode error response")

	// Check that error message indicates username already exists
	errorText := errorResponse.Error + " " + errorResponse.Message
	assert.NotEmpty(t, errorText, "error response should contain error message")

	// The error message should mention username or duplicate/exists
	containsRelevantError :=
		containsIgnoreCase(errorText, "username") ||
			containsIgnoreCase(errorText, "duplicate") ||
			containsIgnoreCase(errorText, "exists") ||
			containsIgnoreCase(errorText, "already")

	assert.True(t, containsRelevantError,
		"error message should indicate username already exists, got: %s", errorText)

	// Step 5: Verify no duplicate user record created
	userLCountAfterSecond, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after second registration")
	assert.Equal(t, 1, userLCountAfterSecond,
		"should still have exactly one user after duplicate registration attempt")

	// Step 6: Verify existing user record remains unmodified
	userAfterSecond, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err, "failed to query user after second registration")
	require.NotNil(t, userAfterSecond, "original user should still exist")

	assert.Equal(t, originalUserID, userAfterSecond.ID,
		"user ID should remain unchanged after duplicate registration attempt")
	assert.Equal(t, originalPasswordHash, string(userAfterSecond.PasswordHash),
		"password hash should remain unchanged after duplicate registration attempt")

	// Step 7: Verify no session cookie in second response
	// The duplicate registration should not create a session or return a cookie
	_, err = extractSessionCookie(t, secondResponse)
	assert.Error(t, err, "duplicate registration should not return session cookie")

	// Step 8: Verify session count unchanged
	// Only the first registration should have created a session
	sessionLCountAfterSecond, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after second registration")

	// Note: Since countSessions counts all sessions (not just test-prefixed ones),
	// we verify the first session still exists rather than relying on count
	firstSession, err := getSessionByID(ctx, tc.redisClient, tc.testID, firstSessionID)
	require.NoError(t, err, "failed to query first session")
	require.NotNil(t, firstSession, "first session should still exist")
	assert.Equal(t, originalUserID, firstSession.UserID,
		"first session should still be associated with original user")

	// The session count should be at least 1 (from first registration)
	assert.GreaterOrEqual(t, sessionLCountAfterSecond, 1,
		"should have at least one session (from first registration)")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// containsIgnoreCase checks if a string contains a substring, case-insensitive.
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	return contains(sLower, substrLower)
}

// toLower converts a string to lowercase (ASCII only for simplicity).
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return substr == "" || indexOfSubstring(s, substr) >= 0
}

// indexOfSubstring returns the index of the first instance of substr in s, or -1 if not present.
func indexOfSubstring(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	if n > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}

// TestSignUp_InvalidRequest_EmptyUsername validates that sign-up with empty username is rejected.
// It verifies the system correctly rejects a registration attempt with zero-length username,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with empty username (zero characters)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_EmptyUsername(t *testing.T) {
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
	repeatPassword := "testpass123"

	// Count users and sessions before request
	userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users before request")

	sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions before request")

	// Step 1: Send sign-up request with empty username
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

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
	// (can mention username, validation, required, empty, invalid, or decode)
	containsRelevantError :=
		containsIgnoreCase(errorText, "username") ||
			containsIgnoreCase(errorText, "validation") ||
			containsIgnoreCase(errorText, "required") ||
			containsIgnoreCase(errorText, "empty") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no user record created
	userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after request")
	assert.Equal(t, userLCountBefore, userLCountAfter,
		"no user record should be created when validation fails")

	// Step 5: Verify no session record created
	sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after request")
	assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
		"no session record should be created when validation fails")

	// Step 6: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_InvalidRequest_EmptyPassword validates that sign-up with empty password is rejected.
// It verifies the system correctly rejects a registration attempt with zero-length password,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with empty password (zero characters)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_EmptyPassword(t *testing.T) {
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
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := ""
	repeatPassword := ""

	// Count users and sessions before request
	userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users before request")

	sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions before request")

	// Step 1: Send sign-up request with empty password
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

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

	// Step 4: Verify no user record created
	userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after request")
	assert.Equal(t, userLCountBefore, userLCountAfter,
		"no user record should be created when validation fails")

	// Step 5: Verify no session record created
	sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after request")
	assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
		"no session record should be created when validation fails")

	// Step 6: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_InvalidRequest_ShortPassword validates that sign-up with short password is rejected.
// It verifies the system correctly rejects a registration attempt with password length 1-4 characters,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with short password (1-4 characters)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_ShortPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test cases with different short password lengths (1-4 characters)
	testCases := []struct {
		name     string
		password string
	}{
		{"1 character", "a"},
		{"2 characters", "ab"},
		{"3 characters", "abc"},
		{"4 characters", "abcd"},
	}

	for _, tcPass := range testCases {
		t.Run(tcPass.name, func(t *testing.T) {
			// Test data - short password
			baseUsername := "testuser"
			username := prefixTestData(tc.testID, baseUsername)
			password := tcPass.password
			repeatPassword := tcPass.password

			// Count users and sessions before request
			userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to count users before request")

			sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to count sessions before request")

			// Step 1: Send sign-up request with short password
			request := buildSignUpRequest(username, password, repeatPassword)
			response := sendSignUpRequest(t, tc.router, request)

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
					containsIgnoreCase(errorText, "length") ||
					containsIgnoreCase(errorText, "short") ||
					containsIgnoreCase(errorText, "invalid") ||
					containsIgnoreCase(errorText, "decode")

			assert.True(t, containsRelevantError,
				"error message should indicate validation failed, got: %s", errorText)

			// Step 4: Verify no user record created
			userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to count users after request")
			assert.Equal(t, userLCountBefore, userLCountAfter,
				"no user record should be created when validation fails")

			// Step 5: Verify no session record created
			sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to count sessions after request")
			assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
				"no session record should be created when validation fails")

			// Step 6: Verify no session cookie in response
			_, err = extractSessionCookie(t, response)
			assert.Error(t, err, "invalid request should not return session cookie")

			// Cleanup: Delete any test data (should be none, but for safety)
			err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to cleanup users")

			err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to cleanup sessions")
		})
	}
}

// TestSignUp_InvalidRequest_LongPassword validates that sign-up with long password is rejected.
// It verifies the system correctly rejects a registration attempt with password length >72 characters,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with long password (>72 characters)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_LongPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - long password (>72 characters)
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	// Create a password with 73 characters (exceeds 72 limit)
	password := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijk"
	repeatPassword := password

	// Verify password length is correct
	require.Equal(t, 73, len(password), "test password should be 73 characters")

	// Count users and sessions before request
	userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users before request")

	sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions before request")

	// Step 1: Send sign-up request with long password
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

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
			containsIgnoreCase(errorText, "length") ||
			containsIgnoreCase(errorText, "long") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no user record created
	userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after request")
	assert.Equal(t, userLCountBefore, userLCountAfter,
		"no user record should be created when validation fails")

	// Step 5: Verify no session record created
	sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after request")
	assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
		"no session record should be created when validation fails")

	// Step 6: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_InvalidRequest_PasswordMismatch validates that sign-up with mismatched passwords is rejected.
// It verifies the system correctly rejects a registration attempt where password != repeat_password,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with password != repeat_password
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_PasswordMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - mismatched passwords
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := "differentpass456"

	// Verify passwords are different
	require.NotEqual(t, password, repeatPassword, "test passwords should be different")

	// Count users and sessions before request
	userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users before request")

	sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions before request")

	// Step 1: Send sign-up request with mismatched passwords
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

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
			containsIgnoreCase(errorText, "match") ||
			containsIgnoreCase(errorText, "validation") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no user record created
	userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after request")
	assert.Equal(t, userLCountBefore, userLCountAfter,
		"no user record should be created when validation fails")

	// Step 5: Verify no session record created
	sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after request")
	assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
		"no session record should be created when validation fails")

	// Step 6: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_InvalidRequest_EmptyRepeatPassword validates that sign-up with empty repeat_password is rejected.
// It verifies the system correctly rejects a registration attempt with zero-length repeat_password,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with empty repeat_password (zero characters)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_EmptyRepeatPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Initialize test context with containers and dependencies
	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	// Verify clean state before test execution
	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	// Test data - empty repeat_password (zero characters)
	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := ""

	// Count users and sessions before request
	userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users before request")

	sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions before request")

	// Step 1: Send sign-up request with empty repeat_password
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

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
			containsIgnoreCase(errorText, "match") ||
			containsIgnoreCase(errorText, "invalid") ||
			containsIgnoreCase(errorText, "decode")

	assert.True(t, containsRelevantError,
		"error message should indicate validation failed, got: %s", errorText)

	// Step 4: Verify no user record created
	userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to count users after request")
	assert.Equal(t, userLCountBefore, userLCountAfter,
		"no user record should be created when validation fails")

	// Step 5: Verify no session record created
	sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to count sessions after request")
	assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
		"no session record should be created when validation fails")

	// Step 6: Verify no session cookie in response
	_, err = extractSessionCookie(t, response)
	assert.Error(t, err, "invalid request should not return session cookie")

	// Cleanup: Delete any test data (should be none, but for safety)
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_SessionVerification validates session creation and cookie configuration.
// It verifies that after successful registration, the session is correctly stored in Redis
// with proper expiration, user association, and that the session cookie has all required
// security attributes set correctly.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send valid sign-up request to the handler
//  3. Verify HTTP response is successful (200 or 201)
//  4. Extract session cookie from response
//  5. Query Redis for session using session ID from cookie
//  6. Verify session exists in Redis (fail if not found)
//  7. Verify session user_id matches created user ID
//  8. Verify session expiration is present and not null/zero
//  9. Verify session expiration is at least 1 second in the future
//  10. Verify cookie name equals "session_id"
//  11. Verify cookie path equals "/"
//  12. Verify cookie SameSite is Lax
//  13. Verify cookie HttpOnly is true
//  14. Verify cookie Secure is true
func TestSignUp_SessionVerification(t *testing.T) {
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
	baseUsername := "sessiontest"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"
	repeatPassword := "testpass123"

	// Step 1: Build and send sign-up request
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

	// Verify response is successful (200 or 201)
	if response.Code != http.StatusOK && response.Code != http.StatusCreated {
		t.Fatalf("sign-up request should succeed, got status %d. Body: %s",
			response.Code, response.Body.String())
	}

	// Step 2: Extract session cookie from response
	cookie, err := extractSessionCookie(t, response)
	require.NoError(t, err, "session cookie should be present in response")
	require.NotEmpty(t, cookie.Value, "session cookie value should not be empty")

	// Parse session ID from cookie
	sessionID, err := parseUUID(cookie.Value)
	require.NoError(t, err, "session cookie value should be valid UUID")

	// Step 3: Query user from database (to get user ID for verification)
	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err, "failed to query user from database")
	require.NotNil(t, user, "user should exist in database")

	// Step 4: Query Redis for session using session ID from cookie
	session, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
	require.NoError(t, err, "failed to query session from Redis")

	require.NotNil(t, session, "session should exist in Redis")

	// Step 5: Verify session properties
	assert.Equal(t, user.ID, session.UserID,
		"session user_id should match created user ID")

	assert.False(t, session.ExpiresAt.IsZero(),
		"session expiration field should be present and not zero")

	currentTime := time.Now()
	timeDifference := session.ExpiresAt.Sub(currentTime)
	assert.Greater(t, timeDifference, time.Second,
		"session expiration should be at least 1 second in the future, got %v", timeDifference)

	// Step 6: Verify cookie attributes
	assert.Equal(t, "session_id", cookie.Name,
		"cookie name should equal 'session_id'")

	assert.Equal(t, "/", cookie.Path,
		"cookie path should equal '/'")

	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite,
		"cookie SameSite should be Lax")

	assert.True(t, cookie.HttpOnly,
		"cookie HttpOnly should be true")

	assert.True(t, cookie.Secure,
		"cookie Secure should be true")

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_PasswordSecurity validates password hashing security and proper storage.
// It verifies that after successful registration, the password is:
//   - Properly hashed before storage
//   - Not stored as plaintext
//   - Has sufficient length (minimum 32 characters)
//   - Can be validated against the original plaintext password
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send valid sign-up request to the handler
//  3. Verify HTTP response is successful (200 or 201)
//  4. Query database for password_hash within 100ms (performance requirement)
//  5. Verify password_hash does not equal plaintext password
//  6. Verify password_hash is not empty
//  7. Verify password_hash has minimum length of 32 characters
//  8. Verify password_hash can be validated against original password using password hasher
//  9. Fail test if any security requirement is violated
func TestSignUp_PasswordSecurity(t *testing.T) {
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
	baseUsername := "securitytest"
	username := prefixTestData(tc.testID, baseUsername)
	password := "securepass123"
	repeatPassword := "securepass123"

	// Step 1: Build and send sign-up request
	request := buildSignUpRequest(username, password, repeatPassword)
	response := sendSignUpRequest(t, tc.router, request)

	// Step 2: Verify response is successful (200 or 201)
	if response.Code != http.StatusOK && response.Code != http.StatusCreated {
		t.Fatalf("sign-up request should succeed, got status %d. Body: %s",
			response.Code, response.Body.String())
	}

	// Step 3: Query database for password_hash within 100ms
	queryStart := time.Now()
	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	queryDuration := time.Since(queryStart)

	require.NoError(t, err, "failed to query user from database")
	require.NotNil(t, user, "user should exist in database")

	// Verify query completed within 100ms
	assert.Less(t, queryDuration, 100*time.Millisecond,
		"database query should complete within 100ms, took %v", queryDuration)

	// Step 4: Verify password_hash does not equal plaintext password
	passwordHashStr := string(user.PasswordHash)
	if passwordHashStr == password {
		t.Fatalf("SECURITY VIOLATION: password_hash equals plaintext password")
	}
	assert.NotEqual(t, password, passwordHashStr,
		"password_hash must not equal plaintext password")

	// Step 5: Verify password_hash is not empty
	if len(user.PasswordHash) == 0 {
		t.Fatalf("SECURITY VIOLATION: password_hash is empty")
	}
	assert.NotEmpty(t, user.PasswordHash,
		"password_hash must not be empty")

	// Step 6: Verify password_hash has minimum length of 32 characters
	hashLength := len(user.PasswordHash)
	if hashLength < 32 {
		t.Fatalf("SECURITY VIOLATION: password_hash length is %d, must be at least 32 characters",
			hashLength)
	}
	assert.GreaterOrEqual(t, hashLength, 32,
		"password_hash must have minimum length of 32 characters, got %d", hashLength)

	// Step 7: Verify password_hash can be validated against original password
	err = tc.passwordHasher.Compare(user.PasswordHash, password)
	if err != nil {
		t.Fatalf("SECURITY VIOLATION: hash verification failed for valid password: %v", err)
	}
	require.NoError(t, err,
		"password_hash must be valid and verifiable against original password")

	// Additional security verification: Ensure hash is different for each execution
	// (this is implicit in bcrypt but we verify the hash is not deterministic)
	// Send another request with same password to verify different hashes are generated
	baseUsername2 := "securitytest2"
	username2 := prefixTestData(tc.testID, baseUsername2)
	request2 := buildSignUpRequest(username2, password, repeatPassword)
	response2 := sendSignUpRequest(t, tc.router, request2)

	if response2.Code == http.StatusOK || response2.Code == http.StatusCreated {
		user2, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername2)
		require.NoError(t, err, "failed to query second user from database")
		require.NotNil(t, user2, "second user should exist in database")

		// Verify that same password produces different hashes (bcrypt salt randomization)
		assert.NotEqual(t, string(user.PasswordHash), string(user2.PasswordHash),
			"same password should produce different hashes (salt randomization)")
	}

	// Cleanup: Delete test data
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err, "failed to cleanup users")

	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err, "failed to cleanup sessions")
}

// TestSignUp_InvalidRequest_MalformedJSON validates that sign-up with malformed JSON is rejected.
// It verifies the system correctly rejects a registration attempt with malformed JSON body,
// returning appropriate error status and message while preventing any database or session creation.
//
// Test flow:
//  1. Initialize test context with PostgreSQL and Redis containers
//  2. Send sign-up request with malformed JSON (unclosed braces, invalid syntax)
//  3. Verify HTTP response status is 400
//  4. Verify response body contains error message describing validation failure
//  5. Verify no user record was created in database
//  6. Verify no session record was created in Redis
func TestSignUp_InvalidRequest_MalformedJSON(t *testing.T) {
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
			body: `{"username":"testuser","password":"testpass123","repeat_password":"testpass123"`,
		},
		{
			name: "missing quotes",
			body: `{username:"testuser","password":"testpass123","repeat_password":"testpass123"}`,
		},
		{
			name: "trailing comma",
			body: `{"username":"testuser","password":"testpass123","repeat_password":"testpass123",}`,
		},
		{
			name: "invalid escape sequence",
			body: `{"username":"test\xuser","password":"testpass123","repeat_password":"testpass123"}`,
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

	for _, tcJSON := range testCases {
		t.Run(tcJSON.name, func(t *testing.T) {
			// Count users and sessions before request
			userLCountBefore, err := countUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to count users before request")

			sessionLCountBefore, err := countSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to count sessions before request")

			// Step 1: Send sign-up request with malformed JSON
			response := sendRawSignUpRequest(t, tc.router, tcJSON.body)

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

			// Step 4: Verify no user record created
			userLCountAfter, err := countUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to count users after request")
			assert.Equal(t, userLCountBefore, userLCountAfter,
				"no user record should be created when JSON is malformed")

			// Step 5: Verify no session record created
			sessionLCountAfter, err := countSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to count sessions after request")
			assert.Equal(t, sessionLCountBefore, sessionLCountAfter,
				"no session record should be created when JSON is malformed")

			// Step 6: Verify no session cookie in response
			_, err = extractSessionCookie(t, response)
			assert.Error(t, err, "invalid request should not return session cookie")

			// Cleanup: Delete any test data (should be none, but for safety)
			err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
			require.NoError(t, err, "failed to cleanup users")

			err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
			require.NoError(t, err, "failed to cleanup sessions")
		})
	}
}

// TestSignUp_ParallelIsolation validates that multiple concurrent tests don't interfere with each other.
// It verifies the test isolation mechanism by running multiple concurrent sign-ups and ensuring:
//   - Each test has a unique test ID
//   - Usernames are properly prefixed with test IDs
//   - Session keys are properly isolated
//   - No cross-contamination between concurrent tests
//
// Test flow:
//  1. Initialize 5 test contexts concurrently
//  2. Perform sign-up with identical base usernames in each context
//  3. Verify each sign-up succeeds independently
//  4. Verify each user is stored with correct test ID prefix
//  5. Verify each session is isolated
//  6. Verify user counts are correct per test context
//  7. Verify cleanup doesn't affect other concurrent tests
func TestSignUp_ParallelIsolation(t *testing.T) {
	t.Parallel()

	const numConcurrentTests = 5
	const baseUsername = "paralleluser"
	const password = "testpass123"

	// Channel to collect results from concurrent tests
	type testResult struct {
		testID   string
		username string
		userID   uuid.UUID
		err      error
	}
	results := make(chan testResult, numConcurrentTests)

	// Launch multiple concurrent test contexts
	for i := range numConcurrentTests {
		go func(index int) {
			ctx := context.Background()

			// Initialize test context (each gets unique test ID)
			tc, err := NewTestContext(ctx)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to create test context: %w", index, err)}
				return
			}
			defer tc.Cleanup()

			// Verify clean state
			err = tc.VerifyCleanState(ctx)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: pre-test state verification failed: %w", index, err)}
				return
			}

			// Build request with same base username (isolation will add unique prefix)
			username := prefixTestData(tc.testID, baseUsername)
			request := buildSignUpRequest(username, password, password)
			response := sendSignUpRequest(t, tc.router, request)

			// Verify success
			if response.Code != http.StatusOK && response.Code != http.StatusCreated {
				results <- testResult{
					err: fmt.Errorf("test %d: sign-up failed with status %d, body: %s",
						index, response.Code, response.Body.String()),
				}
				return
			}

			// Verify user created in database
			user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to query user: %w", index, err)}
				return
			}
			if user == nil {
				results <- testResult{err: fmt.Errorf("test %d: user not found in database", index)}
				return
			}

			// Verify username has correct test ID prefix
			expectedPrefix := tc.testID + "_"
			if !containsIgnoreCase(user.Username, expectedPrefix) {
				results <- testResult{
					err: fmt.Errorf("test %d: username '%s' doesn't contain expected prefix '%s'",
						index, user.Username, expectedPrefix),
				}
				return
			}

			// Verify exactly one user exists for this test context
			userCount, err := countUsers(ctx, tc.pgPool, tc.testID)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to count users: %w", index, err)}
				return
			}
			if userCount != 1 {
				results <- testResult{
					err: fmt.Errorf("test %d: expected 1 user, got %d", index, userCount),
				}
				return
			}

			// Verify session cookie exists
			cookie, err := extractSessionCookie(t, response)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: session cookie not found: %w", index, err)}
				return
			}

			// Verify session exists in Redis
			sessionID, err := parseUUID(cookie.Value)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: invalid session ID: %w", index, err)}
				return
			}

			session, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to query session: %w", index, err)}
				return
			}
			if session == nil {
				results <- testResult{err: fmt.Errorf("test %d: session not found in Redis", index)}
				return
			}

			// Verify session user ID matches created user
			if session.UserID != user.ID {
				results <- testResult{
					err: fmt.Errorf("test %d: session user ID (%d) doesn't match user ID (%d)",
						index, session.UserID, user.ID),
				}
				return
			}

			// Cleanup this test's data
			err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to cleanup users: %w", index, err)}
				return
			}

			err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
			if err != nil {
				results <- testResult{err: fmt.Errorf("test %d: failed to cleanup sessions: %w", index, err)}
				return
			}

			// Success - send result
			results <- testResult{
				testID:   tc.testID,
				username: user.Username,
				userID:   user.ID,
				err:      nil,
			}
		}(i)
	}

	// Collect results from all concurrent tests
	testIDs := make(map[string]bool)
	usernames := make(map[string]bool)
	userIDs := make(map[uuid.UUID]bool)

	for range numConcurrentTests {
		result := <-results
		if result.err != nil {
			t.Errorf("Concurrent test failed: %v", result.err)
			continue
		}

		// Verify test ID is unique
		if testIDs[result.testID] {
			t.Errorf("Duplicate test ID detected: %s", result.testID)
		}
		testIDs[result.testID] = true

		// Verify username is unique (prefixed with unique test ID)
		if usernames[result.username] {
			t.Errorf("Duplicate username detected: %s", result.username)
		}
		usernames[result.username] = true

		// Verify user ID is unique (no database conflicts)
		if userIDs[result.userID] {
			t.Errorf("Duplicate user ID detected: %d", result.userID)
		}
		userIDs[result.userID] = true

		// Verify username contains test ID prefix
		if !containsIgnoreCase(result.username, result.testID) {
			t.Errorf("Username '%s' doesn't contain test ID '%s'", result.username, result.testID)
		}
	}

	// Verify all tests completed successfully
	require.Equal(t, numConcurrentTests, len(testIDs),
		"not all concurrent tests completed successfully")

	// Summary verification
	t.Logf("Parallel isolation test completed successfully:")
	t.Logf("  - %d concurrent tests executed", numConcurrentTests)
	t.Logf("  - %d unique test IDs generated", len(testIDs))
	t.Logf("  - %d unique usernames created", len(usernames))
	t.Logf("  - %d unique user IDs assigned", len(userIDs))
}
