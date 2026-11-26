package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type folderContentItemResponse struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size *int64 `json:"size,omitempty"`
	Type string `json:"type"`
}

func sendGetFolderContentRequest(
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func parseFolderContentResponse(t *testing.T, resp *httptest.ResponseRecorder) []folderContentItemResponse {
	t.Helper()

	var items []folderContentItemResponse
	err := json.Unmarshal(resp.Body.Bytes(), &items)
	require.NoError(t, err, "failed to unmarshal folder content response: %s", resp.Body.String())

	return items
}

func putTestFileInMinIO(
	t *testing.T,
	ctx context.Context,
	tc *TestContext,
	userID string,
	relPath string,
	content string,
) {
	t.Helper()

	key := fmt.Sprintf("user-%s-files/%s", userID, relPath)
	_, err := tc.minioClient.Client.PutObject(
		ctx,
		tc.minioClient.BucketName(),
		key,
		strings.NewReader(content),
		int64(len(content)),
		minio.PutObjectOptions{ContentType: "text/plain"},
	)
	require.NoError(t, err, "failed to upload test file %q to MinIO", key)
}

// TestGetFolderContent_RootEmpty validates that an empty root folder returns 200 OK and [].
func TestGetFolderContent_RootEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	username := prefixTestData(tc.testID, "testuser")
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)),
		http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	t.Run("empty path param", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "", true, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusOK)
		assert.JSONEq(t, "[]", resp.Body.String())
	})

	t.Run("without path param", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "", false, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusOK)
		assert.JSONEq(t, "[]", resp.Body.String())
	})

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestGetFolderContent_RootNonRecursive validates non-recursive listing of files (including 0-byte) and folders.
func TestGetFolderContent_RootNonRecursive(t *testing.T) {
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

	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err)

	// Create root folders + nested folder + files (both root and nested)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder2/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/sub/", true, sessionCookie), http.StatusCreated)

	putTestFileInMinIO(t, ctx, tc, user.ID.String(), "hello.txt", "hello world")
	putTestFileInMinIO(t, ctx, tc, user.ID.String(), "empty.txt", "")
	putTestFileInMinIO(t, ctx, tc, user.ID.String(), "folder1/nested.txt", "nested")

	resp := sendGetFolderContentRequest(t, tc.router, "", true, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusOK)

	items := parseFolderContentResponse(t, resp)
	require.Len(t, items, 4, "should return only direct children of root")

	byName := make(map[string]folderContentItemResponse, len(items))
	for _, item := range items {
		byName[item.Name] = item
	}

	assert.Equal(t, folderContentItemResponse{Path: "", Name: "folder1", Type: "DIRECTORY"}, byName["folder1"])
	assert.Equal(t, folderContentItemResponse{Path: "", Name: "folder2", Type: "DIRECTORY"}, byName["folder2"])

	helloFile, ok := byName["hello.txt"]
	require.True(t, ok)
	assert.Equal(t, "", helloFile.Path)
	assert.Equal(t, "FILE", helloFile.Type)
	require.NotNil(t, helloFile.Size)
	assert.Equal(t, int64(11), *helloFile.Size)

	emptyFile, ok := byName["empty.txt"]
	require.True(t, ok)
	assert.Equal(t, "FILE", emptyFile.Type)
	require.NotNil(t, emptyFile.Size, "0-byte file must include size field")
	assert.Equal(t, int64(0), *emptyFile.Size)

	// Verify raw JSON omits "size" on DIRECTORY and includes "size": 0 on empty FILE
	var rawItems []map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &rawItems))
	for _, raw := range rawItems {
		if raw["type"] == "DIRECTORY" {
			_, hasSize := raw["size"]
			assert.False(t, hasSize, "directory must not contain size field")
		}
		if raw["name"] == "empty.txt" {
			sizeVal, hasSize := raw["size"]
			assert.True(t, hasSize, "0-byte file must contain size field")
			assert.Equal(t, float64(0), sizeVal)
		}
	}

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestGetFolderContent_NestedFolder validates listing a nested directory and an empty subdirectory.
func TestGetFolderContent_NestedFolder(t *testing.T) {
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

	user, err := getUserByUsername(ctx, tc.pgPool, tc.testID, baseUsername)
	require.NoError(t, err)

	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/sub/", true, sessionCookie), http.StatusCreated)
	putTestFileInMinIO(t, ctx, tc, user.ID.String(), "folder1/folder2/file.txt", "12345")

	t.Run("non-empty nested folder", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "folder1/folder2/", true, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusOK)

		items := parseFolderContentResponse(t, resp)
		require.Len(t, items, 2)

		byName := make(map[string]folderContentItemResponse, len(items))
		for _, item := range items {
			byName[item.Name] = item
		}

		assert.Equal(t, folderContentItemResponse{
			Path: "folder1/folder2/",
			Name: "sub",
			Type: "DIRECTORY",
		}, byName["sub"])

		fileItem := byName["file.txt"]
		assert.Equal(t, "folder1/folder2/", fileItem.Path)
		assert.Equal(t, "FILE", fileItem.Type)
		require.NotNil(t, fileItem.Size)
		assert.Equal(t, int64(5), *fileItem.Size)
	})

	t.Run("empty nested folder", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "folder1/folder2/sub/", true, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusOK)
		assert.JSONEq(t, "[]", resp.Body.String())
	})

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestGetFolderContent_NotFound validates 404 Not Found when the requested directory does not exist.
func TestGetFolderContent_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	username := prefixTestData(tc.testID, "testuser")
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)), http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/", true, sessionCookie), http.StatusCreated)
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "folder1/folder2/folder3/", true, sessionCookie), http.StatusCreated)

	t.Run("missing child inside existing folder", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "folder1/folder2/folder3/folder4/", true, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusNotFound)
	})

	t.Run("missing root folder", func(t *testing.T) {
		resp := sendGetFolderContentRequest(t, tc.router, "nonexistent/", true, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusNotFound)
	})

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestGetFolderContent_InvalidPath validates 400 Bad Request for invalid directory paths.
func TestGetFolderContent_InvalidPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	username := prefixTestData(tc.testID, "testuser")
	password := "testpass123"

	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(username, password, password)), http.StatusCreated)
	signInResp := sendSignInRequest(t, tc.router, buildSignInRequest(username, password))
	assertHTTPStatus(t, signInResp, http.StatusOK)

	sessionCookie, err := extractSessionCookie(t, signInResp)
	require.NoError(t, err)

	testCases := []struct {
		name    string
		rawPath string
	}{
		{name: "missing trailing slash", rawPath: "folder1"},
		{name: "file path instead of folder", rawPath: "folder1/file.txt"},
		{name: "leading slash", rawPath: "/folder1/"},
		{name: "double slash", rawPath: "folder1//folder2/"},
		{name: "path traversal root", rawPath: "../secret/"},
		{name: "path traversal nested", rawPath: "folder1/../folder2/"},
		{name: "dot segment", rawPath: "folder1/./folder2/"},
	}

	for _, tcCase := range testCases {
		t.Run(tcCase.name, func(t *testing.T) {
			resp := sendGetFolderContentRequest(t, tc.router, tcCase.rawPath, true, sessionCookie)
			assertHTTPStatus(t, resp, http.StatusBadRequest)
		})
	}

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestGetFolderContent_Unauthorized validates that unauthenticated requests return 401 Unauthorized.
func TestGetFolderContent_Unauthorized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	resp := sendGetFolderContentRequest(t, tc.router, "", true, nil)
	assertHTTPStatus(t, resp, http.StatusUnauthorized)
}

