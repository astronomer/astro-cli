package airflowrt

import "os"

const (
	// StandaloneIndexURL is the package index a standalone environment installs
	// Astro Runtime's Airflow from. Astronomer's own, not PyPI: the Runtime
	// builds carry a +astro local version that PyPI does not serve.
	StandaloneIndexURL = "https://pip.astronomer.io/v2/"
	// DefaultPython is the interpreter assumed when a project names none.
	DefaultPython = "3.12"
	// StandaloneDir is the per-project directory holding a standalone Airflow's
	// sidecar state, relative to the project root.
	StandaloneDir = ".astro/standalone"

	FilePermissions = os.FileMode(0o644)
	DirPermissions  = os.FileMode(0o755)
)
