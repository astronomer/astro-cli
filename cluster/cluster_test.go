package cluster

import (
	"testing"

	testUtil "github.com/sjmiller609/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	testUtil.InitTestConfig()
	expected := false
	// Check that we don't have localhost123 in test config from testUtils.NewTestConfig()
	assert.Equal(t, expected, Exists("localhost123"))
}
