package pack

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/platformversions"
)

// composerDepsFile is the requirements file the Composer environment update
// reads. It is not named requirements.txt because it is not a bucket file
// Composer syncs — it is the input to `gcloud composer environments update`,
// and the name makes that hand-off plain.
const composerDepsFile = "composer-requirements.txt"

// ComposerTarget builds the artifact Cloud Composer 3 consumes. Composer reads
// dags/ (and plugins/) from a GCS bucket, but it does not read PyPI packages
// from the bucket — those are set on the environment. So the artifact is the
// bucket-shaped tree plus a separate composer-requirements.txt for the
// environment-update step, and the reported next steps make that two-part
// hand-off explicit. Like the MWAA target it touches no network.
type ComposerTarget struct{}

// NewComposerTarget builds the Composer target. The artifact is files derived
// from the manifest, so it has no dependencies.
func NewComposerTarget() *ComposerTarget { return &ComposerTarget{} }

func (t *ComposerTarget) Name() string { return TargetComposer }

// Build writes the bucket tree, the environment deps file, and any plugins, and
// reports the two-part hand-off. It warns (rather than fails) on an Airflow pin
// Composer does not list or on OS packages Composer cannot install.
func (t *ComposerTarget) Build(_ context.Context, req Request, cb localrt.Callbacks) (Result, error) {
	outDir, err := prepareTree(req, TargetComposer)
	if err != nil {
		return Result{}, err
	}
	m := req.Manifest

	emit(cb, "copying dags/")
	if err := copyDags(req.ProjectDir, outDir); err != nil {
		return Result{}, fmt.Errorf("copying dags: %w", err)
	}

	emit(cb, "writing "+composerDepsFile)
	if err := writeFile(outDir, composerDepsFile, composerRequirements(m.Project.Dependencies)); err != nil {
		return Result{}, fmt.Errorf("writing %s: %w", composerDepsFile, err)
	}

	res := Result{
		Target:   TargetComposer,
		Kind:     KindTree,
		TreePath: outDir,
		DepsFile: filepath.Join(outDir, composerDepsFile),
	}
	if _, ok := platformversions.Resolve(m.Airflow().Pin, platformversions.Composer); !ok {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the manifest pins Airflow %q, which Composer 3 does not list (Composer offers: %s); pick a supported version when you create or upgrade the environment",
			m.Airflow().Pin, platformversions.List(platformversions.Composer)))
	}
	if w := packagesWarning(m.Astro.Packages, "Composer"); w != "" {
		res.Warnings = append(res.Warnings, w)
	}
	if w := includeWarning(req.ProjectDir); w != "" {
		res.Warnings = append(res.Warnings, w)
	}

	// Composer reads plugins from a gs://<bucket>/plugins folder, so plugins
	// ship as a folder, not a zip.
	hasPlugins := dirHasFiles(filepath.Join(req.ProjectDir, "plugins"))
	if hasPlugins {
		emit(cb, "copying plugins/")
		if err := copyDir(filepath.Join(req.ProjectDir, "plugins"), filepath.Join(outDir, "plugins")); err != nil {
			return Result{}, fmt.Errorf("copying plugins: %w", err)
		}
	}

	checklist, err := envChecklist(m.Project.Name, m.Astro.Env,
		"Composer does not read this project's [tool.astro.env] section. Set these on the environment before your DAGs run.",
		"Set plain values with gcloud composer environments update --update-env-variables; add connections and variables through the Airflow UI or CLI.")
	if err != nil {
		return Result{}, fmt.Errorf("building the env checklist: %w", err)
	}
	if checklist != "" {
		if err := writeFile(outDir, checklistName, checklist); err != nil {
			return Result{}, fmt.Errorf("writing %s: %w", checklistName, err)
		}
	}

	res.NextSteps = composerNextSteps(outDir, hasPlugins)
	if err := saveTree(req, outDir, &res, cb); err != nil {
		return Result{}, err
	}
	return res, nil
}

// composerRequirements is the manifest dependencies with apache-airflow dropped
// (Composer provides Airflow and rejects a pin of it), for the environment
// update. It carries no --constraint line: Composer resolves the packages
// against the environment's own image. The header comment names the command
// that consumes the file.
func composerRequirements(deps []string) string {
	header := "" +
		"# PyPI dependencies for the Composer environment. Composer installs these\n" +
		"# on the environment, not from the bucket. Apply them with:\n" +
		"#   gcloud composer environments update <env> --location <region> \\\n" +
		"#     --update-pypi-packages-from-file " + composerDepsFile + "\n"
	body := strings.Join(dropAirflow(deps), "\n")
	if body != "" {
		body += "\n"
	}
	return header + body
}

// composerNextSteps is the two-part hand-off: DAGs (and plugins) to the bucket
// through the Composer storage commands, and PyPI dependencies to the
// environment through an update. The storage commands take an environment and
// location, so the user never needs the bucket name.
func composerNextSteps(outDir string, hasPlugins bool) []string {
	steps := []string{
		fmt.Sprintf("gcloud composer environments storage dags import --environment=<env> --location=<region> --source=%s/dags", outDir),
	}
	if hasPlugins {
		steps = append(steps, fmt.Sprintf("gcloud composer environments storage plugins import --environment=<env> --location=<region> --source=%s/plugins", outDir))
	}
	steps = append(steps, fmt.Sprintf("gcloud composer environments update <env> --location=<region> --update-pypi-packages-from-file %s/%s", outDir, composerDepsFile))
	return steps
}
