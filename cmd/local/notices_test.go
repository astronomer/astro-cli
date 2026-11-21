package local

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/pkg/texttable"
)

// captureNotices collects what an empty listing tells a person (on stderr
// outside a test) for the length of the test.
func captureNotices(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	texttable.Notices = &buf
	t.Cleanup(func() { texttable.Notices = nil })
	return &buf
}
