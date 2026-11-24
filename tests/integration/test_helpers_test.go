package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildSignUpRequest verifies buildSignUpRequest creates valid request payload
func TestBuildSignUpRequest(t *testing.T) {
	req := buildSignUpRequest("testuser", "password123", "password123")

	assert.Equal(t, "testuser", req.Username)
	assert.Equal(t, "password123", req.Password)
	assert.Equal(t, "password123", req.RepeatPassword)
}

// TestPrefixTestData verifies test data prefixing for isolation
func TestPrefixTestData(t *testing.T) {
	testID := "test_12345"
	username := "user1"

	prefixed := prefixTestData(testID, username)

	assert.Equal(t, "test_12345_user1", prefixed)
	assert.Contains(t, prefixed, testID)
	assert.Contains(t, prefixed, username)
}

// TestGetUserByUsername verifies database query helper works correctly
func TestGetUserByUsername(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Query for non-existent user - should return nil without error
	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, user, "non-existent user should return nil")
}

// TestGetSessionByID verifies Redis session query helper works correctly
func TestGetSessionByID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Query for non-existent session - should return nil without error
	sessionID := uuid.New()
	session, err := getSessionByID(ctx, tc.redisClient, tc.testID, sessionID)
	require.NoError(t, err)
	assert.Nil(t, session, "non-existent session should return nil")
}

// TestCountUsers verifies user counting helper works correctly
func TestCountUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Count users - should be 0 initially
	count, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "should have 0 users initially")
}

// TestCountSessions verifies session counting helper works correctly
func TestCountSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Count sessions - should be 0 initially
	count, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "should have 0 sessions initially")
}

// TestDeleteAllUsers verifies user deletion helper works correctly
func TestDeleteAllUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Delete all users - should succeed even with no users
	err = deleteAllUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err)

	// Verify count is still 0
	count, err := countUsers(ctx, tc.pgPool, tc.testID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestDeleteAllSessions verifies session deletion helper works correctly
func TestDeleteAllSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create test context
	tc, err := NewTestContext(ctx)
	require.NoError(t, err)
	defer tc.Cleanup()

	// Delete all sessions - should succeed even with no sessions
	err = deleteAllSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err)

	// Verify count is still 0
	count, err := countSessions(ctx, tc.redisClient, tc.testID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestAssertHTTPStatus verifies HTTP status assertion helper
func TestAssertHTTPStatus(t *testing.T) {
	response := httptest.NewRecorder()
	response.WriteHeader(http.StatusOK)

	// Should not fail for matching status
	assertHTTPStatus(t, response, http.StatusOK)
}

// TestExtractSessionCookie verifies session cookie extraction
func TestExtractSessionCookie(t *testing.T) {
	response := httptest.NewRecorder()

	// Add a session cookie
	cookie := &http.Cookie{
		Name:  sessionCookieName,
		Value: "test-session-id",
	}
	http.SetCookie(response, cookie)

	// Extract the cookie
	extractedCookie, err := extractSessionCookie(t, response)
	require.NoError(t, err)
	assert.Equal(t, sessionCookieName, extractedCookie.Name)
	assert.Equal(t, "test-session-id", extractedCookie.Value)
}

// TestExtractSessionCookie_NotFound verifies error when cookie is missing
func TestExtractSessionCookie_NotFound(t *testing.T) {
	response := httptest.NewRecorder()

	// Try to extract non-existent cookie
	_, err := extractSessionCookie(t, response)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestGenerateTestID verifies test ID generation
func TestGenerateTestID(t *testing.T) {
	id1 := generateTestID()
	id2 := generateTestID()

	// Each ID should be unique
	assert.NotEqual(t, id1, id2)

	// Both should start with "test_"
	assert.Contains(t, id1, "test_")
	assert.Contains(t, id2, "test_")

	// Both should contain UUID format (has dashes)
	assert.Contains(t, id1, "-")
	assert.Contains(t, id2, "-")
}
