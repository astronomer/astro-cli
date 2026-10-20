package runtimeversions

import (
	"strconv"
	"strings"
)

// ImagePythonOf is the Python the image of build runs when its tag names
// none. An Airflow 3 build's is the catalog's default, so c is needed; an
// Airflow 2 build lists no Python in the catalog, and its image runs 3.12
// from runtime 12 on and 3.11 before, as the images themselves say
// (PYTHON_VERSION in each one's config). "" when it is not known.
func ImagePythonOf(c *Catalog, build string) string {
	build = strings.TrimSpace(build)
	if c != nil {
		if r, ok := c.Runtime(build); ok && strings.TrimSpace(r.DefaultPythonVersion) != "" {
			return strings.TrimSpace(r.DefaultPythonVersion)
		}
	}
	if strings.Contains(build, "-") {
		return ""
	}
	majorText, _, _ := strings.Cut(build, ".")
	major, err := strconv.Atoi(majorText)
	switch {
	case err != nil:
		return ""
	case major >= firstPython312Runtime:
		return "3.12"
	default:
		return "3.11"
	}
}

// firstPython312Runtime is the first Airflow 2 runtime major whose image runs
// Python 3.12.
const firstPython312Runtime = 12

// NewestPublishedRuntimeTagged is the newest build a floating tag serves:
// "3.3" serves 3.3-N, "12" serves 12.x.y and "12.6" serves 12.6.y. Yanked
// builds and ones dated after today are skipped, as for
// NewestPublishedRuntimeFor.
//
// A floating tag serves builds of its own scheme only. A runtime 3 tag is
// Airflow 3's, whose builds are X.Y-N; any other is an Airflow 2 runtime
// version, whose builds are semver. Airflow 2's runtimes were once numbered
// 3.x.y too, so "3.0" serves 3.0-N and never Airflow 2's runtime 3.0.1.
func (c *Catalog) NewestPublishedRuntimeTagged(floating string) (string, bool) {
	floating = strings.TrimSpace(floating)
	airflow3 := floating == "3" || strings.HasPrefix(floating, "3.")
	var best string
	for tag, r := range c.runtimes {
		if r.Yanked || r.scheduled() || strings.Contains(tag, "-") != airflow3 {
			continue
		}
		switch {
		case airflow3 && floating == "3":
			if !strings.HasPrefix(tag, "3.") {
				continue
			}
		case airflow3:
			if !strings.HasPrefix(tag, floating+"-") {
				continue
			}
		case !strings.HasPrefix(tag, floating+"."):
			continue
		}
		if best == "" || compareVersions(tag, best) > 0 {
			best = tag
		}
	}
	return best, best != ""
}
