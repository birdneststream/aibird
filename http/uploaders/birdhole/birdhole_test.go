package birdhole

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aibird/http/request"
	"aibird/logger"
	"aibird/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
	m.Run()
}

func TestUpload_Success(t *testing.T) {
	expectedURL := "https://hole.birdnest.live/derived/abc123.png"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		// Verify form fields
		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)

		assert.Equal(t, "10", r.FormValue("urllen"))
		assert.Equal(t, "3600", r.FormValue("expiry"))
		assert.Equal(t, "test description", r.FormValue("description"))
		assert.Equal(t, "extra", r.FormValue("extra_field"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"url":"%s"}`, expectedURL)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte("test content"), 0600)
	require.NoError(t, err)

	config := &settings.BirdholeConfig{
		Host:     ts.URL,
		Port:     "",
		EndPoint: "/upload",
		UrlLen:   10,
		Expiry:   3600,
		Key:      "test-key",
	}

	fields := []request.Fields{{Key: "extra_field", Value: "extra"}}
	result, err := upload(filePath, "test description", fields, config, false)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, result)

	// upload() calls os.Remove(fileName), so the file should be deleted
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestUpload_JSONParseError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte("test content"), 0600)
	require.NoError(t, err)

	config := &settings.BirdholeConfig{
		Host:     ts.URL,
		Port:     "",
		EndPoint: "/upload",
		UrlLen:   10,
		Expiry:   3600,
		Key:      "test-key",
	}

	_, err = upload(filePath, "test description", nil, config, false)
	require.Error(t, err)
}

func TestUpload_MissingURLKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte("test content"), 0600)
	require.NoError(t, err)

	config := &settings.BirdholeConfig{
		Host:     ts.URL,
		Port:     "",
		EndPoint: "/upload",
		UrlLen:   10,
		Expiry:   3600,
		Key:      "test-key",
	}

	result, err := upload(filePath, "test description", nil, config, false)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBirdHolePNG_Wrapper(t *testing.T) {
	expectedURL := "https://hole.birdnest.live/derived/xyz.png"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"url":"%s"}`, expectedURL)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.png")
	err := os.WriteFile(filePath, []byte("png content"), 0600)
	require.NoError(t, err)

	config := &settings.BirdholeConfig{
		Host:     ts.URL,
		Port:     "",
		EndPoint: "/upload",
		UrlLen:   10,
		Expiry:   3600,
		Key:      "test-key",
	}

	result, err := BirdHolePNG(filePath, "test message", nil, config)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, result)
}