// TestGetFolderContent_UserIsolation validates that a user cannot list another user's directories.
func TestGetFolderContent_UserIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	password := "testpass123"

	user1 := prefixTestData(tc.testID, "user1")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user1, password, password)), http.StatusCreated)
	signIn1 := sendSignInRequest(t, tc.router, buildSignInRequest(user1, password))
	assertHTTPStatus(t, signIn1, http.StatusOK)
	cookie1, err := extractSessionCookie(t, signIn1)
	require.NoError(t, err)

	user2 := prefixTestData(tc.testID, "user2")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user2, password, password)), http.StatusCreated)
	signIn2 := sendSignInRequest(t, tc.router, buildSignInRequest(user2, password))
	assertHTTPStatus(t, signIn2, http.StatusOK)
	cookie2, err := extractSessionCookie(t, signIn2)
	require.NoError(t, err)

	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "private/", true, cookie1), http.StatusCreated)

	// User 2 sees empty root and gets 404 on User 1's folder
	rootResp := sendGetFolderContentRequest(t, tc.router, "", true, cookie2)
	assertHTTPStatus(t, rootResp, http.StatusOK)
	assert.JSONEq(t, "[]", rootResp.Body.String())

	privateResp := sendGetFolderContentRequest(t, tc.router, "private/", true, cookie2)
	assertHTTPStatus(t, privateResp, http.StatusNotFound)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}
