package meta

type AibirdMeta struct {
	AccessLevel int
	BigModel    bool
}

type GPUType string

const (
	GPU4090 GPUType = "4090"
	GPU2070 GPUType = "2070"
)
