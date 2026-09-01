package env

import (
	"errors"
	"strings"
)

// SecretsFetchingNotAllowedErrMsg is shown when the organization disallows
// reading secret values via the env-object API.
const SecretsFetchingNotAllowedErrMsg = `environment secrets fetching is not enabled for this organization.

To resolve this issue:
• Ask an organization administrator to enable "Environment Secrets Fetching" in organization settings
• Navigate to Organization Settings > General > Environment Secrets Fetching
• Toggle the setting to "Enabled"

This setting controls whether deployments can access organization environment secrets during local development.

Without this setting enabled, you can still use 'astro dev start' without the --deployment-id flag for local development.`

// IsSecretsFetchingNotAllowedError reports whether err originated from the
// platform refusing showSecrets at the organization level. The check walks
// the wrapped-error chain so callers can wrap the API error in additional
// context without breaking detection.
func IsSecretsFetchingNotAllowedError(err error) bool {
	if err == nil {
		return false
	}
	for currentErr := err; currentErr != nil; currentErr = errors.Unwrap(currentErr) {
		errStr := strings.ToLower(currentErr.Error())
		if strings.Contains(errStr, "showsecrets") &&
			strings.Contains(errStr, "organization") &&
			strings.Contains(errStr, "not allowed") {
			return true
		}
	}
	return false
}
