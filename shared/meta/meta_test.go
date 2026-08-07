package meta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAibirdMeta_Initialization(t *testing.T) {
	meta := AibirdMeta{
		AccessLevel: 5,
	}
	assert.Equal(t, 5, meta.AccessLevel)
}
