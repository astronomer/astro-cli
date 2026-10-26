package local

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// NewProjectCmd builds `astro project` for the root, wired with its own deps.
// It reads the manifest and nothing else, so it works offline with no account.
func NewProjectCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Read this project's settings from pyproject.toml",
		Long: "Read this project's settings: what pyproject.toml's [project] and [tool.astro] sections say, as the " +
			"CLI reads them. The manifest reference in the repository's docs lists every key.",
		Example: "  astro project inspect                     # every setting, as YAML\n" +
			"  astro project inspect -o json\n" +
			"  astro project inspect --key organization",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newProjectInspectCmd(c))
	cliout.AddOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

// newProjectInspectCmd builds `astro project inspect`: the manifest as the CLI
// reads it, whole or one key of it. It is the first-class way for a script, a
// CI job or an agent to read a project setting, in place of parsing
// pyproject.toml itself: the file's values with the CLI's own resolution on
// top, from anywhere inside the project, and refused the way a start refuses
// a manifest that does not load.
func newProjectInspectCmd(c *cli) *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show the project's settings as the CLI reads them",
		Long: "Show the project's settings from pyproject.toml, as the CLI reads them: the [project] name, " +
			"requires-python and dependencies with the Airflow they pin, and every [tool.astro] key, each " +
			"deployment link with its workspace, target and auth method resolved from the two levels. Three keys " +
			"are resolved past the file. domain is [tool.astro] domain, else ASTRO_DOMAIN, else astronomer.io when " +
			"the project names a workspace. organization is [tool.astro] organization, else the organization of " +
			"the login the project's domain uses: an API token's from ASTRO_API_TOKEN, else the saved login's. " +
			"target is [tool.astro] target, else astro. [tool.astro.env] and [tool.astro.targets] are shown as " +
			"written.\n\n" +
			"It runs offline, from anywhere inside the project, and reports a manifest that does not load the way " +
			"astro local start does.\n\n" +
			"With --key, print one value. A string, number or boolean prints bare, so a script can read it; a " +
			"table prints as YAML, or as JSON under --output json. Keys are dotted, spelled as --output json " +
			"spells them: deployments.prod.deployment. A key the project does not have is a usage error.",
		Example: "  # Show the project's settings\n" +
			"  astro project inspect\n\n" +
			"  # As JSON, for a script or an agent\n" +
			"  astro project inspect -o json\n\n" +
			"  # One value, bare: the organization the project is in\n" +
			"  astro project inspect --key organization\n\n" +
			"  # The Deployment id of the link named prod\n" +
			"  astro project inspect --key deployments.prod.deployment",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runProjectInspect(key)
		},
	}
	cmd.Flags().StringVarP(&key, "key", "k", "", "Print only this key, dotted as --output json spells it, such as organization or deployments.prod.deployment")
	return cmd
}

