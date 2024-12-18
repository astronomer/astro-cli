package houston

import (
	"sort"

	"github.com/astronomer/astro-cli/pkg/logger"
	"golang.org/x/mod/semver"
)

// queryByVersion - defines properties needed to maintain differences in a query overtime introduced by Houston API
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
	for idx := len(s) - 1; idx >= 0; idx-- {
		idxVersion := sanitiseVersionString(s[idx].version)
		cmp := semver.Compare(v, idxVersion)
		if cmp >= 0 {
			return s[idx].query
		}
	}
	logger.Debugf("GraphQL query not defined for the given Platform version: %s, falling back to latest query", v)
	return s[len(s)-1].query
}
