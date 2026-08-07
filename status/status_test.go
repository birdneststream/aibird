// Package status provides API client for the AI service status and model execution checks.
package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aibird/logger"
	"aibird/settings"
	"aibird/shared/meta"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
}

func TestParseTemperature(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"plain number", "45", 45, false},
		{"with C suffix", "45 C", 45, false},
		{"with °C suffix", "45°C", 45, false},
		{"with C no space", "45C", 45, false},
		{"with spaces", " 50 °C ", 50, false},
		{"zero", "0 C", 0, false},
		{"invalid text", "invalid", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTemperature(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestCheckGPUTemperatures(t *testing.T) {
	tests := []struct {
		name      string
		gpus      []GPUInfo
		threshold int
		wantName  string
		wantTemp  int
	}{
		{
			name:      "no gpus",
			gpus:      nil,
			threshold: 80,
			wantName:  "",
			wantTemp:  0,
		},
		{
			name: "all cool",
			gpus: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "45 C"},
			},
			threshold: 80,
			wantName:  "",
			wantTemp:  0,
		},
		{
			name: "hot gpu",
			gpus: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "85 C"},
				{Name: "NVIDIA GeForce RTX 2070", Temperature: "40 C"},
			},
			threshold: 80,
			wantName:  "RTX 4090",
			wantTemp:  85,
		},
		{
			name: "invalid temperature skipped",
			gpus: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "invalid"},
			},
			threshold: 80,
			wantName:  "",
			wantTemp:  0,
		},
		{
			name: "exactly at threshold",
			gpus: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "80 C"},
			},
			threshold: 80,
			wantName:  "",
			wantTemp:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, temp := checkGPUTemperatures(tt.gpus, tt.threshold)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantTemp, temp)
		})
	}
}

func TestFormatGPUInfo(t *testing.T) {
	gpu := GPUInfo{
		Name:           "NVIDIA GeForce RTX 4090",
		Temperature:    "45 C",
		MemoryUsed:     "8192 MiB",
		MemoryTotal:    "24576 MiB",
		UtilizationGPU: "50%",
	}

	result := formatGPUInfo(&gpu)

	assert.Contains(t, result, "RTX 4090")
	assert.Contains(t, result, "45 C")
	assert.Contains(t, result, "8192")
	assert.Contains(t, result, "24576")
	assert.Contains(t, result, "50%")
	assert.NotContains(t, result, "NVIDIA GeForce")
}

func TestFormatDockerStatus(t *testing.T) {
	tests := []struct {
		name     string
		docker   DockerStatus
		wantUp   int
		wantDown int
	}{
		{
			name:     "both running",
			docker:   DockerStatus{LLamacpp: true, ComfyUI: true},
			wantUp:   2,
			wantDown: 0,
		},
		{
			name:     "both stopped",
			docker:   DockerStatus{LLamacpp: false, ComfyUI: false},
			wantUp:   0,
			wantDown: 2,
		},
		{
			name:     "mixed",
			docker:   DockerStatus{LLamacpp: true, ComfyUI: false},
			wantUp:   1,
			wantDown: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDockerStatus(tt.docker)
			assert.Equal(t, tt.wantUp, strings.Count(result, "🟢"))
			assert.Equal(t, tt.wantDown, strings.Count(result, "⚫"))
		})
	}
}

func TestRemoveStringDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{"nil input", nil, []string{}},
		{"empty input", []string{}, []string{}},
		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"all duplicates", []string{"a", "a", "a"}, []string{"a"}},
		{"mixed duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"single element", []string{"a"}, []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeStringDuplicates(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNewClient verifies that NewClient initializes fields correctly.
func TestNewClient(t *testing.T) {
	config := &settings.AiBird{
		StatusUrl:    "http://example.com",
		StatusApiKey: "test-key",
	}
	c := NewClient(config)
	assert.Equal(t, "http://example.com", c.BaseURL)
	assert.Equal(t, "test-key", c.StatusApiKey)
	assert.NotNil(t, c.HTTPClient)
	assert.Equal(t, 45*time.Second, c.HTTPClient.Timeout)
}

// TestDoRequest_Success verifies a successful GET request with valid JSON response.
func TestDoRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("X-API-Key"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "test-key"})
	var result map[string]string
	err := c.doRequest("/api/status", &result)
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
}

// TestDoRequest_Non200 verifies error on non-200 status code.
func TestDoRequest_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	var result map[string]string
	err := c.doRequest("/api/status", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
}

