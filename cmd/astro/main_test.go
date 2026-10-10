package astro

import (
	"flag"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
)

// TestMain pins os.Args to a single non-test argument for the whole package run.
// Several tests execute cobra commands, which fall back to os.Args when no args
// are set. Left as-is, the go-test flags (or a slice a prior test mangled to
// empty) leak between tests and make cobra execution order-dependent under
// -shuffle. Restoring afterward leaves the real args for anything that runs later.
//
// The test flags are parsed first, while os.Args still holds them: m.Run parses
// only if nobody has, and from os.Args. Pinning before parsing made it read
// none, so this package used to ignore -run, -skip, -shuffle and the coverage
// flags `make test` passes. A helper process (TestForcedRenewalHelper) gets
// its -test.run the same way.
//
// It also watches what the commands publish through cliout.Renderer.Emit, and
// fails a passing run that published a shape no golden pins (schema_test.go).
func TestMain(m *testing.M) {
	flag.Parse()
	origArgs := os.Args
	os.Args = []string{"astro"}
	// A login reads its auth config from api.<domain>, which no unit test may
	// reach. The identity provider it names is under .test, which never
	// resolves, so a refresh against it fails without leaving the machine, as
	// the cases that fall back to a browser login expect. A test of the fetch,
	// or of what it answers, sets its own.
	fetchDomainAuthConfig = func(domain string) (auth.Config, error) {
		if !context.IsCloudDomain(domain) {
			return auth.FetchDomainAuthConfig(domain) // refuses it before any request
		}
		return auth.Config{ClientID: "client-id", Audience: "audience", DomainURL: "https://auth.astronomer.test/"}, nil
	}
	watching := emitWatch().Arm()
	code := m.Run()
	os.Args = origArgs

	// A helper process runs one test for its parent, which judges the run.
	if os.Getenv(renewHelperEnv) != "" {
		os.Exit(code)
	}
	os.Exit(watching.Finish(code))
}
