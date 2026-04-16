package meta

type AibirdMeta struct {
	AccessLevel int
}

type GPUType string

const (
	// GPU5090 is the RTX 5090 on cuda:1 - used when Steam is NOT running
	GPU5090 GPUType = "5090"
	// GPU4090 is the RTX 4090 on cuda:0 - used when Steam IS running (fallback)
	GPU4090 GPUType = "4090"

	// Deprecated: Use GPU5090 instead
	// GPU4090Old is kept for backwards compatibility during transition
	// GPU4090Old GPUType = "5090"
	// GPU2070 is kept for backwards compatibility during transition
	// GPU2070 GPUType = "4090"
)
