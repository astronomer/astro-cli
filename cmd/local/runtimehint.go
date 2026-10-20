package local

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// A DAG that imports a package the Astro Runtime image happens to ship works in
// Docker mode and on a Deployment, and fails in standalone, whose venv holds
// only what pyproject.toml asks for: pandas, numpy, kubernetes, psycopg2 and
// the providers the image preinstalls are the usual ones. The check reports the
// import error either way. This adds the fix when the missing module is one the
// project's runtime ships and the project does not list: declare it, which
// also stops the project depending on what an image happens to carry.

// missingModuleRe finds the module a ModuleNotFoundError names.
var missingModuleRe = regexp.MustCompile(`No module named '([^']+)'`)

// moduleDists are the modules whose distribution has another name, for the
// packages the runtime images ship. A module not here is looked up by its own
// name (runtimeDist).
var moduleDists = map[string]string{
	"attr":               "attrs",
	"dateutil":           "python-dateutil",
	"daemon":             "python-daemon",
	"dns":                "dnspython",
	"dotenv":             "python-dotenv",
	"google.api":         "googleapis-common-protos",
	"google.protobuf":    "protobuf",
	"google.rpc":         "googleapis-common-protos",
	"jwt":                "pyjwt",
	"linkify_it":         "linkify-it-py",
	"multipart":          "python-multipart",
	"mysql.connector":    "mysql-connector-python",
	"MySQLdb":            "mysqlclient",
	"openlineage.client": "openlineage-python",
	"openlineage.common": "openlineage-integration-common",
	"psycopg2":           "psycopg2-binary",
	"slugify":            "python-slugify",
	"uc_micro":           "uc-micro-py",
	"websocket":          "websocket-client",
	"yaml":               "pyyaml",
}

// airflowProvidersPrefix is the namespace every Apache Airflow provider
// installs under, airflow.providers.<name>, from apache-airflow-providers-<name>.
const airflowProvidersPrefix = "airflow.providers."

// runtimeDist is the distribution in ships that provides module, or "".
//
// Longest dotted prefix first, so google.protobuf.message finds protobuf
// before google is tried, and each prefix three ways: moduleDists, the
// provider rule (airflow.providers.common.sql is
// apache-airflow-providers-common-sql), and the prefix with its dots as
// hyphens (opentelemetry.sdk is opentelemetry-sdk, kubernetes_asyncio is
// kubernetes-asyncio).
func runtimeDist(module string, ships map[string]bool) string {
	parts := strings.Split(module, ".")
	for n := len(parts); n > 0; n-- {
		prefix := strings.Join(parts[:n], ".")
		candidates := []string{moduleDists[prefix]}
		if rest, ok := strings.CutPrefix(prefix, airflowProvidersPrefix); ok {
			candidates = append(candidates, "apache-airflow-providers-"+strings.ReplaceAll(rest, ".", "-"))
		}
		candidates = append(candidates, strings.Join(parts[:n], "-"))
		for _, c := range candidates {
			if c == "" {
				continue
			}
			if d := manifest.DistName(c); ships[d] {
				return d
			}
		}
	}
	return ""
}

// missingModule is the module an import error's message says is missing, or
// "".
func missingModule(message string) string {
	if m := missingModuleRe.FindStringSubmatch(message); m != nil {
		return m[1]
	}
	return ""
}

// withRuntimeHints sets Hint on each import error for a module the project's
// runtime image ships and its manifest does not list. It reads the runtime
// freeze only when some finding names a missing module, so a clean check
// makes no request, and changes nothing when the freeze cannot be read.
func (c *cli) withRuntimeHints(ctx context.Context, project string, m *manifest.Manifest, res checks.Result) checks.Result {
	if c.d.RuntimeFreeze == nil || !slices.ContainsFunc(res.Findings, func(f checks.Finding) bool {
		return f.Kind == checks.KindImportError && missingModule(f.Message) != ""
	}) {
		return res
	}
	build, pythons := c.runtimeImageOf(ctx, project, m)
	if build == "" {
		return res
	}
	ships := map[string]bool{}
	for _, d := range c.d.RuntimeFreeze(ctx, build, pythons) {
		ships[d] = true
	}
	if len(ships) == 0 {
		return res
	}
	declared := map[string]bool{}
	for _, dep := range m.Project.Dependencies {
		declared[manifest.DistName(dep)] = true
	}
	findings := slices.Clone(res.Findings)
	for i := range findings {
		f := &findings[i]
		if f.Kind != checks.KindImportError {
			continue
		}
		// A dependency the project already lists is missing from an
		// environment that is out of date, which adding it again does not fix.
		if d := runtimeDist(missingModule(f.Message), ships); d != "" && !declared[d] {
			f.Hint = "Astro Runtime " + build + " ships " + d + ", but this project does not depend on it: add it with uv add " + d
		}
	}
	res.Findings = findings
	return res
}

// fallbackPythons are tried after the Python the catalog names, for an
// Airflow 2 build, for which the catalog lists none.
var fallbackPythons = []string{"3.12", "3.11"}

// runtimeImageOf is the runtime build the project's image is built from and the
// Pythons to try for it, best first: the declared Dockerfile's FROM when it
// names an exact build, else [tool.astro] runtime, else the newest published
// build of the Airflow pin. build is "" when none of those gives one.
//
// A Dockerfile FROM a series tag (runtime:3.3) names no build and is read as
// the pin is: the newest build of it.
func (c *cli) runtimeImageOf(ctx context.Context, project string, m *manifest.Manifest) (build string, pythons []string) {
	pin := m.Airflow().Pin
	if pin == "" {
		return "", nil
	}
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build = m.Astro.Runtime
	var tagPython string
	if m.Astro.Dockerfile != "" {
		base := airflowrt.ReadDeclaredBase(filepath.Join(project, filepath.FromSlash(m.Astro.Dockerfile)))
		if v := base.RuntimeVersion(); v != "" {
			build = v
			_, tagPython = airflowrt.ParseRuntimeTagPython(base.Tag)
		}
	}
	if catalog != nil {
		if _, ok := catalog.Runtime(build); !ok {
			build, _ = catalog.NewestPublishedRuntimeFor(pin)
		}
	}
	if build == "" {
		return "", nil
	}
	add := func(py string) {
		if py = strings.TrimSpace(py); py != "" && !slices.Contains(pythons, py) {
			pythons = append(pythons, py)
		}
	}
	add(tagPython)
	if catalog != nil {
		py, _, _ := runtimeversions.ProjectPython(pin, build, m.Project.RequiresPython, func() *runtimeversions.Catalog { return catalog }) //nolint:errcheck // no Python is an answer here: the fallbacks follow
		add(py)
		if r, ok := catalog.Runtime(build); ok {
			add(r.DefaultPythonVersion)
		}
	}
	for _, py := range fallbackPythons {
		add(py)
	}
	return build, pythons
}
