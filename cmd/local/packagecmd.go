package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/containercfg"
	"github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/imageinstall"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/util"
)

// defaultPackagePlatform is the build platform `astro package` targets by
// default: a deployable image runs on Astro's linux/amd64 nodes, not the host.
const defaultPackagePlatform = "linux/amd64"

// NewPackageCmd builds `astro package [TARGET]` for a root to mount. It reads
// the manifest and the project files, needs no account and no deployment link,
// and touches the network only to pull a base image — the CI build stage
// (docs/deploy.md, section 4). The astro, mwaa and composer targets build;
// oss is registered but not built.
func NewPackageCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newPackageCmd(c)
	cliout.AddOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

func newPackageCmd(c *cli) *cobra.Command {
	var opts struct {
		save         string
		tag          string
		platform     string
		outDir       string
		contextDir   string
		buildSecrets []string
	}
	cmd := &cobra.Command{
		Use:   "package [TARGET]",
		Short: "Build a platform's deployable artifact without shipping it",
		Long: "Build the artifact that an Airflow platform consumes, for CI or to give a " +
			"prebuilt image to astro deploy --image-name.\n\n" +
			"TARGET is astro (the default), mwaa, composer, or oss. astro builds an image. mwaa and " +
			"composer build a bucket-shaped directory. oss is not built yet.",
		Example: "  # Build the image Astro runs\n" +
			"  astro package\n\n" +
			"  # Build the bucket layout MWAA reads, and also write it to a zip\n" +
			"  astro package mwaa --save dist/mwaa.zip\n\n" +
			"  # Write the image's build context, to build it with docker or podman yourself\n" +
			"  astro package --context-dir ./build",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := pack.TargetAstro
			if len(args) == 1 {
				target = args[0]
			}
			return c.runPackage(cmd.Context(), target, packageOptions{
				save:         opts.save,
				tag:          opts.tag,
				platform:     opts.platform,
				outDir:       opts.outDir,
				contextDir:   opts.contextDir,
				buildSecrets: opts.buildSecrets,
			})
		},
	}
	cmd.Flags().StringVar(&opts.save, "save", "", "Also write the artifact to this path (a .tar for image targets, a .zip for bucket targets)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "Image reference for image targets (default astro-package/<name>:<runtime>-<hash>)")
	cmd.Flags().StringVar(&opts.platform, "platform", defaultPackagePlatform, "Build platform for image targets")
	cmd.Flags().StringVar(&opts.outDir, "out-dir", "", "Artifact directory for bucket targets (default dist/<target>)")
	cmd.Flags().StringVar(&opts.contextDir, "context-dir", "", "Write the astro image's build context to this new or empty directory and print the docker and podman commands that build it, without building")
	addBuildSecretFlag(cmd, &opts.buildSecrets)
	return cmd
}

// packageOptions carries the flag values into the run function.
type packageOptions struct {
	save         string
	tag          string
	platform     string
	outDir       string
	contextDir   string
	buildSecrets []string
}

