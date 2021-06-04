package airflowversions

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var AirflowVersionReg = regexp.MustCompile(`v?` +
	`(?:` +
	// epoch
	`(?:(?P<epoch>[0-9]+)!)?` +
	`(?P<release>[0-9]+(?:\.[0-9]+)*)` +
	`(?P<pre>                          
	[-_\.]?
	(?P<pre_l>(a|b|c|rc|alpha|beta|pre|preview))
	[-_\.]?
	(?P<pre_n>[0-9]+)?
         )?` +
	`(?P<post>
            (?:-(?P<post_n1>[0-9]+))
            |
            (?:
                [-_\.]?
                (?P<post_l>post|rev|r)
                [-_\.]?
                (?P<post_n2>[0-9]+)?
            )
        )?` +
	`(?P<dev>
            [-_\.]?
            (?P<dev_l>dev)
            [-_\.]?
            (?P<dev_n>[0-9]+)?
        )?` +
	`)` + `(?:\+(?P<local>[a-z0-9]+(?:[-_\.][a-z0-9]+)*))?`,
)

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(airflowVersion string) (string, error) {
	client := NewClient(httputil.NewHTTPClient())
	r := Request{}

	// defaultImageTag := ""
	resp, err := r.DoWithClient(client)
	fmt.Println(resp.AvailableReleases)

	vs := make([]*semver.Version, len(resp.AvailableReleases))
	for i, r := range resp.AvailableReleases {
		v, _ := semver.NewVersion(r.Version)
		vs[i] = v
	}

	sort.Sort(semver.Collection(vs))
	fmt.Println(vs)

	fmt.Println(AirflowVersionReg.MatchString("2.0.0-1"))
	return "", err
}
