package astro

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// emitDeployment under json reads the Deployment afresh; a command run with no
// context (RunE called without ExecuteContext) still publishes it.
func TestEmitDeploymentAcceptsANilContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	d := coreDeployment()
	prev := astroV1Client
	astroV1Client = coreMock(t, d)
	t.Cleanup(func() { astroV1Client = prev })

	var out bytes.Buffer
	r := cliout.Renderer{Format: cliout.FormatJSON, Out: &out, Err: io.Discard}
	require.NotPanics(t, func() {
		//nolint:staticcheck // a nil context is the case under test
		require.NoError(t, emitDeployment(nil, r, &d, func(io.Writer) error { return nil }))
	})
	require.Contains(t, out.String(), coreDeploymentID)
}
