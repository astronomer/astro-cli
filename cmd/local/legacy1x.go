package local

import (
	"fmt"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Astro CLI 1.x and v2 share the home config. These 1.x settings have no v2
// setting: v2 leaves them in the file, where 1.x still reads them, and applies
// none of them. Someone who set one in 1.x gets a different start from v2, so
// a start that would have used one names what replaces it.
const (
	legacyAPIServerPort = "api-server.port"
	legacyWebserverPort = "webserver.port"
	legacyBuildSecrets  = "dev.build_secrets"
)

// legacyOverrides is what this start was given that stands in for a 1.x
// setting, so a hint about a setting the start overrides is not shown.
type legacyOverrides struct {
	// port is true when the start keeps a port of its own choosing: a --port,
	// or a restart of an Airflow that is running, which keeps its port.
	port bool
}

// hintLegacy1xSettings prints a note on stderr for each 1.x setting the home
// config holds a value of its own for that this start would have used in
// 1.x, naming the v2 way to get it. stderr, like the port notice, so stdout
// stays the start's events in either output mode.
//
// It shows on every such start: it is one line per setting, and the start's
// own flag (or, for build secrets, [tool.astro] build-secrets) both does
// what the setting did and silences it. Nothing is recorded to show it once.
func (c *cli) hintLegacy1xSettings(p localrt.Plan, o legacyOverrides) {
	if c.d.Legacy1xSetting == nil {
		return
	}
	if !o.port {
		// 1.x read api-server.port for Airflow 3 and webserver.port for
		// Airflow 2; a file may hold both, and one note is enough.
		for _, key := range []string{legacyAPIServerPort, legacyWebserverPort} {
			if v, file := c.d.Legacy1xSetting(key); v != "" {
				fmt.Fprintf(c.d.Stderr, "note: %s is %s in %s, an Astro CLI 1.x setting v2 does not read. To start on %s, run astro local start --port %s\n",
					key, v, file, v, v)
				break
			}
		}
	}
	// 1.x handed it to the image build. In v2 only a project's own
	// Dockerfile takes build secrets (a generated build refuses them), so
	// that is the one start the replacement fits; any build secret this start
	// has, from the flag, BUILD_SECRET_INPUT or the manifest, replaces it.
	if p.Mode != localrt.ModeDocker || p.Dockerfile == "" || len(p.BuildSecrets) > 0 {
		return
	}
	if v, file := c.d.Legacy1xSetting(legacyBuildSecrets); v != "" {
		fmt.Fprintf(c.d.Stderr, "note: %s is set in %s, an Astro CLI 1.x setting v2 does not apply. "+
			"Pass --build-secret, or declare build-secrets under [tool.astro] in pyproject.toml\n",
			legacyBuildSecrets, file)
	}
}
