package cliout

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNotesToIsStderrOnlyUnderJSON(t *testing.T) {
	cmd := &cobra.Command{}
	var out, errOut bytes.Buffer
	cmd.SetErr(&errOut)
	assert.Same(t, &out, NotesTo(cmd, FormatText, &out), "text: the command's own writer, as always")
	assert.Same(t, &errOut, NotesTo(cmd, FormatJSON, &out), "json: stderr, so stdout holds the one result")
}
