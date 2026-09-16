package env

// SecretsFetchingNotAllowedErrMsg is shown when the organization disallows
// reading secret values via the env-object API.
const SecretsFetchingNotAllowedErrMsg = `environment secrets fetching is not enabled for this organization.

To resolve this issue:
• Ask an organization administrator to enable "Environment Secrets Fetching" in organization settings
• Navigate to Organization Settings > General > Environment Secrets Fetching
• Toggle the setting to "Enabled"

This setting controls whether deployments can access organization environment secrets during local development.

Without this setting enabled, local development still resolves non-secret workspace values; only secret values are withheld.`