// projectView is what `astro project inspect` publishes: the manifest as the
// CLI reads it. Most keys are the file's, with the manifest's own folding
// applied (a link's inherited workspace and target, its auth method's
// default). Domain, Organization and Target are resolved past the file, as
// their comments say, so a script reads the value the CLI acts on rather than
// the one the file happens to spell out. Every list and table is present,
// empty rather than null, so a reader never meets a missing key for a
// setting the project simply does not use.
//
// The yaml tags are the json tags again: text mode prints the same object as
// YAML, under the same names, so --key reads one spelling.
type projectView struct {
	// Path is the manifest the values were read from.
	Path string `json:"path" yaml:"path"`
	// Name, RequiresPython and Dependencies are the [project] fields the CLI
	// reads; Airflow is the version Dependencies pins, the one statement of
	// the project's Airflow version.
	Name           string   `json:"name" yaml:"name"`
	RequiresPython string   `json:"requires_python,omitempty" yaml:"requires_python,omitempty"`
	Dependencies   []string `json:"dependencies" yaml:"dependencies"`
	Airflow        string   `json:"airflow" yaml:"airflow"`
	// Runtime is [tool.astro] runtime, the one build the image is built
	// from when it is pinned; Dockerfile is the project's own build when it
	// declares one. Both are empty in the common case.
	Runtime    string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty" yaml:"dockerfile,omitempty"`
	// Packages is [tool.astro] packages, the OS packages the image installs;
	// BuildSecrets is [tool.astro] build-secrets, as written.
	Packages     []string `json:"packages" yaml:"packages"`
	BuildSecrets []string `json:"build_secrets" yaml:"build_secrets"`
	// Workspace is [tool.astro] workspace, the linked Astro workspace, which
	// every astro link inherits when it sets none.
	Workspace string `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	// Domain is the Astro host the project's logins are on: [tool.astro]
	// domain, else ASTRO_DOMAIN, else astronomer.io when a workspace is
	// linked (manifest.Astro.WorkspaceDomain). Empty when the project names
	// no host and no workspace, which means the current login's.
	Domain string `json:"domain,omitempty" yaml:"domain,omitempty"`
	// Organization is the organization the project is read under:
	// [tool.astro] organization, else that of the login Domain names, from an
	// API token's claims or the saved login (Deps.LoginOrganization). Empty
	// when neither is known.
	Organization string `json:"organization,omitempty" yaml:"organization,omitempty"`
	// Target is the default target every link inherits: [tool.astro]
	// target, else astro.
	Target string `json:"target" yaml:"target"`
	// Deployments is [tool.astro.deployments], each link by name with its
	// inherited values folded in.
	Deployments map[string]projectLink `json:"deployments" yaml:"deployments"`
	// Env is [tool.astro.env] as written: its typing belongs to pkg/envschema
	// and to the commands that resolve values, and a declaration that does
	// not type still shows here, since the manifest itself loaded.
	Env map[string]any `json:"env" yaml:"env"`
	// Pools is [tool.astro.pools], by name.
	Pools map[string]projectPool `json:"pools" yaml:"pools"`
	// Targets is [tool.astro.targets], each target's table as written: the
	// keys are each platform's own.
	Targets map[string]map[string]any `json:"targets" yaml:"targets"`
}

// projectLink is one deployment link as the CLI reads it: its kind derived,
// and its target, workspace and auth method already resolved from the link's
// own table and the [tool.astro] defaults (manifest.Link).
type projectLink struct {
	Kind   string `json:"kind" yaml:"kind"`
	Target string `json:"target" yaml:"target"`
	// Workspace is set on astro and apc links; Deployment on those two;
	// Environment on mwaa and composer links; URL on an endpoint link; Domain
	// on an apc link. A link has exactly the coordinates its kind takes.
	Workspace   string `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Deployment  string `json:"deployment,omitempty" yaml:"deployment,omitempty"`
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
	Domain      string `json:"domain,omitempty" yaml:"domain,omitempty"`
	// AuthMethod is how the CLI proves itself to the link's Airflow: the
	// link's own auth table's method, else its kind's default. The names of
	// the environment variables a method reads are not published: a script
	// reads those from the environment it runs in.
	AuthMethod string `json:"auth_method" yaml:"auth_method"`
	// Default marks the link commands fall through to when none is named.
	Default bool `json:"default" yaml:"default"`
}

