// Package status provides API client for the AI service status and model execution checks.
package status

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

	result := formatGPUInfo(gpu)

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
