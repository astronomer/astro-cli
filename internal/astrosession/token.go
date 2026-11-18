package astrosession

import (
	"errors"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
)

// This file is the token identity both trees share. A run with ASTRO_API_TOKEN
// is the token and nothing else: its credential, and the organization,
// Workspace and Deployment its claims name. The platform tree's setup applies
// the same claims to the current context for the process
// (cmd/astro/setup.go), but the core tree has no setup, so it reads them from
// the token here, and so does everything in this package that answers "who is
// calling": Bearer, BearerFor, Login, Resolve. A run with a token never reads
// the saved login, in either tree.

// The three ways ASTRO_API_TOKEN can be unusable, worded as the platform tree's
// setup words them, so a token fails with the same sentence and the same kind
// (unauthenticated) whichever tree the command is in. All three match
// ErrInvalidToken, which is what the root's kind table asks.
var (
	// ErrInvalidToken is any unusable ASTRO_API_TOKEN. The run does not fall
	// back to the saved login: the person supplied a credential, and it is the
	// one that failed.
	ErrInvalidToken = errors.New("ASTRO_API_TOKEN is unusable")
	// ErrNotAPIToken is a token with no permissions, which no Astro API token lacks.
	ErrNotAPIToken error = &invalidTokenError{"ASTRO_API_TOKEN is not an Astro API token (it has no permissions): set a valid token, or unset it to use your login"}
	// ErrAPITokenExpired is a token past its expiry.
	ErrAPITokenExpired error = &invalidTokenError{"API Token is expired"}
	// ErrAPITokenMalformed is something that is not a token at all.
	ErrAPITokenMalformed error = &invalidTokenError{"API Token is invalid or malformed"}
)

// malformedTokenError is ErrAPITokenMalformed with the parser's reason, which
// a test or a debug reader can reach but the message leaves out.
type malformedTokenError struct{ cause error }

func (e *malformedTokenError) Error() string   { return ErrAPITokenMalformed.Error() }
func (e *malformedTokenError) Unwrap() []error { return []error{ErrAPITokenMalformed, e.cause} }

// invalidTokenError carries its own sentence and matches ErrInvalidToken.
type invalidTokenError struct{ msg string }

func (e *invalidTokenError) Error() string        { return e.msg }
func (e *invalidTokenError) Is(target error) bool { return target == ErrInvalidToken }

// ErrTokenNoOrganization is a token whose claims name no organization, which
// no genuine Astro API token's lack: the run cannot look anything up without
// one, and the saved login's is not the token's.
var ErrTokenNoOrganization = errors.New("ASTRO_API_TOKEN names no organization: set a valid Astro API token, or unset it to use your login")

// Identity is what an ASTRO_API_TOKEN is: its credential, and the selection its
// claims make. A field is empty when the claims do not name one: an
// Organization token has no Workspace, and only a Deployment token has a
// Deployment.
type Identity struct {
	// Credential is the token itself, without a scheme.
	Credential string
	// Organization is the organizationId claim.
	Organization string
	// Workspace is the workspaceId claim.
	Workspace string
	// Deployment is the Deployment the token is scoped to: its one
	// deploymentId claim, empty for none or for several.
	Deployment string
	// ExpiresAt is the token's expiry, the zero time for a token with none.
	ExpiresAt time.Time
}

// TokenIdentity reads ASTRO_API_TOKEN. ok is false, with no error, when the
// variable holds no credential (see APIToken): there is no token run. A
// credential that is malformed, expired or without permissions is an error, as
// setup treats it, and never a reason to read the saved login. The token's
// signature is not checked; the API does that.
func TokenIdentity() (id Identity, ok bool, err error) {
	stored, has := APIToken()
	if !has {
		return Identity{}, false, nil
	}
	credential := Credential(stored)
	claims, err := util.ParseAPIToken(credential)
	if err != nil || claims == nil {
		if err == nil {
			err = errors.New("the token has no claims")
		}
		return Identity{}, true, &malformedTokenError{cause: err}
	}
	if len(claims.Permissions) == 0 {
		return Identity{}, true, ErrNotAPIToken
	}
	id = Identity{Credential: credential}
	if claims.ExpiresAt != nil {
		if claims.ExpiresAt.Before(time.Now()) {
			logger.Debugf("The given API Token %s has expired", claims.APITokenID)
			return Identity{}, true, ErrAPITokenExpired
		}
		id.ExpiresAt = claims.ExpiresAt.Time
	}
	deployments := 0
	workspaces := map[string]bool{}
	for _, permission := range claims.Permissions {
		kind, v, found := strings.Cut(permission, ":")
		if !found {
			continue
		}
		switch kind {
		case "organizationId":
			id.Organization = v
		case "workspaceId":
			// Whitespace around an id is not a different Workspace, and an
			// empty claim names none: neither decides which Workspace the
			// token is for.
			if v = strings.TrimSpace(v); v != "" {
				id.Workspace = v
				workspaces[v] = true
			}
		case "deploymentId":
			deployments++
			id.Deployment = v
		}
	}
	if deployments != 1 {
		id.Deployment = ""
	}
	// Several different Workspaces are no Workspace claim, as several
	// Deployments are no Deployment's: the token is not for one to act in.
	if len(workspaces) > 1 {
		id.Workspace = ""
	}
	return id, true, nil
}

// claimsOf is the token's claims for a reader that cannot report an error: a
// token that is not usable names nothing.
func claimsOf() Identity {
	id, _, err := TokenIdentity()
	if err != nil {
		return Identity{}
	}
	return id
}
