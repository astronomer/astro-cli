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
// read as something else. ShortOptionExceptions lists those.
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
