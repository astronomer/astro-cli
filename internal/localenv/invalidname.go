package localenv

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// An Airflow Variable key is held to the env-var rule on its own
// (airflowenv.ValidVarKey), and older builds were not: they stored a key with
// a leading digit, like 1st_region, which is still a legal AIRFLOW_VAR_1ST_REGION.
// Such an entry no longer resolves, so it is surfaced rather than dropped, and
// it can still be deleted by the key it was stored under.

// InvalidStoredReason says why a stored entry no longer resolves, for a
// listing or a diagnosis, or is empty when the name is fine or is not one this
// explains.
func InvalidStoredReason(kind Kind, name string) string {
	if kind != KindVar || airflowenv.ValidVarKey(name) {
		return ""
	}
	if _, ok := StoredVarEnvKey(name); !ok {
		return ""
	}
	return fmt.Sprintf("not a valid Airflow Variable key (%s); rename or delete it", airflowenv.VarKeyRule)
}

// StoredVarEnvKey is the AIRFLOW_VAR_* key an Airflow Variable stored by an
// older build sits under when its key is no longer valid: the forward encode
// without the key's own rule. ok is false when even that is not an env var.
func StoredVarEnvKey(name string) (string, bool) {
	if name == "" || strings.Contains(name, ":") {
		return "", false
	}
	key := airflowenv.EnvKeyForVarKey(name)
	return key, airflowenv.ValidEnvKey(key)
}