func (c *cli) runPackage(ctx context.Context, targetName string, opts packageOptions) error {
	r := c.renderer()
	// Resolve the target first, so an unknown name errors without needing a
	// project in the working directory.
	target, err := c.packageRegistry().Lookup(targetName)
	if err != nil {
		return err
	}
	dir, err := c.projectPath()
	if err != nil {
		return err
	}
	// Every target, not only the image one: the artifact is this project, and a
	// project whose Dockerfile and requirement name two Airflows is one no
	// other run path accepts either.
	m, err := loadCheckedManifest(dir)
	if err != nil {
		return err
	}
	if opts.contextDir != "" {
		switch {
		case target.Name() != pack.TargetAstro:
			return fmt.Errorf("--context-dir writes an image's build context, and the %s target builds no image", target.Name())
		case opts.save != "":
			return errors.New("--context-dir builds nothing, so --save has nothing to write; build the context, then save the image with docker save")
		}
	}
	if len(opts.buildSecrets) > 0 {
		switch {
		case target.Name() != pack.TargetAstro:
			return fmt.Errorf("--build-secret has no effect with the %s target: it builds no image", target.Name())
		case m.Astro.Dockerfile == "":
			if err := util.CheckGeneratedBuildSecrets(opts.buildSecrets); err != nil {
				return err
			}
		}
	}
	var catalog func(context.Context) *runtimeversions.Catalog
	if c.d.ImageCatalog != nil {
		pin := m.Airflow().Pin
		catalog = func(ctx context.Context) *runtimeversions.Catalog { return c.d.ImageCatalog(ctx, pin) }
	}
	// In text, a warning or note the target finds as it goes prints once,
	// after the build's output, just above the result, where a long build
	// cannot scroll it away, or before the error a failed build returns
	// (packageRecap). shown keeps the warnings already given, for the result's
	// warnings below.
	cb := c.callbacks(r)
	recap := packageRecap(r)
	shown := map[string]bool{}
	warn := func(w string) {
		shown[w] = true
		recap.Warn(w)
	}
	res, err := target.Build(ctx, pack.Request{
		ProjectDir:     dir,
		Manifest:       m,
		Save:           opts.save,
		Tag:            opts.tag,
		Platform:       opts.platform,
		BuildSecrets:   util.ResolveProjectBuildSecrets(opts.buildSecrets, m.Astro.BuildSecretSpecs()),
		OutDir:         opts.outDir,
		ContextDir:     opts.contextDir,
		ResolveInstall: imageinstall.Fill,
		CheckRuntime:   c.d.RuntimeCheck,
		RuntimeCatalog: catalog,
		Warn:           warn,
	}, cb)
	if err != nil {
		recap.Flush()
		return err
	}
	// In text, a bucket target's own warnings, which it returns rather than
	// gives as it goes, print with the rest, above the result rather than
	// listed in it. json's result carries every one.
	if r.Format == cliout.FormatText {
		for _, w := range res.Warnings {
			if !shown[w] {
				recap.Warn(w)
			}
		}
	}
	recap.Flush()
	// An event rather than a result: an image build streamed its progress
	// before it, and this ends that stream, so it is one line too.
	return r.EmitEvent(res, func(w io.Writer) error {
		if err := renderPackage(w, res); err != nil {
			return err
		}
		cliout.PrintNextSteps(c.d.Stderr, packageNextSteps(res, m.Astro.Deployments))
		return nil
	})
}

// packageRegistry builds the target registry with the production astro target:
// the shared image builder and a container command runner, both backed by
// os/exec, driving the engine container.binary picks for the project. A test's
// Deps.PackageAstro stands in for it.
func (c *cli) packageRegistry() *pack.Registry {
	if c.d.PackageAstro != nil {
		return pack.NewRegistry(c.d.PackageAstro)
	}
	cmd := imagebuild.NewExecCommander()
	astro := pack.NewAstroTarget(imagebuild.New(cmd, time.Now), cmd, containercfg.Engine)
	return pack.NewRegistry(astro)
}

// renderPackage renders a finished build as text. json mode serializes the
// Result directly, so this runs only for text. Each artifact shape has its own
// renderer. The commands that hand the artifact off are next steps
// (packageNextSteps), on stderr.
func renderPackage(w io.Writer, res pack.Result) error {
	if _, err := fmt.Fprintf(w, "target: %s\n", res.Target); err != nil {
		return err
	}
	switch res.Kind {
	case pack.KindImage:
		return renderImageResult(w, res)
	case pack.KindTree:
		return renderTreeResult(w, res)
	case pack.KindBundle:
		_, err := fmt.Fprintf(w, "bundle: %s\n", res.BundlePath)
		return err
	case pack.KindContext:
		return renderContextResult(w, res)
	}
	return nil
}

// renderImageResult renders an image target's tag and version, and the
// values the Deployment it is deployed to must supply.
func renderImageResult(w io.Writer, res pack.Result) error {
	if _, err := fmt.Fprintf(w, "image:  %s\n", res.Image); err != nil {
		return err
	}
	if res.RuntimeVersion != "" {
		if _, err := fmt.Fprintf(w, "runtime: %s\n", res.RuntimeVersion); err != nil {
			return err
		}
	}
	if res.SavedPath != "" {
		if _, err := fmt.Fprintf(w, "saved:  %s\n", res.SavedPath); err != nil {
			return err
		}
	}
	return renderDeploymentEnv(w, res.DeploymentEnv)
}

