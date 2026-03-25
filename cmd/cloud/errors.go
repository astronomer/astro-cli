package cloud

import "fmt"

// errRequiredFlag returns a standardized error for missing required flags.
// hint should tell the user how to find valid values (e.g., "astro deployment list").
//
//nolint:unparam
func errRequiredFlag(flag, hint string) error {
	if hint != "" {
		return fmt.Errorf("required flag --%s not set. To find valid values, run: %s", flag, hint)
	}
	return fmt.Errorf("required flag --%s not set. See --help for usage", flag)
}
