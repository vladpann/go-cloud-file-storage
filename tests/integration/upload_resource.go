package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uploadTestFileInput struct {
	fieldName string
	filename  string
	content   string
}

type uploadedResourceTestResponse struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size *int64 `json:"size,omitempty"`
	Type string `json:"type"`
}

func sendUploadResourceRequest(
	t *testing.T,
	router http.Handler,
	rawPath string,
	includePathParam bool,
	files []uploadTestFileInput,
	sessionCookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for i := range files {
		f := &files[i]
		fieldName := f.fieldName
		if fieldName == "" {
			fieldName = "object"
		}

		part, err := writer.CreateFormFile(fieldName, f.filename)
		require.NoError(t, err, "failed to create form file %q", f.filename)

		_, err = io.WriteString(part, f.content)
		require.NoError(t, err, "failed to write content for %q", f.filename)
	}

	require.NoError(t, writer.Close(), "failed to close multipart writer")

	target := "/api/v1/resource"
	if includePathParam {
		target += "?path=" + url.QueryEscape(rawPath)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func sendRawUploadResourceRequest(
	t *testing.T,
	router http.Handler,
	rawPath string,
	contentType string,
	rawBody string,
	sessionCookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	target := "/api/v1/resource?path=" + url.QueryEscape(rawPath)

	var bodyReader io.Reader = http.NoBody
	if rawBody != "" {
		bodyReader = bytes.NewBufferString(rawBody)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, bodyReader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func parseUploadResourceResponse(t *testing.T, resp *httptest.ResponseRecorder) []uploadedResourceTestResponse {
	t.Helper()

	var items []uploadedResourceTestResponse
	err := json.Unmarshal(resp.Body.Bytes(), &items)
	require.NoError(t, err, "failed to unmarshal upload response: %s", resp.Body.String())

	return items
}

func readMinIOObjectContent(t *testing.T, ctx context.Context, tc *TestContext, key string) string {
	t.Helper()

	obj, err := tc.minioClient.GetObject(ctx, tc.minioClient.BucketName(), key, minio.GetObjectOptions{})
	require.NoError(t, err, "failed to get object %q from MinIO", key)
	defer func() {
		_ = obj.Close()
	}()

	data, err := io.ReadAll(obj)
	require.NoError(t, err, "failed to read object %q from MinIO", key)

	return string(data)
}

// TestUploadResource_RootFiles validates uploading regular and 0-byte files into the user's root folder.
func TestUploadResource_RootFiles(t *testing.T) {
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

	files := []uploadTestFileInput{
		{fieldName: "object", filename: "hello.txt", content: "hello world"},
		{fieldName: "object", filename: "empty.txt", content: ""},
	}

	resp := sendUploadResourceRequest(t, tc.router, "", true, files, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusCreated)

	items := parseUploadResourceResponse(t, resp)
	require.Len(t, items, 2)

	byName := make(map[string]uploadedResourceTestResponse, len(items))
	for _, item := range items {
		byName[item.Name] = item
	}

	helloItem, ok := byName["hello.txt"]
	require.True(t, ok)
	assert.Equal(t, "", helloItem.Path)
	assert.Equal(t, "FILE", helloItem.Type)
	require.NotNil(t, helloItem.Size)
	assert.Equal(t, int64(11), *helloItem.Size)

	emptyItem, ok := byName["empty.txt"]
	require.True(t, ok)
	assert.Equal(t, "", emptyItem.Path)
	assert.Equal(t, "FILE", emptyItem.Type)
	require.NotNil(t, emptyItem.Size)
	assert.Equal(t, int64(0), *emptyItem.Size)

	// Verify actual file content stored in MinIO
	helloKey := fmt.Sprintf("user-%s-files/hello.txt", user.ID.String())
	assert.Equal(t, "hello world", readMinIOObjectContent(t, ctx, tc, helloKey))

	emptyKey := fmt.Sprintf("user-%s-files/empty.txt", user.ID.String())
	assert.Equal(t, "", readMinIOObjectContent(t, ctx, tc, emptyKey))

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestUploadResource_NestedFolders validates uploading files with relative subdirectories in filenames.
func TestUploadResource_NestedFolders(t *testing.T) {
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

	// Create target storage folder
	assertHTTPStatus(t, sendCreateEmptyFolderRequest(t, tc.router, "storage_folder/", true, sessionCookie), http.StatusCreated)

	files := []uploadTestFileInput{
		{fieldName: "object", filename: "upload_folder/test.txt", content: "first"},
		{fieldName: "object", filename: "upload_folder/deep/sub/file2.txt", content: "second file"},
	}

	resp := sendUploadResourceRequest(t, tc.router, "storage_folder/", true, files, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusCreated)

	items := parseUploadResourceResponse(t, resp)
	require.Len(t, items, 2)

	byName := make(map[string]uploadedResourceTestResponse, len(items))
	for _, item := range items {
		byName[item.Name] = item
	}

	assert.Equal(t, "storage_folder/upload_folder/", byName["test.txt"].Path)
	assert.Equal(t, "FILE", byName["test.txt"].Type)
	require.NotNil(t, byName["test.txt"].Size)
	assert.Equal(t, int64(5), *byName["test.txt"].Size)

	assert.Equal(t, "storage_folder/upload_folder/deep/sub/", byName["file2.txt"].Path)
	assert.Equal(t, "FILE", byName["file2.txt"].Type)
	require.NotNil(t, byName["file2.txt"].Size)
	assert.Equal(t, int64(11), *byName["file2.txt"].Size)

	// Verify intermediate directory markers and file contents in MinIO
	rootPrefix := fmt.Sprintf("user-%s-files/", user.ID.String())
	for _, dirKey := range []string{
		rootPrefix + "storage_folder/upload_folder/",
		rootPrefix + "storage_folder/upload_folder/deep/",
		rootPrefix + "storage_folder/upload_folder/deep/sub/",
	} {
		_, statErr := tc.minioClient.StatObject(ctx, tc.minioClient.BucketName(), dirKey, minio.StatObjectOptions{})
		require.NoError(t, statErr, "intermediate folder marker %q should exist in MinIO", dirKey)
	}

	assert.Equal(t, "first", readMinIOObjectContent(t, ctx, tc, rootPrefix+"storage_folder/upload_folder/test.txt"))
	assert.Equal(t, "second file", readMinIOObjectContent(t, ctx, tc, rootPrefix+"storage_folder/upload_folder/deep/sub/file2.txt"))

	// Verify GET /directory lists the auto-created upload_folder
	listResp := sendGetFolderContentRequest(t, tc.router, "storage_folder/", true, sessionCookie)
	assertHTTPStatus(t, listResp, http.StatusOK)
	listed := parseFolderContentResponse(t, listResp)
	require.Len(t, listed, 1)
	assert.Equal(t, "upload_folder", listed[0].Name)
	assert.Equal(t, "DIRECTORY", listed[0].Type)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestUploadResource_AlreadyExists validates 409 Conflict and ensures partial uploads do not occur.
func TestUploadResource_AlreadyExists(t *testing.T) {
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

	// Upload initial file
	firstResp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "dup.txt", content: "v1"},
	}, sessionCookie)
	assertHTTPStatus(t, firstResp, http.StatusCreated)

	// Duplicate single file -> 409 Conflict
	secondResp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "dup.txt", content: "v2"},
	}, sessionCookie)
	assertHTTPStatus(t, secondResp, http.StatusConflict)

	// Batch containing a new file + existing file -> 409 Conflict and "new.txt" must NOT be uploaded
	batchResp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "new.txt", content: "should not persist"},
		{fieldName: "object", filename: "dup.txt", content: "v2"},
	}, sessionCookie)
	assertHTTPStatus(t, batchResp, http.StatusConflict)

	newFileKey := fmt.Sprintf("user-%s-files/new.txt", user.ID.String())
	exists, err := tc.minioClient.ObjectExists(ctx, newFileKey)
	require.NoError(t, err)
	assert.False(t, exists, "new.txt should not be uploaded when batch fails with 409")

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestUploadResource_InvalidRequest validates 400 Bad Request for malformed multipart bodies and invalid paths.
func TestUploadResource_InvalidRequest(t *testing.T) {
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

	t.Run("non-multipart body", func(t *testing.T) {
		resp := sendRawUploadResourceRequest(t, tc.router, "", "application/json", `{"file":"x"}`, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("empty multipart without files", func(t *testing.T) {
		resp := sendUploadResourceRequest(t, tc.router, "", true, nil, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid target path without trailing slash", func(t *testing.T) {
		resp := sendUploadResourceRequest(t, tc.router, "folder1", true, []uploadTestFileInput{
			{fieldName: "object", filename: "a.txt", content: "hi"},
		}, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid target path traversal", func(t *testing.T) {
		resp := sendUploadResourceRequest(t, tc.router, "../evil/", true, []uploadTestFileInput{
			{fieldName: "object", filename: "a.txt", content: "hi"},
		}, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid filename path traversal", func(t *testing.T) {
		resp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
			{fieldName: "object", filename: "../evil.txt", content: "hi"},
		}, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("duplicate filename in same request", func(t *testing.T) {
		resp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
			{fieldName: "object", filename: "same.txt", content: "1"},
			{fieldName: "object", filename: "same.txt", content: "2"},
		}, sessionCookie)
		assertHTTPStatus(t, resp, http.StatusBadRequest)
	})

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestUploadResource_TargetFolderNotFound validates 404 Not Found when target directory does not exist.
func TestUploadResource_TargetFolderNotFound(t *testing.T) {
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

	resp := sendUploadResourceRequest(t, tc.router, "missing_folder/", true, []uploadTestFileInput{
		{fieldName: "object", filename: "a.txt", content: "hi"},
	}, sessionCookie)
	assertHTTPStatus(t, resp, http.StatusNotFound)

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}

// TestUploadResource_Unauthorized validates that unauthenticated upload requests return 401 Unauthorized.
func TestUploadResource_Unauthorized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	resp := sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "a.txt", content: "hi"},
	}, nil)
	assertHTTPStatus(t, resp, http.StatusUnauthorized)
}

// TestUploadResource_UserIsolation validates that two users can upload files with the same name independently.
func TestUploadResource_UserIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tc, err := NewTestContext(ctx)
	require.NoError(t, err, "failed to create test context")
	defer tc.Cleanup()

	err = tc.VerifyCleanState(ctx)
	require.NoError(t, err, "pre-test state verification failed")

	password := "testpass123"

	user1Name := prefixTestData(tc.testID, "user1")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user1Name, password, password)), http.StatusCreated)
	signIn1 := sendSignInRequest(t, tc.router, buildSignInRequest(user1Name, password))
	assertHTTPStatus(t, signIn1, http.StatusOK)
	cookie1, err := extractSessionCookie(t, signIn1)
	require.NoError(t, err)

	user2Name := prefixTestData(tc.testID, "user2")
	assertHTTPStatus(t, sendSignUpRequest(t, tc.router, buildSignUpRequest(user2Name, password, password)), http.StatusCreated)
	signIn2 := sendSignInRequest(t, tc.router, buildSignInRequest(user2Name, password))
	assertHTTPStatus(t, signIn2, http.StatusOK)
	cookie2, err := extractSessionCookie(t, signIn2)
	require.NoError(t, err)

	assertHTTPStatus(t, sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "shared.txt", content: "user1-data"},
	}, cookie1), http.StatusCreated)

	assertHTTPStatus(t, sendUploadResourceRequest(t, tc.router, "", true, []uploadTestFileInput{
		{fieldName: "object", filename: "shared.txt", content: "user2-data"},
	}, cookie2), http.StatusCreated)

	user1, err := getUserByUsername(ctx, tc.pgPool, tc.testID, "user1")
	require.NoError(t, err)
	user2, err := getUserByUsername(ctx, tc.pgPool, tc.testID, "user2")
	require.NoError(t, err)

	assert.Equal(t, "user1-data", readMinIOObjectContent(t, ctx, tc, fmt.Sprintf("user-%s-files/shared.txt", user1.ID.String())))
	assert.Equal(t, "user2-data", readMinIOObjectContent(t, ctx, tc, fmt.Sprintf("user-%s-files/shared.txt", user2.ID.String())))

	require.NoError(t, deleteAllUsers(ctx, tc.pgPool, tc.testID))
	require.NoError(t, deleteAllSessions(ctx, tc.redisClient, tc.testID))
}
