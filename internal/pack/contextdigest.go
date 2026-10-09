package pack

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/shipcontext"
)

// projectFiles is what the package found surveying the project files its
// build will copy: the content address of a generated build's copy, and what
// to warn about.
type projectFiles struct {
	digest   string
	warnings []string
}

// dagsIgnoredWarning is the warning for ignore rules that leave every DAG
// file out of a generated image.
const dagsIgnoredWarning = "no DAG file in dags/ reaches the image: .dockerignore leaves them all out, or dags/ links outside the project. A Deployment without DAG deploys runs only the image's DAGs; remove the rule, or move the DAGs into the project, to package them"

// surveyFiles surveys the project files the build will copy, in one walk
// (shipcontext.Take), under the ignore rules the engine will apply: a
// generated build's (the ProjectBuilder's ProjectIgnore), or the file a
// declared Dockerfile's build reads (shipcontext.DeclaredIgnore). It refuses
// files that look like credentials and that git ignores or cannot vouch for,
// and, for a generated build, digests the copy, warns when no DAG file reaches
// the image, and has the build look again just before it copies the project
// (Request.BeforeProjectCopy), since the dependency build before it takes
// minutes.
func surveyFiles(ctx context.Context, cli engineCLI, projectDir, dockerfile string, breq *imagebuild.Request) (projectFiles, error) {
	opts := shipcontext.Options{Git: true}
	var err error
	if breq.ProjectContext != "" {
		opts.Ignore, err = breq.Builder.ProjectIgnore(projectDir, breq.ProjectExcludes)
	} else {
		podman := imagebuild.New(cli.run, time.Now).IsPodman(ctx, imagebuild.Request{Bin: cli.bin, Env: cli.env})
		opts.Ignore, err = shipcontext.DeclaredIgnore(projectDir, dockerfile, podman)
	}
	if err != nil {
		return projectFiles{}, err
	}
	h := sha256.New()
	if breq.ProjectContext != "" {
		opts.Digest = h
	}
	survey, err := shipcontext.Take(projectDir, opts)
	if err != nil {
		return projectFiles{}, err
	}
	if err := survey.SecretsError(); err != nil {
		return projectFiles{}, err
	}
	files := projectFiles{warnings: survey.Warnings()}
	if breq.ProjectContext == "" {
		return files, nil
	}
	if survey.DagsOnDisk > 0 && survey.DagsShipped == 0 {
		files.warnings = append(files.warnings, dagsIgnoredWarning)
	}
	files.digest = fmt.Sprintf("%x", h.Sum(nil))
	breq.BeforeProjectCopy = func() error {
		again, err := shipcontext.Take(projectDir, shipcontext.Options{Ignore: opts.Ignore, Git: true})
		if err != nil {
			return err
		}
		return again.SecretsError()
	}
	return files, nil
}
