package local

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Astro CLI 1.x and v2 share the home config, and a 1.x project carries its
// own .astro/config.yaml. These 1.x settings have no v2 setting: v2 leaves
// them in the files, where 1.x still reads them, and applies none of them.
// Someone who set one in 1.x gets a different start from v2, so a start that
// would have used one says what replaces it.
const (
	legacyAPIServerPort = "api-server.port"
	legacyWebserverPort = "webserver.port"
	legacyBuildSecrets  = "dev.build_secrets"
)

// warnLegacy1xSettings warns about each 1.x setting this start would have
// used in 1.x, naming the v2 way to get it. It goes through emitWarning like
// every other start warning, so a json consumer gets it as a warning event
// with the setting as its key.
//
// It shows on every such start: it is one line per setting, and doing what
// it says (a --port, which the project then keeps; a --build-secret or
// [tool.astro] build-secrets) both does what the setting did and silences
// it. Nothing is recorded to show it once.
func (c *cli) warnLegacy1xSettings(r cliout.Renderer, p localrt.Plan) {
	if c.d.Legacy1xSetting == nil {
		return
	}
	// A port this start was given, from --port, the port the project kept
	// from its last start, or a running Airflow a restart keeps, stands in
	// for the 1.x one.
	if p.RequestedPort <= 0 {
		key := legacyPortKey(p.AirflowVersion)
		if v, file := c.d.Legacy1xSetting(p.ProjectPath, key); v != "" && v != "0" {
			warnLegacy1x(r, file, key, portReason(v))
		}
	}
	// 1.x handed it to the image build. In v2 only a project's own
	// Dockerfile takes build secrets (a generated build refuses them), so
	// that is the one start the replacement fits; any build secret this start
	// has, from the flag, BUILD_SECRET_INPUT or the manifest, replaces it.
	if p.Mode != localrt.ModeDocker || p.Dockerfile == "" || len(p.BuildSecrets) > 0 {
		return
	}
	if v, file := c.d.Legacy1xSetting(p.ProjectPath, legacyBuildSecrets); v != "" {
		warnLegacy1x(r, file, legacyBuildSecrets, "set, but it is an Astro CLI 1.x setting v2 does not apply. "+
			"Pass --build-secret, or declare build-secrets under [tool.astro] in pyproject.toml")
	}
}

// legacyPortKey is the setting 1.x read the Airflow port from: api-server.port
// for Airflow 3, webserver.port for Airflow 2.
func legacyPortKey(airflowVersion string) string {
	if major, _, _ := strings.Cut(airflowVersion, "."); major == "2" {
		return legacyWebserverPort
	}
	return legacyAPIServerPort
}

// portReason names --port with the 1.x value, when that value is a port.
func portReason(v string) string {
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 65535 {
		return fmt.Sprintf("set to %s, but it is an Astro CLI 1.x setting v2 does not read. To start on %s, run astro local start --port %s", v, v, v)
	}
	return fmt.Sprintf("set to %q, which is not a valid port, and it is an Astro CLI 1.x setting v2 does not read. "+
		"Pick the port with astro local start --port <port>", v)
}

func warnLegacy1x(r cliout.Renderer, file, key, reason string) {
	emitWarning(r, event{
		Event:  "warning",
		Text:   fmt.Sprintf("%s: %s: %s", file, key, reason),
		Key:    key,
		Reason: reason,
	})
}
