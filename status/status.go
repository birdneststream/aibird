package status

import (
	"aibird/settings"
	"aibird/shared/meta"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type GPUInfo struct {
	Name           string `json:"name"`
	Temperature    string `json:"temperature"`
	MemoryUsed     string `json:"memory_used"`
	MemoryTotal    string `json:"memory_total"`
	PowerDraw      string `json:"power_draw"`
	FanSpeed       string `json:"fan_speed"`
	UtilizationGPU string `json:"utilization_gpu"`
}

type DockerStatus struct {
	Ollama      bool `json:"ollama"`
	ComfyUI     bool `json:"comfyui"`
	ComfyUI2070 bool `json:"comfyui_2070"`
}

type StatusResponse struct {
	IsRunning    bool         `json:"steam_running"`
	DockerStatus DockerStatus `json:"docker_status"`
	GPUs         []GPUInfo    `json:"gpus,omitempty"`
	Error        string       `json:"error,omitempty"`
}

type Client struct {
	BaseURL         string
	StatusApiKey    string
	HTTPClient      *http.Client
	mu              sync.Mutex
	cachedStatus    *StatusResponse
	statusCacheTime time.Time
	cachedWavs      []string
	wavsCacheTime   time.Time
}

// NewClient creates a new status client
func NewClient(config settings.AiBird) *Client {
	return &Client{
		BaseURL:      config.StatusUrl,
		StatusApiKey: config.StatusApiKey,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) doRequest(endpoint string, target interface{}) error {
	url := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("X-API-Key", c.StatusApiKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	return nil
}

// GetStatus fetches the current status from the status API
func (c *Client) GetStatus() (*StatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedStatus != nil && time.Since(c.statusCacheTime) < 3*time.Second {
		return c.cachedStatus, nil
	}
	var status StatusResponse
	if err := c.doRequest("/api/status", &status); err != nil {
		return nil, err
	}
	c.cachedStatus = &status
	c.statusCacheTime = time.Now()
	return &status, nil
}

// GetWavs fetches the list of voices
func (c *Client) GetWavs() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedWavs != nil && time.Since(c.wavsCacheTime) < 3*time.Second {
		return c.cachedWavs, nil
	}
	var wavs []string
	if err := c.doRequest("/api/wavs", &wavs); err != nil {
		return nil, err
	}

	// Deduplicate the list
	uniqueWavs := removeStringDuplicates(wavs)

	c.cachedWavs = uniqueWavs
	c.wavsCacheTime = time.Now()
	return uniqueWavs, nil
}

// removeStringDuplicates removes duplicate strings from a slice of strings
func removeStringDuplicates(elements []string) []string {
	encountered := make(map[string]bool)
	result := []string{}
	for v := range elements {
		if !encountered[elements[v]] {
			encountered[elements[v]] = true
			result = append(result, elements[v])
		}
	}
	return result
}

// AddVoice sends a request to the birdcheck service to add a new voice.
func (c *Client) AddVoice(voiceURL, voiceName, startTime, duration string) (string, error) {
	endpoint := "/api/add_voice"
	url := fmt.Sprintf("%s%s", c.BaseURL, endpoint)

	requestBody := struct {
		URL       string `json:"url"`
		VoiceName string `json:"voice_name"`
		StartTime string `json:"start_time,omitempty"`
		Duration  string `json:"duration,omitempty"`
	}{
		URL:       voiceURL,
		VoiceName: voiceName,
		StartTime: startTime,
		Duration:  duration,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.StatusApiKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var response map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	return response["message"], nil
}

// IsSteamRunning returns just the steam status
func (c *Client) IsSteamRunning() (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, err
	}
	return status.IsRunning, nil
}

// GetGPUInfo returns just the GPU information
func (c *Client) GetGPUInfo() ([]GPUInfo, error) {
	status, err := c.GetStatus()
	if err != nil {
		return nil, err
	}
	if status.Error != "" {
		return nil, fmt.Errorf("gpu error: %s", status.Error)
	}
	return status.GPUs, nil
}

// GetDockerStatus returns just the Docker container status
func (c *Client) GetDockerStatus() (*DockerStatus, error) {
	status, err := c.GetStatus()
	if err != nil {
		return nil, err
	}
	return &status.DockerStatus, nil
}

// IsOllamaRunning returns true if the Ollama container is running
func (c *Client) IsOllamaRunning() (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, err
	}
	return status.DockerStatus.Ollama, nil
}

// IsComfyUIRunning returns true if the ComfyUI container is running
func (c *Client) IsComfyUIRunning() (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, err
	}
	return status.DockerStatus.ComfyUI, nil
}

