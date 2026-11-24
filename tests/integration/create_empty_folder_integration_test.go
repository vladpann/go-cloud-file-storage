package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type createEmptyFolderTestResponse struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size *int64 `json:"size,omitempty"`
	Type string `json:"type"`
}

func sendCreateEmptyFolderRequest(
	t *testing.T,
	router http.Handler,
	rawPath string,
	includePathParam bool,
	sessionCookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	target := "/api/v1/directory"
	if includePathParam {
		target += "?path=" + url.QueryEscape(rawPath)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, http.NoBody)
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func parseCreateEmptyFolderResponse(t *testing.T, resp *httptest.ResponseRecorder) createEmptyFolderTestResponse {
	t.Helper()

	var body createEmptyFolderTestResponse
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "failed to unmarshal create directory response: %s", resp.Body.String())

	return body
}

// TestCreateEmptyFolder_RootFolder validates creating a directory in the user's root folder.
func TestCreateEmptyFolder_RootFolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	signUpResp := sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password))
	assertHTTPStatus(t, signUpResp, http.StatusCreated)

	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err, "failed to extract session cookie")

	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err, "failed to query user from database")

	// Step 1: Create root folder
	resp := sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusCreated)

	// Step 2: Verify JSON response body (and ensure "size" is omitted)
	body := parseCreateEmptyFolderResponse(t, resp)
	assert.Equal(t, "", body.Path)
	assert.Equal(t, "folder1", body.Name)
	assert.Equal(t, "DIRECTORY", body.Type)
	assert.Nil(t, body.Size)

	var rawMap map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &rawMap))
	_, hasSize := rawMap["size"]
	assert.False(t, hasSize, "directory response must not include size field")

	// Step 3: Verify folder marker exists in MinIO
	expectedKey := fmt.Sprintf("user-%s-files/folder1/", user.ID.String())
	objInfo, err := tc.minioClient.StatObject(ctx, tc.minioClient.BucketName(), expectedKey, minio.StatObjectOptions{})
	require.NoError(t, err, "created folder should exist in MinIO")
	assert.Equal(t, int64(0), objInfo.Size)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestCreateEmptyFolder_NestedFolder validates creating nested folders and parsing parent path + name.
func TestCreateEmptyFolder_NestedFolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	signUpResp := sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password))
	assertHTTPStatus(t, signUpResp, http.StatusCreated)

	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err)

	// Create parent chain first
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/", true, sessionCookie), http.StatusCreated)

	// Create target nested folder
	resp := sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/folder3/", true, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusCreated)

	body := parseCreateEmptyFolderResponse(t, resp)
	assert.Equal(t, "folder1/folder2/", body.Path)
	assert.Equal(t, "folder3", body.Name)
	assert.Equal(t, "DIRECTORY", body.Type)
	assert.Nil(t, body.Size)

	expectedKey := fmt.Sprintf("user-%s-files/folder1/folder2/folder3/", user.ID.String())
	_, err = tc.minioClient.StatObject(ctx, tc.minioClient.BucketName(), expectedKey, minio.StatObjectOptions{})
	require.NoError(t, err, "nested folder should exist in MinIO")

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestCreateEmptyFolder_AlreadyExists validates that creating an existing folder returns 409 Conflict.
func TestCreateEmptyFolder_AlreadyExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)), http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	// First creation succeeds
	firstResp := sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie)
	assertHTTPStatus(t, firstResp, http.StatusCreated)

	// Duplicate creation returns 409 Conflict
	secondResp := sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie)
	assertHTTPStatus(t, secondResp, http.StatusConflict)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestCreateEmptyFolder_InvalidPath validates 400 Bad Request for invalid or missing path parameters.
func TestCreateEmptyFolder_InvalidPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)), http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	testCases := []struct {
		name             string
		rawPath          string
		includePathParam bool
	}{
		{name: "missing path query param", includePathParam: false},
		{name: "empty path", rawPath: "", includePathParam: true},
		{name: "only slash", rawPath: "/", includePathParam: true},
		{name: "missing trailing slash", rawPath: "folder1", includePathParam: true},
		{name: "file path instead of directory", rawPath: "folder1/file.txt", includePathParam: true},
		{name: "leading slash", rawPath: "/folder1/", includePathParam: true},
		{name: "double slash", rawPath: "folder1//folder2/", includePathParam: true},
		{name: "path traversal root", rawPath: "../secret/", includePathParam: true},
		{name: "path traversal nested", rawPath: "folder1/../folder2/", includePathParam: true},
		{name: "dot segment", rawPath: "folder1/./folder2/", includePathParam: true},
	}

	for _, tcCase := range testCases {
		t.Run(tcCase.name, func(t *testing.T) {
			resp := sendCreateEmptyFolderRequest(t, tc.router, tcCase.rawPath, tcCase.includePathParam, sessionCookie)
			assertHTTPStatus(t, resp, http.StatusBadRequest)
		})
	}

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestCreateEmptyFolder_Unauthorized validates that unauthenticated requests return 401 Unauthorized.
func TestCreateEmptyFolder_Unauthorized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	resp := sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, nil)
	assertHTTPStatus(t, resp, http.StatusUnauthorized)
}

// TestCreateEmptyFolder_ParentNotFound validates 404 Not Found when parent folder does not exist.
// (Если проверка существования родительской папки у вас ещё не реализована — удалите или включите после реализации.)
func TestCreateEmptyFolder_ParentNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	baseUsername := "testuser"
	username := prefixTestData(tc.testID, baseUsername)
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)), http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	resp := sendCreateEmptyFolderRequest(t, tc.router, "nonexistent/child/", true, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusNotFound)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestCreateEmptyFolder_UserIsolation validates that two users can create folders with the same name.
func TestCreateEmptyFolder_UserIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	password := "testpass123"

	// User 1
	user1 := prefixTestData(tc.testID, "user1")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user1, password, password)), http.StatusCreated)
	signIn1 := sendSignInRequest(t, tc.router, buildSignInRequest(user1, password))
	assertHTTPStatus(t, signIn1, http.StatusOK)
	cookie1, err := extractSessionCookie(t, signIn1)
	require.NoError(t, err)

	// User 2
	user2 := prefixTestData(tc.testID, "user2")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user2, password, password)), http.StatusCreated)
	signIn2 := sendSignInRequest(t, tc.router, buildSignInRequest(user2, password))
	assertHTTPStatus(t, signIn2, http.StatusOK)
	cookie2, err := extractSessionCookie(t, signIn2)
	require.NoError(t, err)

	// Both create "shared-name/" without conflict
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "shared-name/", true, cookie1), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "shared-name/", true, cookie2), http.StatusCreated)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}
