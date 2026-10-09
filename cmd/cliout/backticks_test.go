package cliout

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A failure's message names a command in backticks. Execute prints it to a
// stream that is not a terminal without them, and the error object carries
// it as plain text; the error the run returns keeps them, for whoever renders
// it next.
func TestFailuresRenderTheirBackticks(t *testing.T) {
	for _, v := range []string{"CLICOLOR_FORCE", "CLICOLOR", "NO_COLOR"} {
		t.Setenv(v, "")
		require.NoError(t, os.Unsetenv(v))
	}
	fail := errors.New("this run needs a login: run `astro login` first, or set `ASTRO_API_TOKEN`")
	const plain = "this run needs a login: run astro login first, or set ASTRO_API_TOKEN"

	text := execTree(context.Background(), testTree(fail), "thing", "list")
	require.ErrorIs(t, text.err, fail)
	assert.Contains(t, text.stderr, "Error: "+plain+"\n")
	assert.NotContains(t, text.stderr, "`")

	plainCmd := execTree(context.Background(), testTree(fail), "thing", "plain")
	assert.Contains(t, plainCmd.stderr, "Error: "+plain+"\n", "a command with no --output too")

	asJSON := execTree(context.Background(), testTree(fail), "thing", "list", "-o", "json")
	assert.Empty(t, asJSON.stderr)
	assert.Equal(t, plain, decodeOne(t, asJSON.stdout).Error)
}
