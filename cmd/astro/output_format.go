package astro

import (
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/output"
)

// resolveOutput is f.Resolve with a bad value marked as a usage error, so it
// exits 2 and publishes kind usage like a bad --output anywhere else. pkg/output
// cannot mark it itself: a pkg/ package does not import cmd/.
func resolveOutput(f *output.Flags) (output.Format, error) {
	format, err := f.Resolve()
	if err != nil {
		return "", cliout.Usage(err)
	}
	return format, nil
}
