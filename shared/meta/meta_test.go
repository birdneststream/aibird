package meta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGPUType_Constants(t *testing.T) {
	assert.Equal(t, GPUType("4090"), GPU4090)
	assert.Equal(t, "4090", string(GPU4090))
}

func TestAibirdMeta_Initialization(t *testing.T) {
	meta := AibirdMeta{
		AccessLevel: 5,
	}
	assert.Equal(t, 5, meta.AccessLevel)
}

func TestGPUType_Equality(t *testing.T) {
	assert.True(t, GPU4090 == "4090")
	assert.False(t, GPU4090 == "5090")
}
