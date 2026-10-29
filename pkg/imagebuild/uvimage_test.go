package imagebuild

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

func TestAGeneratedBuildDefaultsToTheUVImagePin(t *testing.T) {
	t.Setenv(airflowrt.UVImageArg, "")
	assert.Equal(t, airflowrt.UVImage, UVImage)
	assert.Equal(t, airflowrt.UVImage, uvImage())
}
