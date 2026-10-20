package local

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// A DAG that imports a package the Astro Runtime image happens to ship works in
// Docker mode and on a Deployment, and fails in standalone, whose venv holds
// only what pyproject.toml asks for: pandas, numpy, kubernetes, psycopg2 and
// the providers the image preinstalls are the usual ones. The check reports the
// import error either way. This adds the fix when the missing module is one the
// project's runtime ships and the project does not list: declare it, which
// also stops the project depending on what an image happens to carry.

// missingModuleRe reads the module out of the exception line of a failed
// import.
var missingModuleRe = regexp.MustCompile(`^(?:ModuleNotFoundError|ImportError): No module named '([^']+)'`)

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

// moduleOwners are the distributions that would install module itself,
// normalized, three ways: moduleDists, the provider rule
// (airflow.providers.common.sql is apache-airflow-providers-common-sql), and
// the module with its dots as hyphens (opentelemetry.sdk is opentelemetry-sdk,
// kubernetes_asyncio is kubernetes-asyncio).
func moduleOwners(module string) []string {
	var owners []string
	add := func(name string) {
		if d := manifest.DistName(name); d != "" && !slices.Contains(owners, d) {
			owners = append(owners, d)
		}
	}
	add(moduleDists[module])
	if rest, ok := strings.CutPrefix(module, airflowProvidersPrefix); ok {
		add("apache-airflow-providers-" + strings.ReplaceAll(rest, ".", "-"))
	}
	add(strings.ReplaceAll(module, ".", "-"))
	return owners
}

// runtimeDist is the distribution in ships that provides module, or "".
//
// Python names the first package of an import that is missing, so module is
// the missing one and its parent is installed: pandas.io.foo missing means
// pandas is there, and the answer is never a parent's distribution. module is
// matched as it is (moduleOwners), and failing that, under airflow.providers
// only, as a namespace: Python reports airflow.providers.cncf, not the
// provider under it, when no cncf provider is installed. The one shipped
// provider under it is the answer; several are none, since the message cannot
// say which one the Dag needs.
//
// No other namespace is read that way. A shipped distribution whose name
// starts with a module's is not evidence that it provides it (markdown is not
// markdown-it-py, prometheus is not prometheus-client), and a lone google-*
// one is not evidence it is the google.cloud package the Dag imports.
func runtimeDist(module string, ships map[string]bool) string {
	if module == "" {
		return ""
	}
	for _, d := range moduleOwners(module) {
		if ships[d] {
			return d
		}
	}
	rest, ok := strings.CutPrefix(module, airflowProvidersPrefix)
	if !ok {
		return ""
	}
	prefix := manifest.DistName("apache-airflow-providers-"+strings.ReplaceAll(rest, ".", "-")) + "-"
	var under []string
	for d := range ships {
		if strings.HasPrefix(d, prefix) {
			under = append(under, d)
		}
	}
	if len(under) != 1 {
		return ""
	}
	return under[0]
}

// missingModule is the module an import error's message says is missing, or
// "": read from the exception the error ends with (exceptionLine), and only
// when that is the ModuleNotFoundError or ImportError naming it. A chained
// traceback that ends in another exception (a NameError raised while handling
// the missing import) did not fail for the module, and gets no hint.
func missingModule(message string) string {
	if m := missingModuleRe.FindStringSubmatch(exceptionLine(message)); m != nil {
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
	// What the project already gets: what it lists, and what its lock brings
	// in beside that. A module one of these provides is missing from an
	// environment that is out of date, which adding a dependency does not fix.
	known := lockedDists(project)
	for _, dep := range m.Project.Dependencies {
		known[manifest.DistName(dep)] = true
	}
	// The extras and dependency groups too, by the rule the runtime pins use.
	// A file that cannot be read leaves the dependencies the manifest has.
	if listed, err := scaffold.ListedPackages(project); err == nil {
		maps.Copy(known, listed)
	}
	findings := slices.Clone(res.Findings)
	for i := range findings {
		f := &findings[i]
		if f.Kind != checks.KindImportError {
			continue
		}
		module := missingModule(f.Message)
		d := runtimeDist(module, ships)
		// moduleOwners as well as d: a project that lists psycopg2 has the
		// module psycopg2 that the image ships as psycopg2-binary.
		if d == "" || known[d] || slices.ContainsFunc(moduleOwners(module), func(o string) bool { return known[o] }) {
			continue
		}
		f.Hint = "Astro Runtime " + build + " ships " + d + ", but this project does not depend on it: add it with uv add " + d
	}
	res.Findings = findings
	return res
}

// lockedDists are the package names in the project's uv.lock, normalized; an
// empty set when there is no lock or it does not parse, which only costs the
// hint's caution.
func lockedDists(project string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(project, "uv.lock"))
	if err != nil {
		return out
	}
	var lock struct {
		Package []struct {
			Name string `toml:"name"`
		} `toml:"package"`
	}
	if toml.Unmarshal(data, &lock) != nil {
		return out
	}
	for _, p := range lock.Package {
		out[manifest.DistName(p.Name)] = true
	}
	return out
}

// fallbackPythons are tried after the Python the catalog names, for an
// Airflow 2 build, for which the catalog lists none.
var fallbackPythons = []string{"3.12", "3.11"}

// runtimeImageOf is the runtime build the project's image is built from and the
// Pythons to try for it, best first: scaffold.RuntimeBuildFor's build and
// Python, the one answer the runtime pins use too, then the build's default
// Python, then fallbackPythons for a build whose Python none of that tells.
// build is "" when there is none to name.
//
// A hint is a guess worth making where the pins are not: when that names no
// build the catalog lists (a FROM built from an argument, pinned by digest or
// on another image, or a [tool.astro] runtime the catalog does not have), the
// newest build of the Airflow pin is asked about instead, on the Python a
// generated image of it runs for the project's requires-python.
func (c *cli) runtimeImageOf(ctx context.Context, project string, m *manifest.Manifest) (build string, pythons []string) {
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(project, m, catalog)
	var runtime runtimeversions.Runtime
	listed := false
	if catalog != nil {
		runtime, listed = catalog.Runtime(build)
		if err != nil || !listed {
			newest, ok := catalog.NewestPublishedRuntimeFor(m.Airflow().Pin)
			if !ok {
				return "", nil
			}
			build, err = newest, nil
			runtime, listed = catalog.Runtime(newest)
			python = ""
			if py, decided := runtime.ImagePython(m.Project.RequiresPython); decided {
				python = py
			}
		}
	}
	if err != nil {
		return "", nil
	}
	candidates := []string{python}
	if listed {
		candidates = append(candidates, runtime.DefaultPythonVersion)
	}
	for _, py := range append(candidates, fallbackPythons...) {
		if py = strings.TrimSpace(py); py != "" && !slices.Contains(pythons, py) {
			pythons = append(pythons, py)
		}
	}
	return build, pythons
}
