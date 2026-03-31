package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	assert.Empty(t, ConfigData)

	InitTestProvider(t)

	assert.NotEmpty(t, ConfigData)
}
