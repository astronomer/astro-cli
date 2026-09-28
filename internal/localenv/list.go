package localenv

import (
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// ListItem is one row of `astro local env list`: a declared name with the
// source it resolves from, an undeclared entry found in a file (an orphan), or
// an undeclared Airflow setting, which is no orphan (see isAirflowSetting). It
// never carries a value — the row is built from names and sources only, so
// `list` is structurally value-free and safe for agent surfaces.
type ListItem struct {
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	// Required is true for a declared name the start gate needs: every
	// declaration not marked optional. Sensitive is the declaration's
	// sensitive flag, which is always true for a declared connection. Both
	// describe the declaration, so an orphan, which has none, carries false.
	Required  bool `json:"required"`
	Sensitive bool `json:"sensitive"`
	// Description is the declaration's prose for whoever supplies the value,
	// and empty when it has none or the row is an orphan.
	Description string `json:"description,omitempty"`
	// Source is where the value resolves from: shell, project, global,
	// workspace, default, or absent (for a declared name with no value
	// anywhere).
	Source string `json:"source"`
	// Orphan marks an entry present in a file that no schema declares.
	Orphan bool `json:"orphan,omitempty"`
	// Project is the project an --all orphan came from, when it is not the
	// current one. Empty for the current project and the global file.
	Project string `json:"project,omitempty"`
	// RemoveHint is the exact command to remove an orphan.
	RemoveHint string `json:"remove_hint,omitempty"`
	// Applied is false for a global value the current project does not
	// declare: a start passes the global tiers through for declared names
	// only, so that value never reaches this project. Omitted on every other
	// row, and outside a project, where there is nothing to apply it to.
	Applied *bool `json:"applied,omitempty"`
	// DeclareHint is the exact command to declare the name in the current
	// project: for an orphan, so the value it names is one the project expects,
	// and for a row Applied marks false, so the global value reaches it. Empty
	// outside a project and for an --all orphan from another project.
	DeclareHint string `json:"declare_hint,omitempty"`
}

// ListOptions selects which files list reports over.
type ListOptions struct {
	// Scope, when set, restricts the read to one file: the project .env
	// (ScopeProject) or the global file (ScopeGlobal). Empty resolves the
	// whole chain.
	Scope Scope
	// All widens the view to the global file plus every project's .env the
	// CLI knows about, beyond the current project.
	All bool
	// WorkspaceProvider resolves names declared source = "workspace" against
	// the workspace's Environment Manager objects, so `list` can label their
	// source "workspace" (or its unavailable variant). nil leaves such a name
	// unresolved. Built presence-only (no secret values pulled).
	WorkspaceProvider envresolve.Provider
	// VaultProviders are the encrypted tier's providers, in order
	// (internal/vaultenv). Passed in for the same reason WorkspaceProvider is:
	// this package touches no keyring, and the caller is the composition root.
	// nil lists without the vault.
	//
	// A listing does read values here, unlike the Environment Manager's
	// presence-only fetch: resolving is what tells a project secret from a
	// global one, the value never reaches a row, and a keyring that will not
	// open is reported as the source's unavailable label rather than failing
	// the listing.
	VaultProviders []envresolve.Provider
	// VaultTiers are the names each vault tier holds, so an undeclared one is
	// listed as an orphan the way an undeclared file entry is. A connection or
	// Airflow variable is stored in the vault by default, so without this a
	// value just set would be missing from the listing. Only the unnarrowed
	// view reads them, for the reason listProviders gives. nil lists no vault
	// orphans.
	VaultTiers []VaultTier
}

// VaultTier is what one tier of the vault holds, with the label the resolution
// chain gives it and the scope a delete names.
type VaultTier struct {
	Label   string
	Scope   Scope
	Entries []VaultEntry
}

// VaultEntry is one name a vault tier holds. The name is the one stored, not
// one recovered from EnvKey: that encoding upper-cases, so the Airflow
// variables "my_var" and "MY_VAR" share an env key, and a delete hint built
// from it could name neither.
type VaultEntry struct {
	Kind   Kind
	Name   string
	EnvKey string
	// Unlinked marks a global the vault holds that does not resolve for this
	// checkout: its link state names other projects, or the link index could
	// not be read. It is listed so the vault's contents are not hidden, but no
	// provider returns it here.
	//
	// TODO(vault-links): show it as "not linked here" in list output with the
	// link verbs; until then it lists as an ordinary orphan.
	Unlinked bool
}

// List builds the resolver-backed listing: every schema-declared name with
// its resolved source and required/sensitive flags, plus orphan entries. It
// never reads a value into a row.
func List(environ []string, projectDir string, schema *envschema.Schema, opts ListOptions) ([]ListItem, error) {
	src, err := LoadSources(environ, projectDir)
	if err != nil {
		return nil, err
	}
	providers := listProviders(src, opts.Scope, opts.VaultProviders)
	res, err := envresolve.Resolve(envresolve.Inputs{
		Schema:            schema,
		Providers:         providers,
		WorkspaceProvider: opts.WorkspaceProvider,
	})
	if err != nil {
		return nil, err
	}

	var items []ListItem
	for _, rn := range res.Resolved {
		items = append(items, declaredItem(rn, schema))
	}
	items = append(items, orphans(src, schema, opts, projectDir)...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Project < items[j].Project
	})
	return items, nil
}