// TestDoRequest_JSONDecodeError verifies error on invalid JSON response.
func TestDoRequest_JSONDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	var result map[string]string
	err := c.doRequest("/api/status", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

// TestGetStatus_CacheHit verifies that a second call within the cache TTL returns cached data without a second HTTP request.
func TestGetStatus_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{DockerStatus: DockerStatus{ComfyUI: true}})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})

	// First call should hit the server
	status1, err := c.GetStatus()
	require.NoError(t, err)
	assert.True(t, status1.DockerStatus.ComfyUI)
	assert.Equal(t, 1, callCount)

	// Second call should use cache (no additional HTTP request)
	status2, err := c.GetStatus()
	require.NoError(t, err)
	assert.True(t, status2.DockerStatus.ComfyUI)
	assert.Equal(t, 1, callCount)
}

// TestGetStatus_CacheMiss verifies that stale cache triggers a new request.
func TestGetStatus_CacheMiss(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{DockerStatus: DockerStatus{ComfyUI: callCount == 1}})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})

	status1, err := c.GetStatus()
	require.NoError(t, err)
	assert.True(t, status1.DockerStatus.ComfyUI)

	// Expire cache by setting cache time far in the past
	c.mu.Lock()
	c.statusCacheTime = time.Now().Add(-10 * time.Second)
	c.mu.Unlock()

	status2, err := c.GetStatus()
	require.NoError(t, err)
	assert.False(t, status2.DockerStatus.ComfyUI)
	assert.Equal(t, 2, callCount)
}

// TestGetStatus_ErrorPropagation verifies error from doRequest is propagated.
func TestGetStatus_ErrorPropagation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	status, err := c.GetStatus()
	require.Error(t, err)
	assert.Nil(t, status)
}

// TestGetWavs_CacheHit verifies cached WAV list is returned without a second HTTP request.
func TestGetWavs_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]string{"voice1", "voice2"})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})

	wavs1, err := c.GetWavs()
	require.NoError(t, err)
	assert.Equal(t, []string{"voice1", "voice2"}, wavs1)
	assert.Equal(t, 1, callCount)

	wavs2, err := c.GetWavs()
	require.NoError(t, err)
	assert.Equal(t, []string{"voice1", "voice2"}, wavs2)
	assert.Equal(t, 1, callCount)
}

// TestGetWavs_Deduplication verifies duplicate voice entries are deduplicated.
func TestGetWavs_Deduplication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]string{"voice1", "voice2", "voice1", "voice3", "voice2"})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	wavs, err := c.GetWavs()
	require.NoError(t, err)
	assert.Equal(t, []string{"voice1", "voice2", "voice3"}, wavs)
}

// TestAddVoice_Success verifies successful voice addition.
func TestAddVoice_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-key", r.Header.Get("X-API-Key"))
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "http://example.com/voice.wav", body["url"])
		assert.Equal(t, "myvoice", body["voice_name"])

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Voice added successfully"})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "test-key"})
	msg, err := c.AddVoice("http://example.com/voice.wav", "myvoice", "", "")
	require.NoError(t, err)
	assert.Equal(t, "Voice added successfully", msg)
}

// TestAddVoice_Non201 verifies error on unexpected status code.
func TestAddVoice_Non201(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	msg, err := c.AddVoice("http://example.com/voice.wav", "myvoice", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 400")
	assert.Empty(t, msg)
}

// TestGetGPUInfo_Success verifies GPU info extraction from status response.
func TestGetGPUInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			GPUs: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "45 C"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	gpus, err := c.GetGPUInfo()
	require.NoError(t, err)
	require.Len(t, gpus, 1)
	assert.Equal(t, "NVIDIA GeForce RTX 4090", gpus[0].Name)
}

// TestGetGPUInfo_ErrorPropagation verifies error propagation.
func TestGetGPUInfo_ErrorPropagation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	gpus, err := c.GetGPUInfo()
	require.Error(t, err)
	assert.Nil(t, gpus)
}

// TestGetDockerStatus_Success verifies Docker status extraction.
func TestGetDockerStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{DockerStatus: DockerStatus{LLamacpp: true, ComfyUI: false}})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	docker, err := c.GetDockerStatus()
	require.NoError(t, err)
	assert.True(t, docker.LLamacpp)
	assert.False(t, docker.ComfyUI)
}

// TestIsComfyUIRunning_True verifies true when ComfyUI is running.
func TestIsComfyUIRunning_True(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{DockerStatus: DockerStatus{ComfyUI: true}})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	running, err := c.IsComfyUIRunning()
	require.NoError(t, err)
	assert.True(t, running)
}

