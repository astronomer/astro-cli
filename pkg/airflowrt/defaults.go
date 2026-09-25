package airflowrt

import (
	"os"
	"strconv"
	"strings"
)

const (
	// StandaloneIndexURL is the package index a standalone environment installs
	// Astro Runtime's Airflow from. Astronomer's own, not PyPI: the Runtime
	// builds carry a +astro local version that PyPI does not serve.
	StandaloneIndexURL = "https://pip.astronomer.io/v2/"
	// DefaultPython is the interpreter assumed when a project names none. A
	// caller choosing the interpreter for a project goes through PythonFallback,
	// which also knows the Airflow releases this one is too new for.
	DefaultPython = "3.12"
	// StandaloneDir is the per-project directory holding a standalone Airflow's
	// sidecar state, relative to the project root.
	StandaloneDir = ".astro/standalone"

	FilePermissions = os.FileMode(0o644)
	DirPermissions  = os.FileMode(0o755)
)

// pythonBeforeAirflow29 is the newest interpreter an Airflow 2 before 2.9 runs
// under: 3.12 support arrived in 2.9.
const pythonBeforeAirflow29 = "3.11"

// PythonFallback is the interpreter to request for a project, given its
// manifest's [project] requires-python and [tool.astro] airflow: "" when the
// manifest states requires-python, else a concrete version for the pin. It is
// the one rule the CLI and Astro Desktop both apply, so a project whose
// manifest states no requires-python gets the same interpreter from either.
//
// A stated requires-python wins, and "" says so: uv reads it from the
// manifest when it syncs the project, and forcing an interpreter past it would
// fail the resolution rather than honor it. A caller building a venv outside
// the project, where uv cannot see the manifest, passes the requires-python
// itself when this returns "".
//
// Without one, uv resolves against the newest CPython it knows of. Airflow 3
// takes that in stride, but Airflow 2's published metadata carries no upper
// bound, so uv installs it on an interpreter it cannot run and the project
// fails at start from inside werkzeug. So the fallback is a version and not
// "": DefaultPython, or pythonBeforeAirflow29 for a 2.x pin below 2.9. That is
// the same ceiling pkg/scaffold's requiresPython writes into a new manifest,
// so an unset requires-python lands where `astro init` would have put it.
//
// `astro init` and a v1 conversion both write requires-python, so this is for
// a hand-written manifest, or one that declares requires-python dynamic.
func PythonFallback(requiresPython, airflowVersion string) string {
	if strings.TrimSpace(requiresPython) != "" {
		return ""
	}
	major, rest, _ := strings.Cut(strings.TrimSpace(airflowVersion), ".")
	if major == "2" {
		// A bare "2" means the newest Airflow 2, which is past 2.9, and falls
		// through with it: Atoi fails on the empty minor.
		minor, _, _ := strings.Cut(rest, ".")
		if n, err := strconv.Atoi(minor); err == nil && n < 9 {
			return pythonBeforeAirflow29
		}
	}
	return DefaultPython
}
