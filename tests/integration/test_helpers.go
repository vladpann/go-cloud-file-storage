package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	user_service "github.com/vladpann/go-cloud-file-storage/internal/features/users/service"
	users_transport_http "github.com/vladpann/go-cloud-file-storage/internal/features/users/transport/http"
)

var (
	sessionCookieName = "session_id"
)

// Request Building Helpers

// buildSignUpRequest creates a sign-up request payload with the given credentials.
func buildSignUpRequest(username, password, repeatPassword string) users_transport_http.SignUpRequest {
	return users_transport_http.SignUpRequest{
		Username:       username,
		Password:       password,
		RepeatPassword: repeatPassword,
	}
}

// sendSignUpRequest sends HTTP POST request to sign-up endpoint and returns response.
func sendSignUpRequest(t *testing.T, handler http.Handler, req users_transport_http.SignUpRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal sign-up request")

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-up", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// buildSignInRequest creates a sign-in request payload with the given credentials.
func buildSignInRequest(username, password string) users_transport_http.SignInRequest {
	return users_transport_http.SignInRequest{
		Username: username,
		Password: password,
	}
}

// sendSignInRequest sends HTTP POST request to sign-in endpoint and returns response.
func sendSignInRequest(t *testing.T, handler http.Handler, req users_transport_http.SignInRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal sign-up request")

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-in", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// sendMeRequest sends HTTP GET request to me endpoint and returns response.
func sendMeRequest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/user/me", nil)
	httpReq.Header.Set("Content-Type", "application/json")

	if sessionCookie != nil {
		httpReq.AddCookie(sessionCookie)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// sendRawSignUpRequest sends HTTP POST request to sign-up endpoint with raw body string.
// Used for testing malformed JSON and invalid request bodies.
func sendRawSignUpRequest(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-up", bytes.NewReader([]byte(body)))
	httpReq.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// sendRawSignInRequest sends HTTP POST request to sign-in endpoint with raw body string.
// Used for testing malformed JSON and invalid request bodies.
func sendRawSignInRequest(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-in", bytes.NewReader([]byte(body)))
	httpReq.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// Response Parsing Helpers

// parseSignUpResponse parses JSON response body into SignUpResponse struct.
func parseSignUpResponse(t *testing.T, response *httptest.ResponseRecorder) users_transport_http.SignUpResponse {
	t.Helper()

	var signUpResponse users_transport_http.SignUpResponse
	err := json.NewDecoder(response.Body).Decode(&signUpResponse)
	require.NoError(t, err, "failed to decode sign-up response")

	return signUpResponse
}

// parseSignInResponse parses JSON response body into SignInResponse struct.
func parseSignInResponse(t *testing.T, response *httptest.ResponseRecorder) users_transport_http.SignInResponse {
	t.Helper()

	var signInResponse users_transport_http.SignInResponse
	err := json.NewDecoder(response.Body).Decode(&signInResponse)
	require.NoError(t, err, "failed to decode sign-up response")

	return signInResponse
}

// extractSessionCookie extracts session cookie from HTTP response.
func extractSessionCookie(t *testing.T, response *httptest.ResponseRecorder) (*http.Cookie, error) {
	t.Helper()

	cookies := response.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			return cookie, nil
		}
	}

	return nil, fmt.Errorf("session cookie '%s' not found in response", sessionCookieName)
}

// Database Query Helpers

// getUserByUsername queries PostgreSQL for user by username.
// Returns nil user if not found (not an error).
func getUserByUsername(ctx context.Context, pool *pgxpool.Pool, testID, username string) (*domain.User, error) {
	prefixedUsername := prefixTestData(testID, username)

	query := `
		SELECT id, username, password_hash
		FROM cloud_file_storage.users
		WHERE username = $1
	`

	var user domain.User
	err := pool.QueryRow(ctx, query, prefixedUsername).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // Not found, not an error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query user by username: %w", err)
	}

	return &user, nil
}

// getSessionByID queries Redis for session by ID.
// Returns nil session if not found (not an error).
func getSessionByID(ctx context.Context, client *redis.Client, _ string, sessionID uuid.UUID) (*domain.Session, error) {
	// Use the same key format as the production code: "session:UUID"
	key := fmt.Sprintf("session:%s", sessionID.String())

	// Get session JSON from Redis
	data, err := client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // Not found, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session from Redis: %w", err)
	}

	// Parse session data
	var session domain.Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// countUsers returns count of users with test ID prefix in PostgreSQL.
func countUsers(ctx context.Context, pool *pgxpool.Pool, testID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM cloud_file_storage.users
		WHERE username LIKE $1
	`

	pattern := testID + "_%"
	var count int
	err := pool.QueryRow(ctx, query, pattern).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}

// countSessions returns count of sessions with test ID prefix in Redis.
func countSessions(ctx context.Context, client *redis.Client, _ string) (int, error) {
	// Sessions are stored with key "session:UUID", not prefixed with testID
	// For counting test sessions, we need to check all sessions
	// This is a simplification - in a real scenario with many tests,
	// we'd need a different isolation strategy
	pattern := "session:*"

	var count int
	var cursor uint64

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return 0, fmt.Errorf("failed to scan Redis keys: %w", err)
		}

		count += len(keys)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// Cleanup Helpers

// deleteAllUsers removes all user records with test ID prefix from PostgreSQL.
func deleteAllUsers(ctx context.Context, pool *pgxpool.Pool, testID string) error {
	query := `
		DELETE FROM cloud_file_storage.users
		WHERE username LIKE $1
	`

	pattern := testID + "_%"
	_, err := pool.Exec(ctx, query, pattern)
	if err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}

	return nil
}

// deleteAllSessions removes all session records with test ID prefix from Redis.
func deleteAllSessions(ctx context.Context, client *redis.Client, _ string) error {
	// Sessions are stored with key "session:UUID", not prefixed with testID
	pattern := "session:*"

	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan Redis keys: %w", err)
		}

		// Delete found keys
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete session keys: %w", err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}

// Validation Helpers

// assertHTTPStatus fails test if response status doesn't match expected.
func assertHTTPStatus(t *testing.T, response *httptest.ResponseRecorder, expectedStatus int) {
	t.Helper()
	if response.Code != expectedStatus {
		body := response.Body.String()
		t.Fatalf("expected status %d, got %d. Response body: %s",
			expectedStatus, response.Code, body)
	}
}

// verifyPasswordHash verifies that stored hash is valid for given plaintext password.
func verifyPasswordHash(t *testing.T, hasher user_service.PasswordHasher, hash []byte, plaintext string) {
	t.Helper()

	err := hasher.Compare(hash, plaintext)
	require.NoError(t, err, "password hash verification failed")
}

// Test Data Helpers

// prefixTestData prefixes data with test ID for isolation.
func prefixTestData(testID, data string) string {
	return fmt.Sprintf("%s_%s", testID, data)
}

// parseUUID parses a string into a UUID and returns an error if invalid.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// Sign-Out Request Helpers

// sendSignOutRequest sends HTTP POST request to sign-out endpoint with session cookie.
// It simulates the auth middleware by adding sessionID to the request context.
func sendSignOutRequest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-out", http.NoBody)
	httpReq.Header.Set("Content-Type", "application/json")

	if sessionCookie != nil {
		httpReq.AddCookie(sessionCookie)

		// Parse session ID from cookie and add to context (simulating auth middleware)
		sessionID, err := uuid.Parse(sessionCookie.Value)
		if err == nil {
			// Add sessionID to context using the same key as auth middleware
			ctx := context.WithValue(httpReq.Context(), contextKey("sessionID"), sessionID)
			httpReq = httpReq.WithContext(ctx)
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// sendSignOutRequestWithRawCookie sends HTTP POST request to sign-out endpoint with raw cookie value.
func sendSignOutRequestWithRawCookie(t *testing.T, handler http.Handler, cookieName, cookieValue string) *httptest.ResponseRecorder {
	t.Helper()

	httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/sign-out", http.NoBody)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.AddCookie(&http.Cookie{
		Name:  cookieName,
		Value: cookieValue,
	})

	// Try to parse and add sessionID to context
	sessionID, err := uuid.Parse(cookieValue)
	if err == nil {
		ctx := context.WithValue(httpReq.Context(), contextKey("sessionID"), sessionID)
		httpReq = httpReq.WithContext(ctx)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httpReq)

	return recorder
}

// contextKey is the type for context keys (matches the auth middleware)
type contextKey string
