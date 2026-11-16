package cmd

import (
	"fmt"
	"io"
	"sync"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// manifestWarningPrinter is the manifest.SetWarningHandler every command
// runs under: each warning a loaded pyproject.toml carries (a key this build
// does not know) is printed to w, once per file and key however often the run
// loads it, in the words astro local start uses.
//
// One handler at the load rather than one at each caller, because every
// command that reads a project loads it (deploy, check, package, link, env,
// af, api), and a caller that dropped the warnings would deploy or run
// without a word about the key it ignored. stderr, so a command's --output
// json on stdout stays one parseable document.
func manifestWarningPrinter(w io.Writer) func(path string, warnings []manifest.Problem) {
	var mu sync.Mutex
	seen := map[string]bool{}
	return func(path string, warnings []manifest.Problem) {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range warnings {
			key := path + "\x00" + p.Key + "\x00" + p.Reason
			if seen[key] {
				continue
			}
			seen[key] = true
			cliout.Warn(w, fmt.Sprintf("%s: %s: %s", manifest.Marker, p.Key, p.Reason))
		}
	}
}
