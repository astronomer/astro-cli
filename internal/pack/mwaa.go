package pack

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/platformversions"
)

// MWAATarget builds the artifact Amazon MWAA consumes: a directory laid out as
// the environment's S3 prefix — a dags/ folder, a requirements.txt derived from
// the manifest, and a plugins.zip when the project has plugins. MWAA runs no
// image you supply, so there is nothing to build; getting the layout and the
// requirements pin right is the whole job. The target never touches the
// network — it emits the local tree and reports the exact `aws s3 sync` to run.
type MWAATarget struct{}

// NewMWAATarget builds the MWAA target. It has no dependencies: the artifact is
// files, derived from the manifest.
func NewMWAATarget() *MWAATarget { return &MWAATarget{} }

func (t *MWAATarget) Name() string { return TargetMWAA }

// Build writes the S3-shaped artifact and reports it. It resolves the manifest's
// Airflow pin against MWAA's supported versions to pin the constraints file, and
// warns (rather than fails) on a pin MWAA does not list or on OS packages MWAA
// cannot install — the artifact is valid either way.
func (t *MWAATarget) Build(_ context.Context, req Request, cb localrt.Callbacks) (Result, error) {
	outDir, err := prepareTree(req, TargetMWAA)
	if err != nil {
		return Result{}, err
	}
	m := req.Manifest

	emit(cb, "copying dags/")
	if err := copyDags(req.ProjectDir, outDir); err != nil {
		return Result{}, fmt.Errorf("copying dags: %w", err)
	}

	emit(cb, "writing requirements.txt")
	reqs, versionWarning := mwaaRequirements(m.Airflow().Pin, m.Requirements())
	if err := writeFile(outDir, "requirements.txt", reqs); err != nil {
		return Result{}, fmt.Errorf("writing requirements.txt: %w", err)
	}

	res := Result{
		Target:   TargetMWAA,
		Kind:     KindTree,
		TreePath: outDir,
		DepsFile: filepath.Join(outDir, "requirements.txt"),
	}
	if versionWarning != "" {
		res.Warnings = append(res.Warnings, versionWarning)
	}
	if w := packagesWarning(m.Astro.Packages, "MWAA"); w != "" {
		res.Warnings = append(res.Warnings, w)
	}
	if w := includeWarning(req.ProjectDir); w != "" {
		res.Warnings = append(res.Warnings, w)
	}

	// plugins.zip only when the project has plugins; MWAA reads a single zip,
	// not a plugins/ prefix.
	hasPlugins := dirHasFiles(filepath.Join(req.ProjectDir, "plugins"))
	if hasPlugins {
		emit(cb, "writing plugins.zip")
		if err := zipDir(filepath.Join(req.ProjectDir, "plugins"), filepath.Join(outDir, "plugins.zip")); err != nil {
			return Result{}, fmt.Errorf("writing plugins.zip: %w", err)
		}
	}

	checklist, err := envChecklist(m.Project.Name, m.Astro.Env,
		"MWAA does not read this project's [tool.astro.env] section. Set these on the environment before your DAGs run.",
		"Set plain values as Apache Airflow configuration options on the environment; add connections and variables through the Airflow UI or CLI.")
	if err != nil {
		return Result{}, fmt.Errorf("building the env checklist: %w", err)
	}
	if checklist != "" {
		if err := writeFile(outDir, checklistName, checklist); err != nil {
			return Result{}, fmt.Errorf("writing %s: %w", checklistName, err)
		}
	}

	res.NextSteps = mwaaNextSteps(outDir, hasPlugins, mwaaBucket(m))
	if err := saveTree(req, outDir, &res, cb); err != nil {
		return Result{}, err
	}
	return res, nil
}

// mwaaRequirements builds the requirements.txt body: a --constraint line pinned
// to the matched MWAA Airflow version, then the manifest dependencies with
// apache-airflow dropped (MWAA provides Airflow and rejects a pin of it). When
// the manifest pin matches no MWAA version, it writes a commented constraint
// template instead of a wrong pin and returns a warning; MWAA then supplies its
// own constraint.
func mwaaRequirements(airflowPin string, deps []string) (content, warning string) {
	body := strings.Join(dropAirflow(deps), "\n")
	if body != "" {
		body += "\n"
	}
	v, ok := platformversions.Resolve(airflowPin, platformversions.MWAA)
	if ok {
		header := fmt.Sprintf("--constraint %q\n", platformversions.MWAAConstraintURL(v))
		return header + body, ""
	}
	header := "" +
		fmt.Sprintf("# The manifest pins Airflow %q, which MWAA does not offer.\n", airflowPin) +
		fmt.Sprintf("# MWAA offers: %s.\n", platformversions.List(platformversions.MWAA)) +
		"# Pick the version your environment runs and add its constraint, e.g.:\n" +
		"#   --constraint \"https://raw.githubusercontent.com/apache/airflow/constraints-<version>/constraints-<python>.txt\"\n" +
		"# Without a --constraint line MWAA supplies one for you.\n"
	warning = fmt.Sprintf("the manifest pins Airflow %q, which MWAA does not list (MWAA offers: %s); requirements.txt carries a commented constraint template instead of a pinned one",
		airflowPin, platformversions.List(platformversions.MWAA))
	return header + body, warning
}

// mwaaBucketKey is the [tool.astro.targets.mwaa] field naming the S3 bucket
// the environment reads its source from. The upload command names it when it
// is set; nothing else reads it.
const mwaaBucketKey = "bucket"

// mwaaBucket is the bucket from [tool.astro.targets.mwaa], as the s3:// URL the
// sync command takes, or a placeholder when the manifest names none. It takes
// the bucket with or without its s3:// scheme, since the docs write it with
// one and the console shows it without.
func mwaaBucket(m *manifest.Manifest) string {
	const placeholder = "s3://<your-mwaa-bucket>/"
	name, _ := m.Astro.Targets[TargetMWAA][mwaaBucketKey].(string)
	name = strings.Trim(strings.TrimPrefix(strings.TrimSpace(name), "s3://"), "/")
	if name == "" {
		return placeholder
	}
	return "s3://" + name + "/"
}

// mwaaNextSteps is the upload hand-off: sync the tree to the environment's S3
// bucket, then point the environment at the new requirements.txt (and
// plugins.zip) object version, which MWAA tracks by S3 version id.
func mwaaNextSteps(outDir string, hasPlugins bool, bucket string) []string {
	steps := []string{
		fmt.Sprintf("aws s3 sync %s/ %s", outDir, bucket),
		"Point the environment at the new requirements.txt object version (MWAA console, or aws mwaa update-environment --requirements-s3-object-version <ver>).",
	}
	if hasPlugins {
		steps = append(steps, "Point the environment at the new plugins.zip object version (--plugins-s3-object-version <ver>).")
	}
	return steps
}
