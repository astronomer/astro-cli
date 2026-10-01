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
	// Invalid says why a stored entry no longer resolves, a Variable key an
	// older build accepted, and that it should be renamed or deleted.
	// RemoveHint still removes it.
	Invalid string `json:"invalid,omitempty"`
	// RemoveHint is the exact command to remove an orphan.
	RemoveHint string `json:"remove_hint,omitempty"`
	// Applied is true for an undeclared value a start passes to the current
	// project: everything that reaches a project is applied, declared or not.
	// Omitted on declared rows, on a global row the project's own copy
	// shadows, on one not linked here, and outside a project, where there is
	// nothing to apply it to.
	Applied *bool `json:"applied,omitempty"`
	// NotLinkedHere marks a global the vault holds that does not reach this
	// checkout: its link state names other projects, or the link index cannot
	// be used (LinksDown then says why). LinkHint is the command that links it
	// here, empty when the index is the cause.
	NotLinkedHere bool   `json:"not_linked_here,omitempty"`
	LinksDown     string `json:"links_down,omitempty"`
	LinkHint      string `json:"link_hint,omitempty"`
	// DeclareHint is the exact command to declare the name in the current
	// project: for an orphan, so the value it names becomes a requirement the
	// project states. Empty outside a project and for an --all orphan from
	// another project.
	DeclareHint string `json:"declare_hint,omitempty"`
	// SetHint and UndeclareHint are the two ways to settle a declared name no
	// source supplies (not Resolved): the command that sets a value, and the
	// one that removes the declaration. Empty on every other row.
	SetHint       string `json:"set_hint,omitempty"`
	UndeclareHint string `json:"undeclare_hint,omitempty"`
	// Resolved is true for a declared row something supplies. Source alone
	// cannot say so: a workspace-sourced name the workspace did not supply
	// still carries the workspace label.
	Resolved bool `json:"-"`
}

// ListOptions selects which files list reports over.
type ListOptions struct {
	// Scope, when set, restricts the read to one file: the project .env
	// (ScopeProject) or the global file (ScopeGlobal). Empty resolves the
	// whole chain.
	Scope Scope
	// All widens the view to the global file plus every project's .env the
	// CLI knows about, beyond the current project, and to the undeclared
	// globals in the vault that do not reach this project, marked
	// NotLinkedHere. Without it those are left out.
	All bool
	// WorkspaceProvider is the linked workspace's Environment Manager objects,
	// the tier below ~/.astro/env and above declaration defaults: a declared
	// name nothing local holds resolves from it, and what it holds undeclared
	// is listed the way an undeclared file entry is. nil leaves the tier out.
	// Built presence-only (no secret values pulled).
	WorkspaceProvider envresolve.Provider
	// Workspace is the manifest's workspace id, which a row the workspace
	// supplies names as its source: "workspace (<id>)". Empty when the
	// manifest links none, which also leaves out the undeclared rows.
	Workspace string
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
	// LinksDown is set when the link index cannot be used, so every entry of
	// this tier is Unlinked for that reason rather than for where it is
	// linked: a short reason such as "link index unreadable".
	LinksDown string
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
	// provider returns it here. The listing marks it NotLinkedHere.
	Unlinked bool
	// Invalid says why the entry does not resolve at all: a name the key
	// rule now refuses (InvalidStoredReason). Its EnvKey is the key it was
	// stored under.
	Invalid string
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
	unlinked := unlinkedGlobals(opts)
	for _, rn := range res.Resolved {
		item := declaredItem(rn, schema)
		if item.Source == string(envschema.SourceWorkspace) && opts.Workspace != "" {
			item.Source = WorkspaceSource(opts.Workspace)
		}
		// Without --all a global that does not reach this project is as good
		// as absent, so the declared row reads exactly as if none existed.
		if item.Source == SourceAbsent && opts.All {
			if e, ok := unlinked[envKeyOfItem(item)]; ok {
				markNotLinked(&item, e.entry, e.tier)
			}
		}
		// A declared name nothing supplies has two ways out: a value, or no
		// declaration. A row not linked here has its own way out, the link.
		// Keyed on Resolved, not the absent label: a workspace-sourced name the
		// workspace did not supply keeps the workspace label and is no less
		// unsupplied.
		if !item.Resolved && !item.NotLinkedHere {
			item.SetHint = SetHint(item.Kind, item.Name)
			item.UndeclareHint = UndeclareHint(item.Kind, item.Name)
		}
		items = append(items, item)
	}
	items = append(items, orphans(src, schema, opts, projectDir)...)
	items = append(items, workspaceOrphans(src, schema, opts)...)
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

// WorkspaceSource is the source a row the linked workspace supplies carries.
func WorkspaceSource(workspace string) string {
	return string(envschema.SourceWorkspace) + " (" + workspace + ")"
}

// workspaceOrphans lists what the linked workspace holds that the schema does
// not declare. A start passes each to Airflow unless a local source holds the
// same name, so the row is marked applied only then; a row a local source
// shadows reads as undeclared, the way a global the project's own copy
// shadows does. There is nothing to remove locally, so no row carries a
// remove hint. A narrowed listing names one file, which the workspace is not,
// so --project and --global leave these out. A workspace that could not be
// read lists nothing; the caller prints why (envresolve.Outage).
func workspaceOrphans(src Sources, schema *envschema.Schema, opts ListOptions) []ListItem {
	if opts.WorkspaceProvider == nil || opts.Workspace == "" || opts.Scope != "" {
		return nil
	}
	declared := declaredKeySet(schema)
	local := map[string]bool{}
	for _, files := range []map[string]string{src.shell, src.project, src.global} {
		for key := range files {
			local[key] = true
		}
	}
	for _, tier := range opts.VaultTiers {
		for _, e := range tier.Entries {
			if !e.Unlinked && e.Invalid == "" {
				local[e.EnvKey] = true
			}
		}
	}
	var out []ListItem
	for _, key := range envresolve.Keys(opts.WorkspaceProvider) {
		if declared[key] {
			continue
		}
		kind, name := kindFromKey(key)
		item := ListItem{Kind: kind, Name: name, Source: WorkspaceSource(opts.Workspace), Orphan: true}
		if isAirflowSetting(key) {
			item.Orphan = false
			out = append(out, item)
			continue
		}
		if src.hasProject {
			item.DeclareHint = DeclareHint(kind, name)
			if !local[key] {
				applied := true
				item.Applied = &applied
			}
		}
		out = append(out, item)
	}
	return out
}

type unlinkedGlobal struct {
	entry VaultEntry
	tier  VaultTier
}

// unlinkedGlobals is the vault's globals that do not reach this checkout, by
// env key, so under --all a declared name that resolves nowhere can say it is
// held but not linked here. Only the unnarrowed view reads the vault.
func unlinkedGlobals(opts ListOptions) map[string]unlinkedGlobal {
	if opts.Scope != "" {
		return nil
	}
	out := map[string]unlinkedGlobal{}
	for _, tier := range opts.VaultTiers {
		for _, e := range tier.Entries {
			if e.Unlinked {
				out[e.EnvKey] = unlinkedGlobal{entry: e, tier: tier}
			}
		}
	}
	return out
}

// envKeyOfItem is the env key a declared row resolves under.
func envKeyOfItem(item ListItem) string {
	key, _ := EnvKeyFor(item.Kind, item.Name)
	return key
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
		Resolved:    rn.Found,
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
	globalVault := map[string]bool{}
	for _, tier := range opts.VaultTiers {
		if tier.Scope != ScopeGlobal {
			continue
		}
		for _, e := range tier.Entries {
			if !e.Unlinked {
				globalVault[e.EnvKey] = true
			}
		}
	}
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
			// The global vault outranks ~/.astro/env, so a name both hold is
			// applied from the vault, and that row carries the mark.
			if scope != ScopeGlobal || !globalVault[key] {
				markApplied(&item, scope, key, inProject)
			}
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
		out = append(out, vaultOrphans(src, opts, declared, inProject)...)
	}
	if opts.All {
		out = append(out, crossProjectOrphans(declared, projectDir)...)
	}
	return out
}

