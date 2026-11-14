package utils

// ShortOptions is the one table of short options: a letter names exactly one
// flag, in every tree (Astro, hosted Astro, APC, the root, astro local, af and
// api), and a flag takes its letter only under the long name listed here. No
// other flag has a short option. TestShortOptionsFollowTheTable holds every
// flag, visible or hidden, to it, and docs/architecture.md prints the same
// table (TestShortOptionsTableIsDocumented).
//
// The letters keep their reach: a command that has --deployment, --name,
// --workspace and the others takes the letter too, except where Astro CLI 1.x
// gave the letter to a different flag on that command (astro deploy's -d was
// --dags and its -w was --wait), so that an old script is refused instead of
// read as something else (TestTableLettersReachEveryFlagThatCanTakeThem).
var ShortOptions = map[string]string{
	"o": "output",
	"y": "yes",
	"d": "deployment",
	"n": "name",
	"f": "follow",
	"a": "all",
	"h": "help",
	"w": "workspace",
}

// The API passthrough commands are exempt from the table above. They mirror
// tools whose users and agent skills already type their letters (gh api's
// -X, -H, -F, -f and -i, which Vercel's own `vercel api` keeps too, and the
// standalone af CLI's), so they keep every letter they had in 1.x and in
// earlier v2 builds, with the meaning they had there. Each exempt family is a
// table of its own, and a command is in it by its path below the root, so a
// new command under one of the paths takes the exemption by itself and cannot
// drift from it.
//
// A letter means one thing within a family. The two families disagree on -t
// (--template on astro api, --tags on astro af) and -p (--path-param on
// astro api, --poll-interval on astro af), as gh api and af always did.
// ShortOptions still applies on an exempt command wherever the family has no
// letter of its own, so -d, -o, -h and the rest mean what they mean everywhere;
// where the family has the letter, it wins: -f is gh api's raw field on
// astro api, not --follow.

// APIShortOptions is the family of astro api: its airflow, cloud and registry
// requests, and their ls and describe.
var APIShortOptions = map[string]string{
	"X": "method",
	"F": "field",
	"f": "raw-field",
	"H": "header",
	"i": "include",
	"q": "jq",
	"p": "path-param",
	"t": "template",
	"v": "verbose",
	"u": "username",
	"O": "organization-id",
	"W": "workspace-id",
}

// AfShortOptions is the family of astro af and astro local af, the queries
// af's skills type, and of astro local api, which takes af api's request
// flags.
var AfShortOptions = map[string]string{
	"X": "method",
	"F": "field",
	"H": "header",
	"i": "include",
	"v": "verbose",
	"c": "conf",
	"l": "limit",
	"m": "map-index",
	"p": "poll-interval",
	"r": "run-id",
	"s": "state",
	"t": "tags",
}

// PassthroughPaths maps each exempt command path below the root, and with it
// every command under it, to its family's letters.
var PassthroughPaths = map[string]map[string]string{
	"api":       APIShortOptions,
	"af":        AfShortOptions,
	"local af":  AfShortOptions,
	"local api": AfShortOptions,
}
