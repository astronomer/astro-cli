package localenv

import (
	"sort"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// ListItem is one row of `astro local env list`: a declared name with the
// source it resolves from, or an undeclared entry found in a file (an
// orphan). It never carries a value — the row is built from names and
// sources only, so `list` is structurally value-free and safe for agent
// surfaces.
type ListItem struct {
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
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
		items = append(items, declaredItem(rn))
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

func declaredItem(rn envresolve.ResolvedName) ListItem {
	item := ListItem{Name: rn.Name, Source: SourceAbsent}
	// A resolved name carries its winning source; a workspace-source name that
	// did not resolve still carries the "workspace" label (with any
	// "unavailable" reason), so the row says where it was meant to come from.
	// Only a value absent from every source has no label — shown as "absent".
	if rn.Source != "" {
		item.Source = rn.Source
	}
	switch rn.Section {
	case envschema.SectionEnvVar:
		item.Kind = KindEnv
	case envschema.SectionAirflowVariable:
		item.Kind = KindVar
	case envschema.SectionConnection:
		item.Kind = KindConn
	}
	return item
}

// orphans lists file entries that no schema declares. The files searched
// follow the same scope/--all selection as the declared view. projectDir is
// the current project ("" outside one), so --all can skip re-listing it.
func orphans(src Sources, schema *envschema.Schema, opts ListOptions, projectDir string) []ListItem {
	declared := declaredKeySet(schema)
	var out []ListItem
	add := func(files map[string]string, scope Scope) {
		for key := range files {
			if declared[key] {
				continue
			}
			out = append(out, orphanItem(key, scope, ""))
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
			if declared[key] {
				continue
			}
			out = append(out, orphanItem(key, ScopeProject, rec.ProjectPath))
		}
	}
	return out
}

func orphanItem(key string, scope Scope, project string) ListItem {
	kind, name := kindFromKey(key)
	item := ListItem{Kind: kind, Name: name, Source: string(scope), Orphan: true, Project: project}
	// A cross-project orphan (project set) lives in another project's file; the
	// delete command runs against the cwd's project, so a hint would point at
	// the wrong file. Only offer it for the current project and the global file.
	if project == "" {
		item.RemoveHint = removeHint(kind, name, scope)
	}
	return item
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

// removeHint is the exact `astro local env delete` command for an orphan,
// with the scope flag that names the file it lives in.
func removeHint(kind Kind, name string, scope Scope) string {
	return "astro local env delete " + setArgs(kind, name) + " --" + string(scope)
}

// setArgs renders the positional part of a set/get/delete command for a
// kind and name: "NAME", "conn <id>", or "var <key>".
func setArgs(kind Kind, name string) string {
	switch kind {
	case KindConn:
		return "conn " + name
	case KindVar:
		return "var " + name
	case KindEnv:
		return name
	default:
		return name
	}
}
