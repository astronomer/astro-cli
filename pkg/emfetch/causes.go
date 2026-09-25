package emfetch

import (
	"fmt"
	"net/http"
)

// Cause is why a value declared `source = "workspace"` is missing after a start
// read the workspace: the causes docs/v2-workspace-link.md lists.
//
// The Astro CLI and Astro Desktop both report them, so the text lives here, in
// the reader both already share, and a workspace that cannot be read reads the
// same from either app. Each app keeps its own error types and decides which
// cause one of them is; only the words are shared. They are deliberately
// English: operational messages that quote commands, not UI copy.
type Cause int

const (
	// CauseNotLoggedIn is no login stored for the manifest's domain.
	CauseNotLoggedIn Cause = iota + 1
	// CauseSessionExpired is the platform answering 401, or a stored login
	// whose token could not be refreshed.
	CauseSessionExpired
	// CauseNoAccess is the platform answering 403.
	CauseNoAccess
	// CauseNotFound is the platform answering 404.
	CauseNotFound
	// CauseSecretsWithheld is a value the organization's secrets policy kept
	// back. Withheld decides which values count.
	CauseSecretsWithheld
	// CauseNoValue is an object the workspace holds with no value, or a name it
	// holds no object for.
	CauseNoValue
	// CauseOffline is a read that got no response.
	CauseOffline
	// CauseNoWorkspace is a manifest that sets no top-level `workspace` for a
	// workspace source to read. It is a missing value, not a manifest error: a
	// local value still satisfies the name, with no network needed.
	CauseNoWorkspace
)

// Text is the message for c. domain is the manifest's, already defaulted and
// normalized; every cause that involves the platform names it, because a
// production workspace asked of a dev host is the commonest wrong answer and
// the fix is the login, not the manifest. workspaceID is the manifest's
// `workspace`, which only CauseNotFound quotes.
func (c Cause) Text(domain, workspaceID string) string {
	switch c {
	case CauseNotLoggedIn:
		return fmt.Sprintf("not logged in to %s. Log in with `astro login %s`", domain, domain)
	case CauseSessionExpired:
		return fmt.Sprintf("your %s session expired. Log in again with `astro login %s`", domain, domain)
	case CauseNoAccess:
		return fmt.Sprintf("you don't have access to this workspace on %s. Check your current organization (`astro organization switch`), or ask an org admin", domain)
	case CauseNotFound:
		return fmt.Sprintf("workspace %s was not found on %s. Check `workspace` and `domain` in pyproject.toml, and your current organization", workspaceID, domain)
	case CauseSecretsWithheld:
		return "your org disables Environment Secrets Fetching. Ask an org admin to enable it, or set the value locally"
	case CauseNoValue:
		return "the workspace holds no value for it"
	case CauseOffline:
		return fmt.Sprintf("could not reach %s. Check your connection, or set the value locally", domain)
	case CauseNoWorkspace:
		return "the manifest sets no `workspace`. Add `workspace = \"<id>\"` under [tool.astro]"
	}
	return fmt.Sprintf("unknown cause %d", int(c))
}

// StatusCause is the cause a non-200 answer from the platform names, and false
// for a status the contract gives no cause of its own.
func StatusCause(status int) (Cause, bool) {
	switch status {
	case http.StatusUnauthorized:
		return CauseSessionExpired, true
	case http.StatusForbidden:
		return CauseNoAccess, true
	case http.StatusNotFound:
		return CauseNotFound, true
	}
	return 0, false
}

// StatusText is the message for a read the platform answered with a non-200
// status: the cause's text for the statuses the contract names, and otherwise
// the domain with err, the platform's own account of what went wrong.
func StatusText(status int, domain, workspaceID string, err error) string {
	if c, ok := StatusCause(status); ok {
		return c.Text(domain, workspaceID)
	}
	return fmt.Sprintf("%s returned an error: %v", domain, err)
}

// Object is what the withheld rule needs to know about one Environment Manager
// object, as a read returned it.
type Object struct {
	// Connection is a native CONNECTION object. An AIRFLOW_CONN_* environment
	// variable is not one: it is an env var, and follows the Secret rule.
	Connection bool
	// Secret is an env var or Airflow variable marked secret.
	Secret bool
	// Value is the value the read returned, empty when it returned none.
	Value string
}

// Withheld reports whether obj counts as withheld by the organization's
// secrets policy: missing with CauseSecretsWithheld, never resolved blank.
// secretsIncluded is what WithSecretsFallback returned for the read.
//
// A read that included secret values withholds nothing, so a secret it returned
// blank is one the workspace holds no value for (CauseNoValue). A read without
// them withholds every secret value, and every native connection: its password
// and the values in its extra arrive blank, and nothing in the object says which
// were blanked, so "no password" and "password withheld" cannot be told apart.
// Encoding what is left would hand Airflow a connection that starts and then
// fails to authenticate.
func Withheld(obj Object, secretsIncluded bool) bool {
	if secretsIncluded {
		return false
	}
	return obj.Connection || (obj.Secret && obj.Value == "")
}
