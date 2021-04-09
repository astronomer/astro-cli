package cluster

import (
	"bytes"
	"github.com/astronomer/astro-cli/config"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	testUtil.InitTestConfig()
	// Check that we don't have localhost123 in test config from testUtils.NewTestConfig()
	assert.False(t, Exists("localhost123"))
}

func TestList(t *testing.T) {
	testUtil.InitTestConfig()
	buf := new(bytes.Buffer)
	err := List(buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), testUtil.GetEnv("ASTROHUB_HOST", "localhost"))
}

func TestGetCurrentCluster(t *testing.T) {
	testUtil.InitTestConfig()
	cluster, err := GetCurrentCluster()
	assert.NoError(t, err)
	assert.Equal(t, cluster.Domain, testUtil.GetEnv("ASTROHUB_HOST", "localhost"))
	assert.Equal(t, cluster.Workspace, "ck05r3bor07h40d02y2hw4n4v")
	assert.Equal(t, cluster.LastUsedWorkspace, "ck05r3bor07h40d02y2hw4n4v")
	assert.Equal(t, cluster.Token, "token")
}

func TestPrintContext(t *testing.T) {
	testUtil.InitTestConfig()
	buf := new(bytes.Buffer)
	cluster, err := GetCurrentCluster()
	assert.NoError(t, err)

	err = cluster.PrintContext(buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), testUtil.GetEnv("ASTROHUB_HOST", "localhost"))
}

func TestGetContextKeyValidClusterConfig(t *testing.T) {
	c := config.Context{Domain: "test.com"}
	key, err := c.GetContextKey()
	assert.NoError(t, err)
	assert.Equal(t, key, "test_com")
}

func TestGetContextKeyInvalidClusterConfig(t *testing.T) {
	c := config.Context{}
	_, err := c.GetContextKey()
	assert.EqualError(t, err, "cluster config invalid, no domain specified")
}
