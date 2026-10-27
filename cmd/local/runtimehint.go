package local

import (
	"context"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Standalone mode installs what a project declares. The runtime image also
// has what it preinstalls (pandas, psycopg2, kubernetes, a few providers), so
// a Dag that imports one of those without declaring it works in Docker mode
// and on a Deployment and fails in standalone. The check reports the import
// error either way; this adds the fix when the missing module is, by name, a
// package the image ships: declare it at the image's version, which a
// deployment then installs as it already runs.

// exceptionLineRe matches the line a traceback ends with: the exception's
// type and message.
var exceptionLineRe = regexp.MustCompile(`^[A-Za-z_][\w.]*(Error|Exception|Exit|Interrupt)\b`)

// missingModuleRe reads the module a ModuleNotFoundError names.
var missingModuleRe = regexp.MustCompile(`^ModuleNotFoundError: No module named '([^']+)'$`)

// missingModule is the module an import error's final exception says is
// missing, or "" when it ends in another exception.
func missingModule(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !exceptionLineRe.MatchString(line) {
			continue
		}
		if m := missingModuleRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		return ""
	}
	return ""
}

// withRuntimeHints sets Hint on each import error whose missing module is,
// by name, a package the project's runtime image ships and the project does
// not list. It reads the image's package list only when some finding names a
// missing module, so a clean check makes no request, and changes nothing when
// the list cannot be read.
func (c *cli) withRuntimeHints(ctx context.Context, project string, m *manifest.Manifest, res checks.Result) checks.Result {
	if c.d.RuntimeFreeze == nil {
		return res
	}
	missing := false
	for _, f := range res.Findings {
		if f.Kind == checks.KindImportError && missingModule(f.Message) != "" {
			missing = true
			break
		}
	}
	if !missing {
		return res
	}
	var catalog *runtimeversions.Catalog
	if c.d.RuntimeCatalog != nil {
		catalog = c.d.RuntimeCatalog(ctx)
	}
	build, python, err := scaffold.RuntimeBuildFor(project, m, catalog)
	if err != nil || python == "" {
		return res
	}
	freeze, err := c.d.RuntimeFreeze(ctx, build, python)
	if err != nil {
		return res
	}
	ships := map[string]string{}
	for _, pin := range freeze {
		name, version, _ := strings.Cut(pin, "==")
		ships[manifest.DistName(name)] = name + "==" + version
	}
	listed := map[string]bool{}
	for _, dep := range m.Project.Dependencies {
		listed[manifest.DistName(dep)] = true
	}
	findings := make([]checks.Finding, len(res.Findings))
	copy(findings, res.Findings)
	for i := range findings {
		f := &findings[i]
		if f.Kind != checks.KindImportError {
			continue
		}
		dist := manifest.DistName(missingModule(f.Message))
		pin, ok := ships[dist]
		if dist == "" || !ok || listed[dist] {
			continue
		}
		name, version, _ := strings.Cut(pin, "==")
		f.Hint = "Astro Runtime " + build + " ships " + name + " " + version +
			", but this project doesn't list it: uv add '" + pin + "'"
	}
	res.Findings = findings
	return res
}
