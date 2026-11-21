package config

import (
	"bytes"
)

// Piped, the context table is its header and row, tab-separated.
func (s *Suite) TestPrintContextIsTabSeparatedWhenPiped() {
	ctx := Context{Domain: "example.com", Workspace: "ck05r3bor07h40d02y2hw4n4v"}
	buf := new(bytes.Buffer)
	s.NoError(ctx.PrintCloudContext(buf))
	s.Equal("CONTROLPLANE\tWORKSPACE\nexample.com\tck05r3bor07h40d02y2hw4n4v\n", buf.String())
}
