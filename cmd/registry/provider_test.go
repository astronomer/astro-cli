package registry

import (
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestProviderAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	assert.Equal(t, 1, 1)
}

func TestProviderList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	assert.Equal(t, 1, 0)
}