// projectPool is one [tool.astro.pools] entry.
type projectPool struct {
	Slots       int    `json:"slots" yaml:"slots"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// IncludeDeferred is present only when the pool sets it.
	IncludeDeferred *bool `json:"include_deferred,omitempty" yaml:"include_deferred,omitempty"`
}

func (c *cli) runProjectInspect(key string) error {
	r := c.renderer()
	view, err := c.projectView()
	if err != nil {
		return err
	}
	if key == "" {
		return r.Emit(view, func(w io.Writer) error { return writeYAML(w, view) })
	}
	value, err := lookupProjectKey(&view, key)
	if err != nil {
		return err
	}
	return r.Emit(value, func(w io.Writer) error { return writeProjectValue(w, value) })
}

// projectView loads the manifest of the project the working directory is in
// and builds its view. Outside a project the error is project discovery's,
// which names what is missing and carries the no_project kind; a
// pyproject.toml with no [tool.astro] is reported the same way, and one that
// does not load is reported as a start reports it, every problem at once.
func (c *cli) projectView() (projectView, error) {
	dir, err := c.projectPath()
	if err != nil {
		return projectView{}, err
	}
	path := filepath.Join(dir, project.Marker)
	m, err := manifest.Load(path)
	if err != nil {
		wd, _ := c.d.WorkingDir() //nolint:errcheck // projectPath just resolved it; the start only shapes the message
		return projectView{}, project.LoadError(wd, dir, err)
	}
	return c.viewOf(path, m), nil
}

// viewOf is the view of a loaded manifest: the file's values, the manifest's
// own folding, and the three resolutions the view's comments describe.
func (c *cli) viewOf(path string, m *manifest.Manifest) projectView {
	a := &m.Astro
	// The host the project's logins are on. LoginDomain applies no default,
	// so a project that names no host and no workspace reads as none: its
	// commands use the current login, whichever host that is.
	domain := a.LoginDomain()
	if domain == "" && a.Workspace != "" {
		domain = a.WorkspaceDomain()
	}
	login := ""
	if c.d.LoginOrganization != nil {
		login = c.d.LoginOrganization(domain)
	}
	v := projectView{
		Path:           path,
		Name:           m.Project.Name,
		RequiresPython: m.Project.RequiresPython,
		Dependencies:   orEmptyList(m.Project.Dependencies),
		Airflow:        m.Airflow().Pin,
		Runtime:        a.Runtime,
		Dockerfile:     a.Dockerfile,
		Packages:       orEmptyList(a.Packages),
		BuildSecrets:   orEmptyList(a.BuildSecrets),
		Workspace:      a.Workspace,
		Domain:         domain,
		Organization:   a.WorkspaceOrganization(login),
		Target:         cmp.Or(a.Target, string(manifest.KindAstro)),
		Deployments:    make(map[string]projectLink, len(a.Deployments)),
		Env:            orEmptyTable(a.Env),
		Pools:          make(map[string]projectPool, len(a.Pools)),
		Targets:        orEmptyTable(a.Targets),
	}
	// Ranged by key: a link is a wide struct, and each field is read once.
	for name := range a.Deployments {
		l := a.Deployments[name]
		v.Deployments[name] = projectLink{
			Kind:        string(l.Kind()),
			Target:      l.Target,
			Workspace:   l.Workspace,
			Deployment:  l.Deployment,
			Environment: l.Environment,
			URL:         l.URL,
			Domain:      l.Domain,
			AuthMethod:  string(l.Auth.Method),
			Default:     l.Default,
		}
	}
	for name, p := range a.Pools {
		v.Pools[name] = projectPool{Slots: p.Slots, Description: p.Description, IncludeDeferred: p.IncludeDeferred}
	}
	return v
}

// orEmptyList is s, or an empty list for nil, so the key publishes as [] and
// never null.
func orEmptyList(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// orEmptyTable is m, or an empty table for nil, so the key publishes as {}
// and never null.
func orEmptyTable[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

// lookupProjectKey finds key, a dotted path, in the view as --output json
// spells it, so one spelling reads the whole object and one value of it. It
// walks the json encoding rather than the struct, which is what makes the
// spelling the same and lets a path reach into the tables shown as written.
//
// A key the project does not have is a usage error, exit 2: the command was
// asked for something it cannot answer, and a script reading the value
// should stop there rather than go on with an empty one. The message names
// the keys the table has, and says when a top-level key exists but is unset,
// such as an organization the project does not name and no login supplies.
func lookupProjectKey(view *projectView, key string) (any, error) {
	//astro:non-output-json // a round trip to walk the view by its published key names; the value found is what Emit publishes
	data, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	var node any
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	parts := strings.Split(key, ".")
	for i, part := range parts {
		where := strings.Join(parts[:i], ".")
		table, ok := node.(map[string]any)
		if !ok {
			return nil, cliout.Usage(fmt.Errorf("--key %s: %s is a value, not a table, so it has no %s", key, where, part))
		}
		next, ok := table[part]
		if ok {
			node = next
			continue
		}
		if i == 0 && slices.Contains(projectViewKeys(), part) {
			return nil, cliout.Usage(fmt.Errorf("--key %s: this project does not set %s, and nothing supplies one", key, part))
		}
		if where == "" {
			return nil, cliout.Usage(fmt.Errorf("--key %s: no such key. The keys are: %s", key, strings.Join(projectViewKeys(), ", ")))
		}
		return nil, cliout.Usage(fmt.Errorf("--key %s: %s has no %s. Its keys are: %s", key, where, part, strings.Join(slices.Sorted(maps.Keys(table)), ", ")))
	}
	return node, nil
}

// projectViewKeys are the top-level keys --output json spells, in the order
// the object lists them, read from the struct so the two cannot drift.
func projectViewKeys() []string {
	t := reflect.TypeFor[projectView]()
	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

// writeYAML prints v as YAML, two-space indented, as text mode prints the
// whole view and a --key that names a table.
func writeYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return enc.Close()
}

// writeProjectValue prints one --key value in text mode: a table or list as
// YAML, anything else bare on one line, so $(astro project inspect --key
// organization) holds the value and nothing else. The value comes from the
// json encoding, so a number is a float64 and prints without an exponent or
// a trailing .0.
func writeProjectValue(w io.Writer, v any) error {
	switch value := v.(type) {
	case map[string]any, []any:
		return writeYAML(w, value)
	case float64:
		_, err := fmt.Fprintln(w, strconv.FormatFloat(value, 'f', -1, 64))
		return err
	default:
		_, err := fmt.Fprintln(w, value)
		return err
	}
}