// renderDeploymentEnv lists the values the Deployment must supply because the
// image carries none of them, one aligned row each. Nothing when the project
// declares none.
func renderDeploymentEnv(w io.Writer, values []pack.DeploymentValue) error {
	if len(values) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nSet these on the Deployment, as environment variables or linked Environment Manager objects:"); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, v := range values {
		if _, err := fmt.Fprintf(tw, "  %s %s\t%s\n", v.Kind, v.Name, v.Status); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// packageNextSteps are the commands that hand an artifact off, which the
// package cannot run itself: deploy an image, upload a tree, or build a
// context with either engine. They are required: an artifact nobody knows how
// to ship is not finished, so they print in CI and under
// ASTRO_GUIDANCE_DISABLED too.
func packageNextSteps(res pack.Result, links map[string]manifest.Link) cliout.NextSteps {
	var steps []cliout.NextStep
	switch res.Kind {
	case pack.KindImage:
		steps = []cliout.NextStep{deployImageStep(res.Image, links)}
	case pack.KindTree:
		for _, cmd := range res.NextSteps {
			steps = append(steps, cliout.NextStep{Command: cmd})
		}
	case pack.KindContext:
		for _, cmd := range res.NextSteps {
			engine, _, _ := strings.Cut(cmd, " ")
			steps = append(steps, cliout.NextStep{Why: "Build it with " + engine, Command: cmd})
		}
	case pack.KindBundle:
		// A bundle is the deploy input itself; nothing hands it off.
	}
	return cliout.NextSteps{Steps: steps, Required: true}
}

// deployImageStep is the deploy command for an image. Deploy refuses to pick a
// target when it cannot prompt, so the command names one: the default link, or
// the only deployable one. With several and no default, it names a placeholder
// and lists them the way deploy's own refusal does.
func deployImageStep(image string, links map[string]manifest.Link) cliout.NextStep {
	deployable := deploy.DeployableLinks(links)
	target := ""
	if name, _, ok := manifest.DefaultLink(links); ok && slices.Contains(deployable, name) {
		target = name
	} else if len(deployable) == 1 {
		target = deployable[0]
	}
	switch {
	case target != "":
		return cliout.NextStep{Why: "Deploy it to " + target, Command: fmt.Sprintf("astro deploy %s --image-name %s", target, image)}
	case len(deployable) > 1:
		return cliout.NextStep{
			Why:     "Deploy it to one of " + strings.Join(deployable, ", "),
			Command: fmt.Sprintf("astro deploy <link> --image-name %s", image),
		}
	default:
		return cliout.NextStep{Why: "Deploy it", Command: "astro deploy --image-name " + image}
	}
}

// renderTreeResult renders a bucket target's paths. The upload commands are
// next steps (packageNextSteps).
func renderTreeResult(w io.Writer, res pack.Result) error {
	if _, err := fmt.Fprintf(w, "tree:   %s\n", res.TreePath); err != nil {
		return err
	}
	if res.DepsFile != "" {
		if _, err := fmt.Fprintf(w, "deps:   %s\n", res.DepsFile); err != nil {
			return err
		}
	}
	if res.SavedPath != "" {
		if _, err := fmt.Fprintf(w, "saved:  %s\n", res.SavedPath); err != nil {
			return err
		}
	}
	return nil
}

// renderContextResult renders a written build context. The commands that
// build it are next steps (packageNextSteps).
func renderContextResult(w io.Writer, res pack.Result) error {
	_, err := fmt.Fprintf(w, "context: %s\n", res.ContextDir)
	return err
}

// packageRecap shows the warnings a package finds. In text they are
// Warning: lines on stderr (or stdout when there is no stderr), held until
// after the build's progress. Under json each is a warning event, emitted as
// found, as it always was.
func packageRecap(r cliout.Renderer) *cliout.Recap {
	if r.Format == cliout.FormatText {
		w := r.Err
		if w == nil {
			w = r.Out
		}
		return cliout.NewRecap(w, true)
	}
	return cliout.NewRecapFunc(func(_ bool, msg string) {
		emitWarning(r, event{Event: "warning", Text: msg})
	}, false)
}
