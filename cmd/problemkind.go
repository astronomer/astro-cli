package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/config"
	astrocontext "github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astroAuth "github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// The kinds a cloud command can fail with. They sit beside cmd/local's, in the
// same vocabulary (see cliout.ProblemKind), and are composed with them below.
const (
	// KindUnauthenticated: there is no usable login — none was ever made, the
	// API refused the token (401), or one was needed and this run may not
	// start a browser login (--output json). Log in and try again.
	KindUnauthenticated cliout.ProblemKind = "unauthenticated"
	// KindForbidden: the login is good but does not carry the permission this
	// needs (403).
	KindForbidden cliout.ProblemKind = "forbidden"
	// KindNotFound: the API has no such object, or will not admit to one this
	// login can see (404).
	KindNotFound cliout.ProblemKind = "not_found"
	// KindConflict: the API refused because the object already exists or is
	// in a state the change cannot apply to (409). The Environment Manager's
	// create, update and delete document it.
	KindConflict cliout.ProblemKind = "conflict"
	// KindAPIUnavailable: the API failed on its side (5xx). Transient as far as
	// the CLI can tell; try again.
	KindAPIUnavailable cliout.ProblemKind = "api_unavailable"
)

// cloudKinds recognizes cloud failures.
//
// Every Astro API failure goes through httputil.NormalizeAPIError, which
// returns a *httputil.StatusError carrying the response status, so the status
// is the reliable handle and the API's message is left as prose. Before any
// request, the "no login" errors are sentinels: config's, from the shell tree's
// context lookup; astrosession's, from the core's; auth's ErrLoginNeeded, for a
// login a run under --output json may not start; and `astro auth token`'s,
// for a context holding no token.
//
// APC (Houston) failures are not classified: its GraphQL client reports
// errors as text, with nothing typed to assert.
var cloudKinds = cliout.Kinds{
	{Kind: KindUnauthenticated, Match: func(err error) bool {
		return errors.Is(err, config.ErrGetHomeString) ||
			errors.Is(err, astrosession.ErrLoggedOut) ||
			errors.Is(err, astroAuth.ErrLoginNeeded) ||
			errors.Is(err, errNoAuthToken) ||
			httputil.HasStatus(err, http.StatusUnauthorized)
	}},
	{Kind: KindForbidden, Match: status(http.StatusForbidden)},
	{Kind: KindNotFound, Match: status(http.StatusNotFound)},
	{Kind: KindConflict, Match: status(http.StatusConflict)},
	{Kind: KindAPIUnavailable, Match: func(err error) bool {
		var se *httputil.StatusError
		return errors.As(err, &se) && se.StatusCode >= http.StatusInternalServerError
	}},
}

func status(code int) func(error) bool {
	return func(err error) bool { return httputil.HasStatus(err, code) }
}

// problemKinds is every kind the CLI publishes, in the order they are tried.
// cmd/local's come first: they are the more specific, and an Astro Deployment's
// Airflow that does not answer (deployment_hibernating, ...) is a better
// answer than the status underneath it.
var problemKinds = append(append(cliout.Kinds{}, local.ProblemKinds...), cloudKinds...)

// Execute builds the root for this machine and runs it against the process's
// arguments under the output contract (cliout.Execute). The error it returns
// is for cliout.ExitCode; it has already been reported.
//
// A run refused as a usage error may have been a command the CLI does not
// have, which is recorded once the refusal is out (trackUnknownCommand).
func Execute(ctx context.Context) error {
	// Once, on the real CLI path only, before any command runs: under an APC
	// context, whose deploy still builds the 1.x layout, everything that
	// speaks of a 1.x project says so (project.SetUnderAPC). Not in
	// NewRootCmd, which tests build for several platforms.
	project.SetUnderAPC(!astrocontext.IsCloudContext())

	root, args := NewRootCmd(), os.Args[1:]
	err := cliout.Execute(ctx, root, args, os.Stdout, problemKinds)
	if cliout.IsUsage(err) {
		trackUnknownCommand(root, args)
	}
	return err
}
