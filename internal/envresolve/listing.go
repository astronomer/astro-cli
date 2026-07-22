package envresolve

import (
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// ListedName is one declared name in the credential-free listing: the spec,
// plus whether a vault entry exists for it — never the value.
type ListedName struct {
	Section     envschema.Section
	Name        string
	Type        envschema.ValueType // vars and airflow_variables only
	ConnType    string              // connections only
	Required    bool
	Sensitive   bool
	Description string
	// InVault reports that a vault entry exists (project scope or global).
	InVault bool
	// VaultScope says which entry wins: VaultScopeProject,
	// VaultScopeGlobal, or "" when none exists.
	VaultScope string
}

// VaultScope labels for ListedName.
const (
	VaultScopeProject = "project"
	VaultScopeGlobal  = "global"
)

// Listing is the LLM/agent-visible projection of the project's environment.
// It is built strictly from Store.ListMeta and the schema — ListMeta reads
// cached ciphertext and is structurally incapable of returning a value, so
// no code path here can reach one. A nil store (headless machine) lists the
// schema with no vault info. Sorted by section then name.
func Listing(s *envschema.Schema, store secrets.Store, scope string) ([]ListedName, error) {
	var present map[string]string // vault key without kind prefix -> winning scope label
	if store != nil {
		metas, err := store.ListMeta()
		if err != nil {
			return nil, err
		}
		present = presence(metas, scope)
	}
	var out []ListedName
	if s != nil {
		for name, spec := range s.EnvVars {
			out = append(out, ListedName{
				Section: envschema.SectionEnvVar, Name: name,
				Type: spec.Type, Required: spec.Required, Sensitive: spec.Sensitive,
				Description: spec.Description,
				VaultScope:  present[VaultKindEnv+":"+name],
			})
		}
		for name, spec := range s.AirflowVariables {
			out = append(out, ListedName{
				Section: envschema.SectionAirflowVariable, Name: name,
				Type: spec.Type, Required: spec.Required, Sensitive: spec.Sensitive,
				Description: spec.Description,
				VaultScope:  present[VaultKindEnv+":"+airflowenv.EnvKeyForVarKey(name)],
			})
		}
		for connID, spec := range s.Connections {
			out = append(out, ListedName{
				Section: envschema.SectionConnection, Name: connID,
				ConnType: spec.ConnType, Required: spec.Required,
				Sensitive:   true, // connections always carry credentials
				Description: spec.Description,
				VaultScope:  present[VaultKindConn+":"+strings.ToLower(connID)],
			})
		}
	}
	for i := range out {
		out[i].InVault = out[i].VaultScope != ""
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// presence indexes vault metadata by "kind:name", labeling each name with
// the scope tier that wins for this project: an entry scoped to this
// project beats a global one; entries scoped to other projects are
// invisible here.
func presence(metas []secrets.Meta, scope string) map[string]string {
	out := map[string]string{}
	for _, meta := range metas {
		kind, keyScope, name, ok := ParseVaultKey(meta.Key)
		if !ok {
			continue
		}
		id := kind + ":" + name
		switch {
		case keyScope == scope && scope != "":
			out[id] = VaultScopeProject
		case keyScope == "":
			if out[id] == "" {
				out[id] = VaultScopeGlobal
			}
		}
	}
	return out
}