// vaultOrphans lists the undeclared names the vault tiers hold.
func vaultOrphans(src Sources, opts ListOptions, declared, inProject map[string]bool) []ListItem {
	var out []ListItem
	for _, tier := range opts.VaultTiers {
		for _, e := range tier.Entries {
			if declared[e.EnvKey] {
				continue
			}
			// A global that does not reach this project is not part of it,
			// so only --all lists it. Outside a project nothing is reached
			// through links, and hiding would empty the view.
			if e.Unlinked && !opts.All && src.hasProject {
				continue
			}
			item := vaultOrphanItem(e.Kind, e.Name, tier, src.hasProject)
			if e.Invalid != "" {
				item.Invalid, item.DeclareHint = e.Invalid, ""
				out = append(out, item)
				continue
			}
			if isAirflowSetting(e.EnvKey) {
				item = ListItem{Kind: e.Kind, Name: e.Name, Source: tier.Label}
			}
			if e.Unlinked {
				// It does not reach this checkout at all, so a start leaves it
				// out whether or not it is declared, and linking is the fix.
				markNotLinked(&item, e, tier)
			} else {
				markApplied(&item, tier.Scope, e.EnvKey, inProject)
			}
			out = append(out, item)
		}
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
// source but never as an orphan.
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
	item.Invalid = InvalidStoredReason(kind, name)
	// A cross-project orphan (project set) lives in another project's file; the
	// delete command runs against the cwd's project, so a hint would point at
	// the wrong file. Only offer it for the current project and the global file.
	if project == "" {
		item.RemoveHint = removeHint(kind, name, scope)
		if inProject && item.Invalid == "" {
			item.DeclareHint = DeclareHint(kind, name)
		}
	}
	return item
}

// markNotLinked marks a row for a global the vault holds that does not reach
// this checkout.
func markNotLinked(item *ListItem, e VaultEntry, tier VaultTier) {
	item.NotLinkedHere = true
	item.LinksDown = tier.LinksDown
	if tier.LinksDown == "" {
		item.LinkHint = LinkHint(e.Kind, e.Name)
	}
}

// markApplied marks an undeclared row that a start passes to the current
// project: everything that reaches a project is applied, declared or not.
// Outside a project there is nothing to apply it to. A global row whose name
// the project's own .env or vault also holds is shadowed by that copy rather
// than applied; inProject holds those keys.
func markApplied(item *ListItem, scope Scope, key string, inProject map[string]bool) {
	if inProject == nil || (scope == ScopeGlobal && inProject[key]) {
		return
	}
	applied := true
	item.Applied = &applied
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

// SetHint is the `astro local env <noun> set` command for a name. It names no
// value: set prompts for one and refuses it as an argument.
func SetHint(kind Kind, name string) string {
	return "astro local env " + Noun(kind) + " set " + name
}

// UndeclareHint is the `astro local env <noun> undeclare` command for a name.
func UndeclareHint(kind Kind, name string) string {
	return "astro local env " + Noun(kind) + " undeclare " + name
}

// LinkHint is the command that links a global vault entry to the project it
// is run in.
func LinkHint(kind Kind, name string) string {
	return "astro local env " + Noun(kind) + " link " + name
}
