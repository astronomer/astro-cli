package manifest

import (
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// RuntimeGroupName is the [dependency-groups] group astro writes the top-level
// packages of the project's Astro Runtime image into, and adds to [tool.uv]
// default-groups, so `uv sync`, `uv run` and standalone mode install what the
// image has. Its first comment is the seed record (SeedRecord): which build
// and Python the project's uv.lock was seeded from, which holds each of these
// packages, and what they pull in, at the image's version.
//
//	[dependency-groups]
//	astro-runtime = [
//	    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12). Run astro init to seed it again.
//	    # The packages that image installs, so the local environment has what it has. astro writes them; to choose a version yourself, list the package in [project] dependencies.
//	    "apache-airflow-providers-celery",
//	    …
//	]
const RuntimeGroupName = "astro-runtime"

// RuntimeGroupComment is the comment line that says what the runtime group
// holds, after the seed record.
const RuntimeGroupComment = "The packages that image installs, so the local environment has what it has. astro writes them; to choose a version yourself, list the package in [project] dependencies."

// RuntimeGroupEmptyComments are the comment lines of the runtime group with
// nothing in it: the project builds on no Astro Runtime that lists its
// packages, or lists every one of them itself. The group stays, so a choice
// made in default-groups stays with it.
func RuntimeGroupEmptyComments() []string {
	return []string{
		"astro writes here the packages the project's Astro Runtime image installs, so the local environment has what the image has.",
		"None apply to this project now.",
	}
}

var runtimeGroupKey = []string{"dependency-groups", RuntimeGroupName}

// runtimeGroupOf is RuntimeGroupName out of a manifest's bytes, given
// [dependency-groups] as it decoded: its names, nil when it is not there or
// lists none, and the seed record its comments hold, "" when there is none.
func runtimeGroupOf(data []byte, groups map[string]any) (names []string, build, python string) {
	list, isList := groups[RuntimeGroupName].([]any)
	if !isList {
		return nil, "", ""
	}
	for _, v := range list {
		if s, isString := v.(string); isString {
			names = append(names, s)
		}
	}
	for _, l := range tomledit.ArrayLines(data, runtimeGroupKey, list) {
		if c, isComment := l.(tomledit.Comment); isComment {
			if b, p, ok := IsSeedRecord(string(c)); ok {
				return names, b, p
			}
		}
	}
	return names, "", ""
}
