package env

import (
	"slices"
	"strings"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// InventoryItem is one row of the cross-kind listing: what an object is, what
// it is called, which scope holds it, and its ID when the query asked for one.
//
// It carries no value. That is not a redaction — it is what the four per-type
// listings have in common once their type-specific columns are removed, so a
// value has nowhere to go. The property is worth keeping deliberately: this is
// the one listing that can be handed to anyone, and --include-secrets has
// nothing to act on.
type InventoryItem struct {
	// Kind is the subcommand that manages this object — `variable`,
	// `connection`, `airflow-variable`, `metrics-export` — not the API's
	// enum. A listing that names the command you would type next is worth
	// more than one that names the wire constant.
	Kind string `json:"kind"`
	Key  string `json:"key"`
	// Scope is the platform's own word, WORKSPACE or DEPLOYMENT, because it
	// is not a command — there is nothing to type it into — and every
	// per-kind listing already prints it this way.
	Scope string `json:"scope"`
	// ID is set only when the objects were fetched without link resolution;
	// a resolved row refers to a link rather than a directly addressable
	// object, which is why the per-kind tables hide the column too.
	ID string `json:"id,omitempty"`
}

// inventoryKind pairs an object type with the noun the CLI spells it with.
//
// The two deliberately differ — ENVIRONMENT_VARIABLE is a wire value,
// `variable` is a command — and this table is the only place that knows both.
// It also fixes the order the listing groups by, which is the order `astro env
// --help` lists the nouns.
var inventoryKinds = []struct {
	noun string
	list func(Scope, bool, bool, astrov1.APIClient) ([]astrov1.EnvironmentObject, error)
}{
	{"variable", ListVars},
	{"connection", ListConns},
	{"airflow-variable", ListAirflowVars},
	{"metrics-export", ListMetricsExports},
}

// nounForListType maps the list endpoint's type filter onto the CLI's noun,
// so a failure names a command the user can type rather than a wire constant.
func nounForListType(t astrov1.ListEnvironmentObjectsParamsObjectType) string {
	return nounForObjectType(astrov1.EnvironmentObjectObjectType(t))
}

// nounForObjectType maps the API's object-type enum onto the CLI's noun, for
// rows whose type does not match the table above.
func nounForObjectType(t astrov1.EnvironmentObjectObjectType) string {
	switch t {
	case astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE:
		return "variable"
	case astrov1.EnvironmentObjectObjectTypeCONNECTION:
		return "connection"
	case astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE:
		return "airflow-variable"
	case astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT:
		return "metrics-export"
	case "":
		// A row that carried no type still belongs in the inventory, but a
		// blank KIND cell reads as a rendering fault rather than as missing
		// data, and `"kind":""` matches nothing a consumer switches on.
		return "unknown"
	default:
		// A type this CLI does not know yet is shown as the API spells it.
		// Dropping it would make the inventory quietly incomplete, which is
		// worse than an unfamiliar word.
		return string(t)
	}
}

// ListInventory returns every environment object in the scope, of every type.
//
// It asks for each type in turn rather than omitting the objectType filter and
// taking whatever comes back. The endpoint documents the parameter only as
// "the environment object type to filter for" and states no default, nothing
// in this repo has ever called it without one, and there is no spec vendored
// here to settle it. If the endpoint turned out to default to a single type,
// the unfiltered version would print a confident, complete-looking table that
// silently omitted three kinds — the failure this command exists to prevent.
// Four requests is the price of not guessing; a single call is a worthwhile
// optimization once someone confirms the behavior against a live API.
//
// includeSecrets is not a parameter: no value is rendered, so asking the
// platform to unmask one would fetch a secret with nowhere to put it.
func ListInventory(scope Scope, resolveLinked bool, astroV1Client astrov1.APIClient) ([]InventoryItem, error) {
	var items []InventoryItem
	for _, k := range inventoryKinds {
		objs, err := k.list(scope, resolveLinked, false, astroV1Client)
		if err != nil {
			return nil, err
		}
		for i := range objs {
			item := InventoryItem{
				Kind:  nounForObjectType(objs[i].ObjectType),
				Key:   objs[i].ObjectKey,
				Scope: string(objs[i].Scope),
			}
			// A resolved row's ID refers to the link, not to something the
			// caller can address, so it is reported only when resolution was
			// off — the same rule the per-kind tables use to hide the column.
			if !resolveLinked && objs[i].Id != nil {
				item.ID = *objs[i].Id
			}
			items = append(items, item)
		}
	}
	// Grouped by kind in the order `astro env` lists the nouns, then by key.
	// Server order is unspecified, so without this the kinds interleave and
	// the numbered column means something different on every run.
	slices.SortStableFunc(items, func(a, b InventoryItem) int {
		if c := inventoryKindRank(a.Kind) - inventoryKindRank(b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
	return items, nil
}

// inventoryKindRank orders known kinds as the help lists them and sorts
// anything unrecognized to the end rather than into the middle of a group.
func inventoryKindRank(noun string) int {
	for i := range inventoryKinds {
		if inventoryKinds[i].noun == noun {
			return i
		}
	}
	return len(inventoryKinds)
}
