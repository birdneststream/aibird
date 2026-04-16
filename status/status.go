package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"aibird/logger"
	"aibird/settings"
	"aibird/shared/meta"
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
	Ollama  bool `json:"ollama"`
	ComfyUI bool `json:"comfyui"`
}

type StatusResponse struct {
	IsRunning    bool         `json:"steam_running"`
	DockerStatus DockerStatus `json:"docker_status"`
	GPUs         []GPUInfo    `json:"gpus,omitempty"`
	CanUse       bool         `json:"can_use"`
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
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Client) doRequest(endpoint string, target interface{}) error {
	url := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	req, err := http.NewRequest("GET", url, http.NoBody)
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

// CheckCanUse returns whether the system allows GPU usage.
// This is a simpler check than CheckModelExecution - it only validates
// can_use flag and basic service availability.
func (c *Client) CheckCanUse() (bool, error) {
	status, err := c.GetStatus()
	if err != nil {
		return false, errors.New("AI rig is offline!!! Sorry pal")
	}

	if !status.CanUse {
		return false, errors.New("sorry pal jewbird is doing secret ai experiments and requires all gpus")
	}

	return true, nil
}

// parseTemperature extracts the numeric temperature value from a string like "45 C" or "45°C"
func parseTemperature(temp string) (int, error) {
	// Remove common suffixes and whitespace
	temp = strings.TrimSpace(temp)
	temp = strings.TrimSuffix(temp, "°C")
	temp = strings.TrimSuffix(temp, " C")
	temp = strings.TrimSuffix(temp, "C")
	temp = strings.TrimSpace(temp)

	var tempVal int
	_, err := fmt.Sscanf(temp, "%d", &tempVal)
	return tempVal, err
}

// checkGPUTemperatures checks if any GPU is over the temperature threshold
// Returns the GPU name and temperature if any GPU is too hot, empty string otherwise
func checkGPUTemperatures(gpus []GPUInfo, threshold int) (string, int) {
	for _, gpu := range gpus {
		temp, err := parseTemperature(gpu.Temperature)
		if err != nil {
			continue
		}
		if temp > threshold {
			shortName := strings.Replace(gpu.Name, "NVIDIA GeForce ", "", 1)
			return shortName, temp
		}
	}
	return "", 0
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
			steamStatus = "🔴 In Use (4090 lane for you)"
		}
		for _, gpu := range status.GPUs {
			gpuLine := formatGPUInfo(gpu)
			if strings.Contains(gpu.Name, "5090") {
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
	CanUsePremiumGPU() bool
}

// CheckModelExecution performs pre-flight checks before executing a ComfyUI model.
// Returns an error if execution is not permitted.
func (c *Client) CheckModelExecution(metaData *meta.AibirdMeta, user UserAccess) error {
	status, err := c.GetStatus()
	if err != nil {
		return errors.New("AI rig is offline!!! Sorry pal")
	}

	comfyUIRunning := status.DockerStatus.ComfyUI
	canUse := status.CanUse

	logger.Debug("CheckModelExecution",
		"comfyui", comfyUIRunning,
		"canUse", canUse,
		"userAccessLevel", user.GetAccessLevel(),
		"canUsePremiumGPU", user.CanUsePremiumGPU(),
	)

	// Check if bot usage is allowed
	if !canUse {
		return errors.New("sorry pal jewbird is doing secret ai experiments and requires all gpus")
	}

	if !comfyUIRunning {
		return errors.New("AI rig seems online but the comfyui generation is not running!!! Sorry pal")
	}

	// Check GPU temperatures - warn if any GPU is over 80°C
	const tempThreshold = 80
	if hotGPU, temp := checkGPUTemperatures(status.GPUs, tempThreshold); hotGPU != "" {
		return fmt.Errorf("🌡️ GPU %s is running hot at %d°C! Please wait a moment and try again when it cools down", hotGPU, temp)
	}

	// Access level check
	if user.GetAccessLevel() < metaData.AccessLevel {
		return fmt.Errorf("⛔️ Sorry, you need access level %d to use this command. Check !support for more info", metaData.AccessLevel)
	}

	return nil
}
