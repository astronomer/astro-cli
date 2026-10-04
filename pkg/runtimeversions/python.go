package runtimeversions

import (
	"fmt"
	"strings"
)

// ProjectPython is the Python minor a project runs, in a generated image and in
// a standalone venv alike, for its Airflow pin, its [tool.astro] runtime build
// ("" for none) and its [project] requires-python. One answer for both modes,
// so a project does not run 3.13 in its image and 3.14 in its venv.
//
// The build is the one runtime names, or else the newest published build of
// the pin's series, the one the series tag (runtime:3.3) serves: a patch pin
// still reads the series, so requires-python picks the Python and never the
// Airflow. The Python is that build's default when requires-python admits it,
// and otherwise the newest it ships that requires-python admits
// (Runtime.ImagePython). build is the exact build when that Python is not the
// default, which is when an image needs runtime:<build>-python-X.Y; it is ""
// when the build's default is chosen, so the series tag still serves.
//
// python is "" when there is nothing to decide on: no requires-python, a pin
// that is not Airflow 3 or names no series, no catalog (catalog nil or
// returning nil, as offline), no build of the pin in it, a build that lists no
// Python, or a requires-python this does not read. A caller then keeps its own
// rule. catalog is called only when requires-python is set.
//
// A build that ships no Python requires-python admits is
// *PythonNotShippedError, and python is "".
func ProjectPython(airflowVersion, runtime, requiresPython string, catalog func() *Catalog) (python, build string, err error) {
	if strings.TrimSpace(requiresPython) == "" || catalog == nil {
		return "", "", nil
	}
	pin := strings.TrimSpace(airflowVersion)
	if majorOf(pin) != "3" {
		return "", "", nil
	}
	series := seriesOf(pin)
	if series == "" {
		return "", "", nil
	}
	c := catalog()
	if c == nil {
		return "", "", nil
	}
	build = strings.TrimSpace(runtime)
	if build == "" {
		var ok bool
		if build, ok = c.NewestPublishedRuntimeFor(series); !ok {
			return "", "", nil
		}
	}
	r, ok := c.Runtime(build)
	if !ok {
		return "", "", nil
	}
	python, ok = r.ImagePython(requiresPython)
	switch {
	case !ok:
		return "", "", nil
	case python == "":
		return "", "", &PythonNotShippedError{
			RequiresPython: strings.TrimSpace(requiresPython),
			Build:          build,
			Pythons:        append([]string(nil), r.PythonVersions...),
		}
	case python == strings.TrimSpace(r.DefaultPythonVersion):
		return python, "", nil
	}
	return python, build, nil
}

// PythonNotShippedError is a requires-python that admits none of the Pythons
// a runtime build ships. Callers branch with errors.As.
type PythonNotShippedError struct {
	RequiresPython string
	Build          string
	Pythons        []string
}

func (e *PythonNotShippedError) Error() string {
	return fmt.Sprintf("requires-python %s in pyproject.toml admits none of the Pythons runtime %s ships (%s)",
		e.RequiresPython, e.Build, strings.Join(e.Pythons, ", "))
}

// ImagePython is the Python a generated image of this build runs for a
// project's [project] requires-python: the build's default when
// requires-python admits it, so a range that already covers the image keeps
// it, and otherwise the newest one the build ships that it admits.
//
// ok is false when there is nothing to decide: no requires-python, a build
// that lists no Python (every Airflow 2 build), or a specifier this does not
// read. With ok true and python empty, the build ships no Python that
// requires-python admits.
//
// A shipped Python is a minor version, "3.13", and the image runs some patch
// of it, so a clause admits the minor when some patch of it satisfies the
// clause: ">=3.13.2" admits 3.13, and "<3.13" does not. The clauses are
// checked one at a time, as uv picks an interpreter by its minor.
func (r *Runtime) ImagePython(requiresPython string) (python string, ok bool) {
	clauses, ok := parsePythonSpecifier(requiresPython)
	if !ok || len(r.PythonVersions) == 0 {
		return "", false
	}
	if d := strings.TrimSpace(r.DefaultPythonVersion); numericVersion(d) && admitsMinor(clauses, d) {
		return d, true
	}
	for _, v := range r.PythonVersions {
		v = strings.TrimSpace(v)
		if !numericVersion(v) || !admitsMinor(clauses, v) {
			continue
		}
		if python == "" || compareVersions(v, python) > 0 {
			python = v
		}
	}
	return python, true
}

type pythonClause struct {
	op       string
	version  string
	wildcard bool
}

// pythonOperators is longest first, so "<=" is not read as "<".
var pythonOperators = []string{"~=", "==", "!=", "<=", ">=", "<", ">"}

// parsePythonSpecifier reads a PEP 440 specifier set of plain release
// versions. A pre-release, a local version, "===" or a wildcard where PEP 440
// allows none report false.
func parsePythonSpecifier(spec string) ([]pythonClause, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, false
	}
	var clauses []pythonClause
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		var c pythonClause
		for _, op := range pythonOperators {
			if strings.HasPrefix(part, op) && !strings.HasPrefix(part, "===") {
				c.op = op
				break
			}
		}
		if c.op == "" {
			return nil, false
		}
		v := strings.TrimSpace(strings.TrimPrefix(part, c.op))
		c.version = strings.TrimSuffix(v, ".*")
		c.wildcard = c.version != v
		switch {
		case !numericVersion(c.version):
			return nil, false
		case c.wildcard && c.op != "==" && c.op != "!=":
			return nil, false
		case c.op == "~=" && !strings.Contains(c.version, "."):
			return nil, false
		}
		clauses = append(clauses, c)
	}
	return clauses, true
}

func admitsMinor(clauses []pythonClause, minor string) bool {
	for _, c := range clauses {
		if !c.admitsMinor(minor) {
			return false
		}
	}
	return true
}

func (c pythonClause) admitsMinor(minor string) bool {
	switch c.op {
	case "==":
		return c.matchesMinor(minor)
	case "!=":
		return !c.wildcard || !c.matchesMinor(minor) || len(segments(c.version)) > 2
	case ">=", ">":
		return compareVersions(minor, firstTwo(c.version)) >= 0
	case "<=":
		return compareVersions(minor, c.version) <= 0
	case "<":
		return compareVersions(minor, c.version) < 0
	case "~=":
		prefix := pythonClause{version: c.version[:strings.LastIndex(c.version, ".")], wildcard: true}
		return compareVersions(minor, firstTwo(c.version)) >= 0 && prefix.matchesMinor(minor)
	}
	return false
}

// matchesMinor reports whether some patch of minor equals the clause's
// version, or starts with it for a wildcard. "3.*" names every 3.x, and
// "3.13", "3.13.*" and "3.13.1" all name some patch of 3.13.
func (c pythonClause) matchesMinor(minor string) bool {
	if c.wildcard && len(segments(c.version)) == 1 {
		return majorOf(minor) == c.version
	}
	return compareVersions(minor, firstTwo(c.version)) == 0
}

// firstTwo is a version's major.minor, "3.13" from "3.13.2", or the version
// itself when it names fewer.
func firstTwo(version string) string {
	parts := segments(version)
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, ".")
}
