// Package meta defines shared metadata types used across the application.
package meta

type AibirdMeta struct {
	AccessLevel int
}

type GPUType string

const (
	// GPU4090 is the RTX 4090 on cuda:0
	GPU4090 GPUType = "4090"
)