// IsComfyUI2070Running returns true if the ComfyUI-2070 container is running
func (c *Client) IsComfyUI2070Running() (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, err
	}
	return status.DockerStatus.ComfyUI2070, nil
}

// formatGPUInfo formats a single GPU's information
func formatGPUInfo(gpu GPUInfo) string {
	shortName := strings.Replace(gpu.Name, "NVIDIA GeForce ", "", 1)
	return fmt.Sprintf("%s [Temp: %s | Mem: %s/%s | Usage: %s]",
		shortName,
		gpu.Temperature,
		strings.TrimSuffix(gpu.MemoryUsed, " MiB"),
		strings.TrimSuffix(gpu.MemoryTotal, " MiB"),
		gpu.UtilizationGPU)
}

// formatDockerStatus formats the Docker container status
func formatDockerStatus(docker DockerStatus) string {
	format := func(name string, status bool) string {
		if status {
			return fmt.Sprintf("🟢 %s", name)
		}
		return fmt.Sprintf("⚫ %s", name)
	}

	statuses := []string{
		format("Ollama", docker.Ollama),
		format("ComfyUI", docker.ComfyUI),
		format("ComfyUI-2070", docker.ComfyUI2070),
	}
	return strings.Join(statuses, " | ")
}

// GetFormattedStatus returns a formatted string suitable for IRC with newline separation
func (c *Client) GetFormattedStatus() (string, error) {
	status, err := c.GetStatus()
	if err != nil {
		return fmt.Sprintf("Error fetching status: %v", err), nil
	}

	var lines []string

	// Add Docker status
	lines = append(lines, fmt.Sprintf(" • Docker Containers: %s", formatDockerStatus(status.DockerStatus)))

	// Add GPU status
	if status.Error != "" {
		lines = append(lines, fmt.Sprintf("GPUs: ❌ Error: %s", status.Error))
	} else if len(status.GPUs) > 0 {
		steamStatus := "🟢 Available"
		if status.IsRunning {
			steamStatus = "🔴 In Use (2070 slow lane for you)"
		}
		for _, gpu := range status.GPUs {
			gpuLine := formatGPUInfo(gpu)
			if strings.Contains(gpu.Name, "4090") {
				gpuLine = fmt.Sprintf("%s | Status: %s", gpuLine, steamStatus)
			}
			lines = append(lines, fmt.Sprintf(" • %s", gpuLine))
		}
	} else {
		lines = append(lines, "GPUs: ❌ No GPU information available")
	}

	return strings.Join(lines, "\n"), nil
}

// UserAccess defines the necessary methods for a user object to perform access checks.
type UserAccess interface {
	GetAccessLevel() int
	CanUse4090() bool
}

// CheckModelExecution performs various checks to see if a model can be executed.
// It returns a boolean indicating if the high-performance port should be used, and an error if execution is not permitted.
func (c *Client) CheckModelExecution(model string, meta *meta.AibirdMeta, user UserAccess, userNickName string) (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, errors.New("AI rig is offline!!! Sorry pal")
	}

	isSteamRunning := status.IsRunning
	comfyUi4090Running := status.DockerStatus.ComfyUI
	comfyUi2070Running := status.DockerStatus.ComfyUI2070

	if !comfyUi2070Running && !comfyUi4090Running {
		return false, errors.New("AI rig seems online but the comfyui generation is not running!!! Sorry pal")
	}

	// Access level check
	if user.GetAccessLevel() < meta.AccessLevel {
		return false, fmt.Errorf("⛔️ Sorry, you need access level %d to use this command. Check !support for more info", meta.AccessLevel)
	}

	// Big model check
	if meta.BigModel && (isSteamRunning || !comfyUi4090Running) {
		return false, errors.New("no GPU for these big models, sorry pal, have to wait for jewbird to stop gayming")
	}

	use4090Port := false
	if !isSteamRunning && comfyUi4090Running {
		if user.CanUse4090() || meta.BigModel {
			use4090Port = true
		}
	}

	return use4090Port, nil
}