// TestIsComfyUIRunning_False verifies false when ComfyUI is not running.
func TestIsComfyUIRunning_False(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{DockerStatus: DockerStatus{ComfyUI: false}})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	running, err := c.IsComfyUIRunning()
	require.NoError(t, err)
	assert.False(t, running)
}

// TestCheckCanUse_Offline verifies error when status API is unreachable.
func TestCheckCanUse_Offline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	canUse, err := c.CheckCanUse()
	require.Error(t, err)
	assert.False(t, canUse)
	assert.Contains(t, err.Error(), "offline")
}

// TestCheckCanUse_Disabled verifies error when can_use is false.
func TestCheckCanUse_Disabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{CanUse: false})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	canUse, err := c.CheckCanUse()
	require.Error(t, err)
	assert.False(t, canUse)
	assert.Contains(t, err.Error(), "sorry pal")
}

// TestCheckCanUse_Success verifies success when can_use is true.
func TestCheckCanUse_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{CanUse: true})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	canUse, err := c.CheckCanUse()
	require.NoError(t, err)
	assert.True(t, canUse)
}

// TestGetFormattedStatus_WithGPUs verifies formatting when GPUs are present.
func TestGetFormattedStatus_WithGPUs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			DockerStatus: DockerStatus{LLamacpp: true, ComfyUI: true},
			GPUs: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "45 C", MemoryUsed: "8192 MiB", MemoryTotal: "24576 MiB", UtilizationGPU: "50%"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	result, err := c.GetFormattedStatus()
	require.NoError(t, err)
	assert.Contains(t, result, "Docker Containers")
	assert.Contains(t, result, "RTX 4090")
	assert.Contains(t, result, "45 C")
}

// TestGetFormattedStatus_WithError verifies formatting when status reports a GPU error.
func TestGetFormattedStatus_WithError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			Error: "nvidia-smi not found",
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	result, err := c.GetFormattedStatus()
	require.NoError(t, err)
	assert.Contains(t, result, "nvidia-smi not found")
}

// TestGetFormattedStatus_NoGPUs verifies formatting when no GPU info is available.
func TestGetFormattedStatus_NoGPUs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			DockerStatus: DockerStatus{LLamacpp: true, ComfyUI: true},
			GPUs:         []GPUInfo{},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	result, err := c.GetFormattedStatus()
	require.NoError(t, err)
	assert.Contains(t, result, "No GPU information available")
}

// TestCheckModelExecution_Offline verifies error when status API is unreachable.
func TestCheckModelExecution_Offline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	err := c.CheckModelExecution(&meta.AibirdMeta{AccessLevel: 0}, &mockUserAccess{level: 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offline")
}

// TestCheckModelExecution_ComfyUINotRunning verifies error when ComfyUI is not running.
func TestCheckModelExecution_ComfyUINotRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			CanUse:       true,
			DockerStatus: DockerStatus{ComfyUI: false},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	err := c.CheckModelExecution(&meta.AibirdMeta{AccessLevel: 0}, &mockUserAccess{level: 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "comfyui generation is not running")
}

// TestCheckModelExecution_HotGPU verifies error when a GPU exceeds temperature threshold.
func TestCheckModelExecution_HotGPU(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			CanUse:       true,
			DockerStatus: DockerStatus{ComfyUI: true},
			GPUs: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "85 C"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	err := c.CheckModelExecution(&meta.AibirdMeta{AccessLevel: 0}, &mockUserAccess{level: 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "running hot")
	assert.Contains(t, err.Error(), "85°C")
}

// TestCheckModelExecution_AccessLevelDenied verifies error when user access level is insufficient.
func TestCheckModelExecution_AccessLevelDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			CanUse:       true,
			DockerStatus: DockerStatus{ComfyUI: true},
			GPUs: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "45 C"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	err := c.CheckModelExecution(&meta.AibirdMeta{AccessLevel: 3}, &mockUserAccess{level: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access level 3")
}

// TestCheckModelExecution_Success verifies success when all checks pass.
func TestCheckModelExecution_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(StatusResponse{
			CanUse:       true,
			DockerStatus: DockerStatus{ComfyUI: true},
			GPUs: []GPUInfo{
				{Name: "NVIDIA GeForce RTX 4090", Temperature: "45 C"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(&settings.AiBird{StatusUrl: server.URL, StatusApiKey: "key"})
	err := c.CheckModelExecution(&meta.AibirdMeta{AccessLevel: 1}, &mockUserAccess{level: 5})
	require.NoError(t, err)
}

// mockUserAccess implements UserAccess for testing.
type mockUserAccess struct {
	level int
}

func (m *mockUserAccess) GetAccessLevel() int { return m.level }
