package runtimeversions

import (
	"strconv"
	"strings"
)

// compareVersions orders two versions segment by segment, numerically, splitting
// on "." and "-" so a runtime tag ("3.3-10") orders the way its builds shipped,
// and an Airflow series ("3.10") above "3.9". A segment that is not a number
// sorts low rather than failing, so an unexpected tag never wins.
func compareVersions(a, b string) int {
	as, bs := segments(a), segments(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		an, bn := segment(as, i), segment(bs, i)
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}
	return 0
}

// CompareRuntimes orders two runtime tags of one line ("3.3-8" below
// "3.3-10", "13.9.0" below "13.11.0"): -1, 0 or 1.
func CompareRuntimes(a, b string) int {
	return compareVersions(a, b)
}

func segments(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' })
}

func segment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return -1
	}
	return n
}

// numericVersion reports a dotted version of digits only, like "3.12".
func numericVersion(v string) bool {
	if v == "" {
		return false
	}
	for _, part := range strings.Split(v, ".") {
		if part == "" {
			return false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}