// listProviders picks the chain a list reads over. A scope flag narrows it to
// one FILE so `list --project` shows exactly what that file holds; otherwise
// the full chain reports the true winning source.
//
// The vault is deliberately absent from the narrowed chains: --project and
// --global name the two dotenv files, which is what those flags have always
// meant, and folding an encrypted tier into "what this file holds" would make
// the answer untrue. A vault entry shows up in the default listing, labeled
// with the tier that held it.
func listProviders(src Sources, scope Scope, vault []envresolve.Provider) []envresolve.Provider {
	switch scope {
	case ScopeProject:
		return []envresolve.Provider{mapProvider{label: SourceProject, vals: src.project}}
	case ScopeGlobal:
		return []envresolve.Provider{mapProvider{label: SourceGlobal, vals: src.global}}
	default:
		return src.Providers(vault)
	}
}

func declaredItem(rn envresolve.ResolvedName, schema *envschema.Schema) ListItem {
	spec := specFor(schema, rn.Section, rn.Name)
	item := ListItem{
		Name:        rn.Name,
		Source:      SourceAbsent,
		Required:    !spec.Optional,
		Sensitive:   spec.Sensitive,
		Description: spec.Description,
	}
	// A resolved name carries its winning source; a workspace-source name that
	// did not resolve still carries the "workspace" label (with any
	// "unavailable" reason), so the row says where it was meant to come from.
	// Only a value absent from every source has no label — shown as "absent".
	if rn.Source != "" {
		item.Source = rn.Source
	}
	item.Kind = KindForSection(rn.Section)
	return item
}

// specFor returns the declaration behind a resolved name. The resolver only
// reports names it found in schema, so the zero spec is reached only by a
// section this function does not know.
func specFor(schema *envschema.Schema, section envschema.Section, name string) envschema.ValueSpec {
	if schema == nil {
		return envschema.ValueSpec{}
	}
	switch section {
	case envschema.SectionEnvVar:
		return schema.EnvVars[name]
	case envschema.SectionAirflowVariable:
		return schema.AirflowVariables[name]
	case envschema.SectionConnection:
		return schema.Connections[name]
	default:
		return envschema.ValueSpec{}
	}
}

