package airflowversions

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"

	semver "github.com/Masterminds/semver/v3"
)

// AirflowVersionReg represents translated python regexpr to JS from https://www.python.org/dev/peps/pep-0440/#appendix-b-parsing-version-strings-with-regular-expressions
// only fixed: error parsing regexp: invalid or unsupported Perl syntax: `(?<` via replace `?<` with `?P<`
var AirflowVersionReg = regexp.MustCompile(`v?(?:(?:(?P<epoch>[0-9]+)!)?(?P<release>[0-9]+(?:\.[0-9]+)*)(?P<pre>[-_.]?(?P<pre_l>(a|b|c|rc|alpha|beta|pre|preview))[-_.]?(?P<pre_n>[0-9]+)?)?(?P<post>(?:-(?P<post_n1>[0-9]+))|(?:[-_.]?(?P<post_l>post|rev|r)[-_.]?(?P<post_n2>[0-9]+)?))?(?P<dev>[-_.]?(?P<dev_l>dev)[-_.]?(?P<dev_n>[0-9]+)?)?)(?:\+(?P<local>[a-z0-9]+(?:[-_.][a-z0-9]+)*))?`)

// Response wraps all updates.astronomer.io response structs used for json marshaling
type Response struct {
	AvailableReleases []AirflowVersionRaw       `json:"available_releases"`
	Version           string                    `json:"version"`
	RuntimeVersions   map[string]RuntimeVersion `json:"runtimeVersions"`
	RuntimeVersionsV3 map[string]RuntimeVersion `json:"runtimeVersionsV3"`
}

type RuntimeVersion struct {
	Metadata   RuntimeVersionMetadata   `json:"metadata"`
	Migrations RuntimeVersionMigrations `json:"migrations"`
}

type RuntimeVersionMetadata struct {
	AirflowVersion string `json:"airflowVersion"`
	Channel        string `json:"channel"`
	ReleaseDate    string `json:"releaseDate"`
}

type RuntimeVersionMigrations struct {
	AirflowDatabase bool `json:"airflowDatabase"`
}

// AirflowVersionRaw represents a single airflow version.
type AirflowVersionRaw struct {
	Version     string   `json:"version"`
	Level       string   `json:"level"`
	ReleaseDate string   `json:"release_date"`
	Tags        []string `json:"tags"`
	Channel     string   `json:"channel"`
}

// AirflowVersion represents semver with extra postN1 attribute and image tags
type AirflowVersion struct {
	semver.Version
	postN1 uint64
	tags   []string
}

// NewAirflowVersion parses a given version and returns an instance of AirflowVersion or
// an error if unable to parse the version.
func NewAirflowVersion(v string, tags []string) (*AirflowVersion, error) {
	semV, err := semver.NewVersion(v)
	if err != nil {
		return nil, err
	}

	// get post_n1
	m := AirflowVersionReg.FindStringSubmatch(v)
	postN1, _ := strconv.ParseUint(m[8], 10, 64) //nolint:mnd

	av := AirflowVersion{
		*semV,
		postN1,
		tags,
	}
	return &av, nil
}

// Coerce aims to provide a very forgiving translation of a non-semver string to semver.
// Longer versions are simply truncated (2.0.1-2 becomes 2.0.0)
func (v *AirflowVersion) Coerce() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%d.%d.%d", v.Major(), v.Minor(), v.Patch())

	return buf.String()
}

// LessThan tests if one version is less than another one.
func (v *AirflowVersion) LessThan(o *AirflowVersion) bool {
	return v.Compare(o) < 0
}

// GreaterThan tests if one version is greater than another one.
func (v *AirflowVersion) GreaterThan(o *AirflowVersion) bool {
	return v.Compare(o) > 0
}

func compareSegment(v, o uint64) int {
	if v < o {
		return -1
	}
	if v > o {
		return 1
	}

	return 0
}

// Compare compares this version to another one. It returns -1, 0, or 1 if
// the version smaller, equal, or larger than the other version.
func (v *AirflowVersion) Compare(o *AirflowVersion) int {
	// Compare the major, minor, and patch version for differences. If a
	// difference is found return the comparison.
	if d := compareSegment(v.Major(), o.Major()); d != 0 {
		return d
	}
	if d := compareSegment(v.Minor(), o.Minor()); d != 0 {
		return d
	}
	if d := compareSegment(v.Patch(), o.Patch()); d != 0 {
		return d
	}

	// At this point the major, minor, and patch versions are the same.
	if v.postN1 > o.postN1 {
		return 1
	}

	if v.postN1 < o.postN1 {
		return -1
	}
	return 0
}

// AirflowVersions is a collection of AirflowVersion instances and implements the sort
// interface. See the sort package for more details.
// https://golang.org/pkg/sort/
type AirflowVersions []*AirflowVersion

// Len returns the length of a collection. The number of Version instances
// on the slice.
func (c AirflowVersions) Len() int {
	return len(c)
}

// Less is needed for the sort interface to compare two Version objects on the
// slice. If checks if one is less than the other.
func (c AirflowVersions) Less(i, j int) bool {
	return c[i].LessThan(c[j])
}

// Swap is needed for the sort interface to replace the Version objects
// at two different positions in the slice.
func (c AirflowVersions) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
