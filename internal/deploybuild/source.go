package deploybuild

import (
	"context"

	"github.com/astronomer/astro-cli/internal/shipcheck"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/util"
)

// Source is a project an image is about to be built from, and how to report
// on it.
type Source struct {
	// ProjectDir is the project's root, and Manifest its loaded pyproject.toml.
	ProjectDir string
	Manifest   *manifest.Manifest
	// DagsOnly is a deploy of the DAGs alone (--dags), and ImageName a
	// prebuilt image (--image-name). Neither builds from the manifest, so
	// neither is checked.
	DagsOnly  bool
	ImageName string
	// BuildSecrets are the docker build --secret specs the build will be
	// handed.
	BuildSecrets []string
	// CheckRuntime holds the manifest's [tool.astro] runtime build to its
	// Airflow pin (runtimeversions.CheckRuntime's contract: warnings to
	// report, and the blocking finding as the error). nil checks nothing.
	CheckRuntime func(runtime, airflowPin string) ([]runtimeversions.Finding, error)
	// Warn reports a finding the deploy goes ahead despite, as a sentence.
	// nil drops it.
	Warn func(string)
	// Engine resolves the container engine that will build the image. It is
	// asked only whether that engine is podman, whose builds of a declared
	// Dockerfile read a .containerignore first. nil, or an engine it cannot
	// resolve, counts as docker.
	Engine func() (bin string, env []string, err error)
}

// CheckSource holds what an image would be built from to the Airflow
// requirement: a declared Dockerfile's FROM line
// (scaffold.CheckDockerfileAirflow), and a [tool.astro] runtime build
// (Source.CheckRuntime). It also checks the build secrets, and holds the files
// the image would carry to shipcheck.Survey: the local and per-machine files a
// declared Dockerfile's context carries are warned about, and so are the files
// git ignores, and a file that looks like a credential and that git does not
// track is refused. It returns the build secrets the declared Dockerfile
// mounts and nobody gave, for a failed build to name again, and the survey of
// what the image would carry, made once for every check of it.
//
// A deploy that builds nothing from the manifest (Source.DagsOnly,
// Source.ImageName) and one with no manifest are not held to any of it, and
// get no survey.
func CheckSource(s *Source) (imagebuild.MissingSecrets, *imagebuild.ProjectSurvey, error) {
	var missing imagebuild.MissingSecrets
	if s.DagsOnly || s.ImageName != "" || s.Manifest == nil {
		return missing, nil, nil
	}
	if err := scaffold.CheckDockerfileAirflow(s.ProjectDir, s.Manifest); err != nil {
		return missing, nil, err
	}
	if err := scaffold.CheckDockerfileOnBuild(s.ProjectDir, s.Manifest); err != nil {
		return missing, nil, err
	}
	warn := s.Warn
	if warn == nil {
		warn = func(string) {}
	}
	if w := scaffold.InstallHelperWarning(s.ProjectDir, s.Manifest.Astro.Dockerfile); w != "" {
		warn(w)
	}
	var err error
	if missing, err = imagebuild.CheckBuildSecrets(s.ProjectDir, s.Manifest.Astro.Dockerfile, s.BuildSecrets); err != nil {
		return missing, nil, err
	}
	missing.Hint = util.BuildSecretHint
	for _, w := range missing.Warnings() {
		warn(w)
	}
	survey, err := shipcheck.Survey(s.ProjectDir, s.Manifest.Astro.Dockerfile, enginePodman(s.Engine), warn)
	if err != nil {
		return missing, nil, err
	}
	airflow := s.Manifest.Airflow()
	if airflow.Runtime == "" || s.CheckRuntime == nil {
		return missing, survey, nil
	}
	warnings, err := s.CheckRuntime(airflow.Runtime, airflow.Pin)
	for _, w := range warnings {
		warn(manifest.Marker + ": tool.astro.runtime: " + w.Message)
	}
	return missing, survey, err
}

// enginePodman reports whether the engine engine resolves is podman, by
// asking it (imagebuild.IsPodman). nil, or an engine that cannot be resolved,
// is not.
func enginePodman(engine func() (string, []string, error)) bool {
	if engine == nil {
		return false
	}
	bin, env, err := engine()
	return err == nil && imagebuild.IsPodman(context.Background(), imagebuild.NewExecCommander(), bin, env)
}