// KindForSection maps a manifest schema section onto the kind it declares.
// An unrecognized section yields the zero Kind, which is what the switch it
// replaced left behind and what the callers already tolerate.
func KindForSection(s envschema.Section) Kind {
	switch s {
	case envschema.SectionEnvVar:
		return KindEnv
	case envschema.SectionAirflowVariable:
		return KindVar
	case envschema.SectionConnection:
		return KindConn
	default:
		return ""
	}
}

// orphans lists file entries that no schema declares. The files searched
// follow the same scope/--all selection as the declared view. projectDir is
// the current project ("" outside one), so --all can skip re-listing it.
func orphans(src Sources, schema *envschema.Schema, opts ListOptions, projectDir string) []ListItem {
	declared := declaredKeySet(schema)
	inProject := projectCopies(src, opts.VaultTiers)
	var out []ListItem
	add := func(files map[string]string, scope Scope) {
		for key := range files {
			if declared[key] {
				continue
			}
			item := orphanItem(key, scope, "", src.hasProject)
			if isAirflowSetting(key) {
				item = ListItem{Kind: KindEnv, Name: key, Source: string(scope)}
			}
			markIfNotApplied(&item, scope, key, inProject, DeclareHint(item.Kind, item.Name))
			out = append(out, item)
		}
	}
	switch {
	case opts.Scope == ScopeProject:
		add(src.project, ScopeProject)
	case opts.Scope == ScopeGlobal:
		add(src.global, ScopeGlobal)
	default:
		if src.hasProject {
			add(src.project, ScopeProject)
		}
		add(src.global, ScopeGlobal)
		for _, tier := range opts.VaultTiers {
			for _, e := range tier.Entries {
				if declared[e.EnvKey] {
					continue
				}
				item := vaultOrphanItem(e.Kind, e.Name, tier, src.hasProject)
				if isAirflowSetting(e.EnvKey) {
					item = ListItem{Kind: e.Kind, Name: e.Name, Source: tier.Label}
				}
				markIfNotApplied(&item, tier.Scope, e.EnvKey, inProject, vaultDeclareHint(e.Kind, e.Name))
				out = append(out, item)
			}
		}
	}
	if opts.All {
		out = append(out, crossProjectOrphans(declared, projectDir)...)
	}
	return out
}

// crossProjectOrphans reads every other project's .env the CLI knows about
// (from the runtime state records) and lists its entries. The current
// project's file is already covered by orphans above, so it is skipped by
// canonical path; a project whose file is missing simply contributes nothing.
func crossProjectOrphans(declared map[string]bool, projectDir string) []ListItem {
	recs, err := localrt.RecordedList()
	if err != nil {
		return nil // best-effort: --all still shows the current project and global
	}
	current := ""
	if projectDir != "" {
		current, _ = localrt.CanonicalPath(projectDir) //nolint:errcheck // a canonicalize failure just means we don't dedupe against it
	}
	var out []ListItem
	for i := range recs {
		rec := &recs[i]
		if current != "" {
			if canon, cerr := localrt.CanonicalPath(rec.ProjectPath); cerr == nil && canon == current {
				continue // the current project's file is already listed above
			}
		}
		m, err := readMap(ProjectEnvPath(rec.ProjectPath))
		if err != nil || len(m) == 0 {
			continue
		}
		for key := range m {
			// Skip keys the current project declares — those are already rows.
			if declared[key] || isAirflowSetting(key) {
				continue
			}
			out = append(out, orphanItem(key, ScopeProject, rec.ProjectPath, false))
		}
	}
	return out
}

// isAirflowSetting reports a key Airflow reads as a configuration option,
// AIRFLOW__{SECTION}__{KEY}. Such a key sets how Airflow runs rather than a
// value the project's code expects, so an undeclared one is listed with its
// source but never as an orphan. A global one still needs a declaration to reach
// a project, so it can carry the not-applied mark.
func isAirflowSetting(key string) bool {
	return strings.HasPrefix(key, "AIRFLOW__")
}

