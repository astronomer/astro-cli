package astro

import (
	"fmt"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// errRequiredFlag returns the usage error for a missing required flag: exit 2,
// kind usage, as cobra's own required-flag check. It is for a flag the command
// cannot do without whatever the terminal, so it is not for a value that could
// be asked for at a prompt (that is input.Required).
// hint should tell the user how to find valid values (e.g., "astro deployment list").
//
//nolint:unparam // signature kept for consistency
func errRequiredFlag(flag, hint string) error {
	if hint != "" {
		return cliout.Usage(fmt.Errorf("required flag --%s not set. To find valid values, run: %s", flag, hint))
	}
	return cliout.Usage(fmt.Errorf("required flag --%s not set. See --help for usage", flag))
}
