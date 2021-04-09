package astrohub

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetValidTagsSimpleSemVer(t *testing.T) {
	dCfg := DeploymentConfig{
		AirflowImages: []AirflowImage{
			{Tag: "1.10.5-11-alpine3.10-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-alpine3.10", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster", Version: "1.10.5-11"},
			{Tag: "1.10.5-buster-onbuild", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10", Version: "1.10.5"},
			{Tag: "1.10.5-buster", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10-onbuild", Version: "1.10.5"},
			{Tag: "1.10.7-7-buster-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-buster", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-8-alpine3.10-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-alpine3.10", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster", Version: "1.10.7-8"},
		},
	}
	validTags := dCfg.GetValidTags("1.10.7")
	expectedTags := []string{
		"1.10.7-7-buster-onbuild",
		"1.10.7-7-alpine3.10",
		"1.10.7-7-buster",
		"1.10.7-7-alpine3.10-onbuild",
		"1.10.7-8-alpine3.10-onbuild",
		"1.10.7-8-buster-onbuild",
		"1.10.7-8-alpine3.10",
		"1.10.7-8-buster"}
	assert.Equal(t, expectedTags, validTags)
}

func TestGetValidTagsWithPreReleaseTag(t *testing.T) {
	dCfg := DeploymentConfig{
		AirflowImages: []AirflowImage{
			{Tag: "1.10.5-11-alpine3.10-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-alpine3.10", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster", Version: "1.10.5-11"},
			{Tag: "1.10.5-buster-onbuild", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10", Version: "1.10.5"},
			{Tag: "1.10.5-buster", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10-onbuild", Version: "1.10.5"},
			{Tag: "1.10.7-7-buster-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-buster", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-8-alpine3.10-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-alpine3.10", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster", Version: "1.10.7-8"},
			{Tag: "1.10.12-buster", Version: "1.10.12"},
			{Tag: "1.10.12-buster-onbuild", Version: "1.10.12"},
			{Tag: "1.10.12-1-buster-onbuild", Version: "1.10.12-1"},
		},
	}
	validTags := dCfg.GetValidTags("1.10.12-1-alpine3.10")
	expectedTags := []string{"1.10.12-buster", "1.10.12-buster-onbuild", "1.10.12-1-buster-onbuild"}
	assert.Equal(t, expectedTags, validTags)
}

func Test_coerce(t *testing.T) {
	assert.Equal(t, "1.10.5", coerce("1.10.5-11-alpine3.10-onbuild").String())
	assert.Equal(t, "1.10.5", coerce("1.10.5").String())
	assert.Equal(t, "1.10.12", coerce("1.10.12-buster").String())
}