// vaultOrphanItem is an undeclared value held only in the vault tier.
func vaultOrphanItem(kind Kind, name string, tier VaultTier, inProject bool) ListItem {
	item := ListItem{
		Kind: kind, Name: name, Source: tier.Label, Orphan: true,
		RemoveHint: removeHint(kind, name, tier.Scope) + " --secret",
	}
	if inProject {
		item.DeclareHint = vaultDeclareHint(kind, name)
	}
	return item
}

// vaultDeclareHint declares a vault-held variable sensitive, so a later set
// leaves it in the vault; a connection is always sensitive.
func vaultDeclareHint(kind Kind, name string) string {
	if kind == KindConn {
		return DeclareHint(kind, name)
	}
	return DeclareHint(kind, name) + " --sensitive"
}

func orphanItem(key string, scope Scope, project string, inProject bool) ListItem {
	kind, name := kindFromKey(key)
	item := ListItem{Kind: kind, Name: name, Source: string(scope), Orphan: true, Project: project}
	// A cross-project orphan (project set) lives in another project's file; the
	// delete command runs against the cwd's project, so a hint would point at
	// the wrong file. Only offer it for the current project and the global file.
	if project == "" {
		item.RemoveHint = removeHint(kind, name, scope)
		if inProject {
			item.DeclareHint = DeclareHint(kind, name)
		}
	}
	return item
}

// markIfNotApplied marks an undeclared global row that a start leaves out of
// the current project. Outside a project nothing is left out. A copy in the
// project .env or the project vault reaches Airflow undeclared, so declaring
// the name would not bring the global value in; inProject holds those keys.
// hint is the command that declares the name.
func markIfNotApplied(item *ListItem, scope Scope, key string, inProject map[string]bool, hint string) {
	if scope != ScopeGlobal || inProject == nil || inProject[key] {
		return
	}
	applied := false
	item.Applied = &applied
	item.DeclareHint = hint
}

// projectCopies is the set of keys the current project holds itself, in its
// .env or its vault tier. It is nil outside a project.
func projectCopies(src Sources, tiers []VaultTier) map[string]bool {
	if !src.hasProject {
		return nil
	}
	keys := make(map[string]bool, len(src.project))
	for key := range src.project {
		keys[key] = true
	}
	for _, tier := range tiers {
		if tier.Scope != ScopeProject {
			continue
		}
		for _, e := range tier.Entries {
			keys[e.EnvKey] = true
		}
	}
	return keys
}

// kindFromKey infers what an undeclared file entry is from its env-var name.
//
// The name is all this has, so it asks only about the key. It used to reach the
// connection case through DecodeConnEnv with a placeholder "{}" value, which
// answered correctly but for the wrong reason: it tied "is this key a
// connection" to whatever the value decoder accepts, so a decoder that grew any
// requirement about the value would silently reclassify every connection here
// as a plain env var.
func kindFromKey(key string) (kind Kind, name string) {
	if v, _, ok := airflowenv.DecodeVarEnv(key, ""); ok {
		return KindVar, v
	}
	if airflowenv.IsConnEnvKey(key) {
		return KindConn, airflowenv.ConnIDForEnvKey(key)
	}
	return KindEnv, key
}

func declaredKeySet(schema *envschema.Schema) map[string]bool {
	set := map[string]bool{}
	for _, k := range envschema.DeclaredEnvKeys(schema) {
		set[k] = true
	}
	return set
}

// removeHint is the exact `astro local env <noun> delete` command for an
// orphan, with the scope flag that names the file it lives in.
func removeHint(kind Kind, name string, scope Scope) string {
	return "astro local env " + Noun(kind) + " delete " + name + " --" + string(scope)
}

// DeclareHint is the exact `astro local env <noun> declare` command for a
// name. A declaration lives in the project's pyproject.toml whichever file
// holds the value, so it takes no scope flag.
func DeclareHint(kind Kind, name string) string {
	return "astro local env " + Noun(kind) + " declare " + name
}
