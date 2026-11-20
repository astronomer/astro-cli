package env

// SecretsFetchingNotAllowedErrMsg is shown when the organization disallows
// reading secret values via the env-object API.
const SecretsFetchingNotAllowedErrMsg = `environment secrets fetching is not enabled for this Organization.

To enable it, an Organization admin can turn on Environment Secrets Fetching under Organization Settings > General.

Until then, local development still resolves non-secret Workspace values and withholds secret values.`
