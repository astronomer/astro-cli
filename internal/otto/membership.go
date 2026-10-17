package otto

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/util"
)

// ErrNotOrgMember signals that the signed-in user cannot reach the
// organization Otto would run in. Start has already printed what to do about
// it, so launchOtto exits quietly, as for ErrNotLoggedIn.
var ErrNotOrgMember = errors.New("not a member of the organization")

// MembershipCheck asks the platform whether the signed-in user can reach org.
// It returns the user's name and true only when that is known for certain not
// to be the case: the organization refused or hid itself from a login that is
// a user. Anything else — a member, an API token, an error, a timeout — is
// false, and the launch goes ahead. internal/otto may not reach a platform
// itself, so cmd/ supplies it.
type MembershipCheck func(ctx context.Context, org string) (user string, notMember bool)

// membershipTimeout bounds the check: a slow API must not hold up the launch
// for longer than this, and a check that runs out lets Otto start.
const membershipTimeout = 5 * time.Second

// Where the organization Otto runs in came from, for the guidance.
const (
	orgFromLogin   = "login"
	orgFromProject = "project"
	orgFromEnv     = "env"
)

// effectiveOrganization is the organization BuildEnv hands Otto, and where it
// came from: the project's or the login's when there is one, else an
// ASTRO_ORGANIZATION inherited from the shell, which BuildEnv passes through
// because it sets nothing for an empty value.
func effectiveOrganization(cfg *Config) (org, source string) {
	switch {
	case cfg.Organization == "":
		return os.Getenv("ASTRO_ORGANIZATION"), orgFromEnv
	case cfg.Organization != cfg.loginOrganization:
		return cfg.Organization, orgFromProject
	default:
		return cfg.Organization, orgFromLogin
	}
}

// isAPIToken reports whether token is an Astro API token rather than a user's
// login: its claims carry the permissions it was granted, as checkAPIToken in
// cmd/astro reads them. An API token reaches an organization by those grants,
// not by membership, so there is no membership to check.
func isAPIToken(token string) bool {
	claims, err := util.ParseAPIToken(token)
	return err == nil && len(claims.Permissions) > 0
}

// verifyOrgMembership stops the launch when the signed-in user cannot reach
// the organization Otto would run in. Otto's model gateway is not
// permission-checked, so without this a mismatch (a stale ASTRO_ORGANIZATION,
// an organization chosen under another login) shows only later, as a 403 from
// the first organization-scoped call, which reads like a bug in that feature.
// A nil check, no organization, no token, or an API token skips it, and so
// does a project organization on another host than the login's: the check
// asks the login's host, which would not know it.
func verifyOrgMembership(cfg *Config, check MembershipCheck, out io.Writer) error {
	org, source := effectiveOrganization(cfg)
	if check == nil || org == "" || cfg.Token == "" || isAPIToken(cfg.Token) {
		return nil
	}
	if source == orgFromProject && manifest.NormalizeDomain(cfg.projectDomain) != manifest.NormalizeDomain(cfg.Domain) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), membershipTimeout)
	defer cancel()
	user, notMember := check(ctx, org)
	if !notMember {
		return nil
	}

	// The API refused or hid the organization. Most often the user is not a
	// member, but the organization may not exist, or may want a fresh sign-in
	// (SSO, say), so say what was seen rather than guess.
	fmt.Fprintf(out, "%s can't access organization %s, so Otto can't run in it.\n", user, org)
	fmt.Fprintln(out, "You may not be a member of it, it may not exist, or it may need you to sign in again.")
	fmt.Fprintln(out)
	switch source {
	case orgFromEnv:
		fmt.Fprintln(out, "The organization comes from ASTRO_ORGANIZATION in your environment. To fix:")
		fmt.Fprintln(out, "  • Clear it:                 unset ASTRO_ORGANIZATION")
	case orgFromProject:
		fmt.Fprintln(out, "The organization comes from this project's workspace link ([tool.astro] organization). To fix:")
		fmt.Fprintln(out, "  • Run Otto outside this project, or link a workspace in an organization you belong to")
	default:
		fmt.Fprintln(out, "The organization was selected for this login. To fix:")
		fmt.Fprintln(out, "  • Switch organization:      astro organization switch")
	}
	fmt.Fprintln(out, "  • Sign in again:            astro login")
	return ErrNotOrgMember
}
