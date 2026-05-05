package request

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aibird/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: "error", Format: "text"})
}

func TestRequest_Fields(t *testing.T) {
	r := Request{
		Url:     "http://example.com",
		Method:  methodPost,
		Payload: map[string]string{"key": "value"},
		Headers: []Headers{
			{Key: "Content-Type", Value: "application/json"},
		},
	}

	assert.Equal(t, "http://example.com", r.Url)
	assert.Equal(t, methodPost, r.Method)
	assert.Len(t, r.Headers, 1)
	assert.Equal(t, "application/json", r.Headers[0].Value)

	payload, ok := r.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "value", payload["key"])
}

func TestRequest_AddHeader(t *testing.T) {
	r := Request{}
	r.AddHeader("X-Custom", "value")
	require.Len(t, r.Headers, 1)
	assert.Equal(t, "X-Custom", r.Headers[0].Key)
	assert.Equal(t, "value", r.Headers[0].Value)
}

func TestGetFileContentType_JPEG(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.jpg")
	// Write a minimal JPEG header
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", ct)
}

func TestGetFileContentType_PNG(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.png")
	// Write a minimal PNG signature
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "image/png", ct)
}

func TestGetFileContentType_TXT(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("hello world"), 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "text/plain", ct)
}

func TestGetFileContentType_WEBP(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.webp")
	// Write minimal data that won't be detected as a known type
	require.NoError(t, os.WriteFile(tmpFile, []byte("RIFF\x00\x00\x00\x00WEBP"), 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "image/webp", ct)
}

func TestGetFileContentType_WEBM(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.webm")
	require.NoError(t, os.WriteFile(tmpFile, []byte("some data"), 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "video/webm", ct)
}

func TestGetFileContentType_Unknown(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.bin")
	require.NoError(t, os.WriteFile(tmpFile, []byte{0x00, 0x01, 0x02, 0x03}, 0644))

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	ct, err := getFileContentType(f)
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", ct)
}

func TestCall_PostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, methodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]string
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)
		assert.Equal(t, "world", payload["hello"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result": "ok"}`))
	}))
	defer server.Close()

	req := Request{
		Url:    server.URL + "/test",
		Method: methodPost,
		Headers: []Headers{
			{Key: "Content-Type", Value: "application/json"},
		},
		Payload: map[string]string{"hello": "world"},
	}

	var response map[string]string
	err := req.Call(&response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response["result"])
}

func TestCall_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, methodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "active"}`))
	}))
	defer server.Close()

	req := Request{
		Url:    server.URL + "/status",
		Method: methodGet,
	}

	var response map[string]string
	err := req.Call(&response)
	require.NoError(t, err)
	assert.Equal(t, "active", response["status"])
}

func TestCall_StringResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("https://example.com/uploaded"))
	}))
	defer server.Close()

	req := Request{
		Url:    server.URL,
		Method: methodPost,
	}

	var response string
	err := req.Call(&response)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/uploaded", response)
}

func TestCall_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "boom"}`))
	}))
	defer server.Close()

	req := Request{
		Url:    server.URL,
		Method: methodGet,
	}

	var response map[string]string
	err := req.Call(&response)
	// Call does not check status code, it just decodes the body
	require.NoError(t, err)
	assert.Equal(t, "boom", response["error"])
}
