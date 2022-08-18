package houston

import (
	"sort"

	"golang.org/x/mod/semver"
)

type queryByVersion struct {
	version string
	query   string
}

type queryList []queryByVersion

func (s queryList) Len() int {
	return len(s)
}

func (s queryList) Less(i, j int) bool {
	iVersion := sanitiseVersionString(s[i].version)
	jVersion := sanitiseVersionString(s[j].version)
	return semver.Compare(iVersion, jVersion) < 0
}

func (s queryList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s queryList) GreatestLowerBound(v string) string {
	if !sort.IsSorted(s) {
		sort.Sort(s)
	}

	v = sanitiseVersionString(v)
	var prevQuery string
	for idx := range s {
		idxVersion := sanitiseVersionString(s[idx].version)
		cmp := semver.Compare(v, idxVersion)
		if cmp == 0 {
			return s[idx].query
		} else if cmp > 0 {
			if prevQuery != "" {
				return prevQuery
			} else {
				return s[idx].query
			}
		}
		prevQuery = s[idx].query
	}
	return s[len(s)-1].query
}
