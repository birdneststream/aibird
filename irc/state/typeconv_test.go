package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToBoolE(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    bool
		wantErr bool
	}{
		{"bool true", true, true, false},
		{"bool false", false, false, false},
		{"string true", "true", true, false},
		{"string false", "false", false, false},
		{"string 1", "1", true, false},
		{"string 0", "0", false, false},
		{"int non-zero", 1, true, false},
		{"int zero", 0, false, false},
		{"int64 non-zero", int64(5), true, false},
		{"int64 zero", int64(0), false, false},
		{"unsupported type", 3.14, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toBoolE(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToIntE(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    int
		wantErr bool
	}{
		{"int value", 42, 42, false},
		{"int64 value", int64(100), 100, false},
		{"string value", "42", 42, false},
		{"float64 value", float64(42), 42, false},
		{"string invalid", "notanumber", 0, true},
		{"unsupported type", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toIntE(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToInt64E(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    int64
		wantErr bool
	}{
		{"int64 value", int64(100), 100, false},
		{"int value", 42, 42, false},
		{"string value", "100", 100, false},
		{"float64 value", float64(42), 42, false},
		{"string invalid", "abc", 0, true},
		{"unsupported type", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toInt64E(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToFloat64E(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    float64
		wantErr bool
	}{
		{"float64 value", 3.14, 3.14, false},
		{"int value", 42, 42.0, false},
		{"int64 value", int64(100), 100.0, false},
		{"string value", "3.14", 3.14, false},
		{"string invalid", "abc", 0, true},
		{"unsupported type", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat64E(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"string value", "hello", "hello"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int value", 42, "42"},
		{"int64 value", int64(100), "100"},
		{"float64 value", 3.14, "3.14"},
		{"nil value", nil, "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
