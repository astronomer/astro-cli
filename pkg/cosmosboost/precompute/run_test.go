package precompute

import "time"

// Run discovers the units under roots and runs them, as a deploy does in two
// steps, timing both.
func Run(roots []string, version string, opts Options) (Summary, error) {
	start := time.Now()
	units, err := Discover(roots)
	if err != nil {
		return Summary{}, err
	}
	summary := units.Run(version, opts)
	summary.Duration = time.Since(start)
	return summary, nil
}
