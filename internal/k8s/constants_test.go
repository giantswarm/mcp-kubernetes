package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultNamespace verifies the default namespace constant.
func TestDefaultNamespace(t *testing.T) {
	assert.Equal(t, "default", DefaultNamespace,
		"DefaultNamespace should be 'default' following kubectl behavior")
}
