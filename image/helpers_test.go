package image

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aibird/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
	m.Run()
}

func createTestPNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, image.White)

	filePath := filepath.Join(dir, "test.png")
	f, err := os.Create(filePath)
	require.NoError(t, err)
	defer f.Close()

	err = png.Encode(f, img)
	require.NoError(t, err)
	return filePath
}

func TestToJpeg_ValidPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, image.White)
	img.Set(1, 1, image.Black)

	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	require.NoError(t, err)

	jpegBytes, err := ToJpeg(buf.Bytes())
	require.NoError(t, err)
	assert.Greater(t, len(jpegBytes), 0)

	// Verify it's valid JPEG
	decoded, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	require.NoError(t, err)
	assert.Equal(t, 2, decoded.Bounds().Dx())
	assert.Equal(t, 2, decoded.Bounds().Dy())
}

func TestToJpeg_NonPNG(t *testing.T) {
	// JPEG bytes should fail
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, nil)
	require.NoError(t, err)

	_, err = ToJpeg(buf.Bytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to convert")
}

func TestToJpeg_InvalidBytes(t *testing.T) {
	_, err := ToJpeg([]byte("not an image"))
	require.Error(t, err)
}

func TestConvertPngToJpg_Success(t *testing.T) {
	tmpDir := t.TempDir()
	pngPath := createTestPNG(t, tmpDir)

	result := ConvertPngToJpg(pngPath)
	assert.NotEqual(t, failedConvertPNGToJPG, result)
	assert.Contains(t, result, ".jpg")

	// Verify JPG exists
	_, err := os.Stat(result)
	require.NoError(t, err)

	// Verify original PNG is removed
	_, err = os.Stat(pngPath)
	assert.True(t, os.IsNotExist(err))
}

func TestConvertPngToJpg_NonPngFile(t *testing.T) {
	// Passing a non-png file should return it unchanged
	result := ConvertPngToJpg("/some/path/image.jpg")
	assert.Equal(t, "/some/path/image.jpg", result)
}

func TestConvertPngToJpg_PathTraversal(t *testing.T) {
	result := ConvertPngToJpg("../../../etc/passwd.png")
	assert.Equal(t, failedConvertPNGToJPG, result)
}

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single URL",
			input:    "Check out https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "multiple URLs",
			input:    "Visit https://foo.com and http://bar.org for more",
			expected: []string{"https://foo.com", "http://bar.org"},
		},
		{
			name:     "no URLs",
			input:    "Just plain text without links",
			expected: nil,
		},
		{
			name:     "URL in parentheses",
			input:    "See (https://example.com/path) for details",
			expected: []string{"https://example.com/path)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls, err := ExtractURLs(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, urls)
		})
	}
}

func TestIsImageURL_ValidImage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	assert.True(t, IsImageURL(ts.URL+"/image.png"))
}

func TestIsImageURL_NonImage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	assert.False(t, IsImageURL(ts.URL+"/page.html"))
}

func TestIsImageURL_InvalidScheme(t *testing.T) {
	assert.False(t, IsImageURL("ftp://example.com/image.png"))
	assert.False(t, IsImageURL("file:///local/image.png"))
}

func TestIsImageURL_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	assert.False(t, IsImageURL(ts.URL+"/error"))
}
