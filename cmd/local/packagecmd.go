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
		base         bool
		buildSecrets []string
	}
	cmd := &cobra.Command{
		Use:   "package [TARGET]",
		Short: "Build a platform's deployable artifact without shipping it",
		Long: "Build the artifact a given Airflow platform consumes — for CI, or to hand a " +
			"prebuilt image to astro deploy --image-name. TARGET is astro (the default), " +
			"mwaa, composer, or oss. astro builds an image; mwaa and composer build a " +
			"bucket-shaped directory; only oss is not built yet.",
		Example: "  # Build the image Astro runs\n" +
			"  astro package\n\n" +
			"  # Build the bucket layout MWAA reads, and also write it to a zip\n" +
			"  astro package mwaa --save dist/mwaa.zip\n\n" +
			"  # Write the image's build context, to build it with docker or podman yourself\n" +
			"  astro package --context-dir ./build\n\n" +
			"  # Build only the install base your Dockerfile builds FROM ${ASTRO_BASE} on, and print its tag\n" +
			"  astro package --base",
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
				base:         opts.base,
				buildSecrets: opts.buildSecrets,
			})
		},
	}
	cmd.Flags().StringVar(&opts.save, "save", "", "Also write the artifact to this path (a .tar for image targets, a .zip for bucket targets)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "Image reference for image targets (default astro-package/<name>:<runtime>-<hash>)")
	cmd.Flags().StringVar(&opts.platform, "platform", defaultPackagePlatform, "Build platform for image targets")
	cmd.Flags().StringVar(&opts.outDir, "out-dir", "", "Artifact directory for bucket targets (default dist/<target>)")
	cmd.Flags().StringVar(&opts.contextDir, "context-dir", "", "Write the astro image's build context to this new or empty directory instead of building it, and print the docker and podman commands that build it")
	cmd.Flags().BoolVar(&opts.base, "base", false, "Build only the install base your own Dockerfile builds FROM ${ASTRO_BASE} on, and print its tag, for docker build --build-arg ASTRO_BASE=<tag>")
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
	base         bool
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
	if opts.base {
		switch {
		case target.Name() != pack.TargetAstro:
			return fmt.Errorf("--base builds an image, and the %s target builds no image", target.Name())
		case opts.contextDir != "":
			return errors.New("--base builds the install base and --context-dir builds nothing; use one")
		case opts.save != "" || opts.tag != "":
			return errors.New("--base builds the install base under its own local tag, which it prints, so --save and --tag do not apply")
		}
	}
	if opts.contextDir != "" {
		switch {
		case target.Name() != pack.TargetAstro:
			return fmt.Errorf("--context-dir writes an image's build context, and the %s target builds no image", target.Name())
		case opts.save != "":
			return errors.New("--context-dir builds nothing, so there is nothing for --save to write; build the context, then save the image with docker save")
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
	if opts.base && r.Format == cliout.FormatText {
		// In text, --base's stdout is the tag alone, so `$(astro package
		// --base)` is the tag: the build's progress goes to stderr.
		progress := r
		progress.Out = c.d.Stderr
		cb = c.callbacks(progress)
	}
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
		BaseOnly:       opts.base,
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
		return renderPackage(w, res, m.Astro.Deployments)
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
// renderer. links are the project's deployment links, for the deploy command
// an image hands off to.
func renderPackage(w io.Writer, res pack.Result, links map[string]manifest.Link) error {
	// The install base's tag alone, so `$(astro package --base)` is the tag.
	if res.Kind != pack.KindInstallBase {
		if _, err := fmt.Fprintf(w, "target: %s\n", res.Target); err != nil {
			return err
		}
	}
	switch res.Kind {
	case pack.KindInstallBase:
		_, err := fmt.Fprintln(w, res.Image)
		return err
	case pack.KindImage:
		return renderImageResult(w, res, links)
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

// renderImageResult renders an image target's tag, version, and the deploy
// command that consumes it, so the CI story is one copy-paste.
func renderImageResult(w io.Writer, res pack.Result, links map[string]manifest.Link) error {
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
	if _, err := fmt.Fprintf(w, "\nDeploy it with:\n%s\n", deployImageHint(res.Image, links)); err != nil {
		return err
	}
	return renderDeploymentEnv(w, res.DeploymentEnv)
}

// renderDeploymentEnv lists, under the deploy command, the values the
// Deployment must supply because the image carries none of them, one aligned
// row each. Nothing when the project declares none.
func renderDeploymentEnv(w io.Writer, values []pack.DeploymentValue) error {
	if len(values) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "and set these on the Deployment, as environment variables or linked Environment Manager objects:"); err != nil {
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

// deployImageHint is the deploy command for an image. Deploy refuses to pick a
// target when it cannot prompt, so the command names one: the default link, or
// the only deployable one. With several and no default, it names a placeholder
// and lists them the way deploy's own refusal does.
func deployImageHint(image string, links map[string]manifest.Link) string {
	deployable := deploy.DeployableLinks(links)
	target := ""
	if name, _, ok := manifest.DefaultLink(links); ok && slices.Contains(deployable, name) {
		target = name
	} else if len(deployable) == 1 {
		target = deployable[0]
	}
	switch {
	case target != "":
		return fmt.Sprintf("  astro deploy %s --image-name %s", target, image)
	case len(deployable) > 1:
		return fmt.Sprintf("  astro deploy <link> --image-name %s\nDeployable links: %s", image, strings.Join(deployable, ", "))
	default:
		return fmt.Sprintf("  astro deploy --image-name %s", image)
	}
}

// renderTreeResult renders a bucket target's paths, any warnings, and the exact
// upload commands — the hand-off the target cannot run itself.
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
	return renderNextSteps(w, res.NextSteps)
}

// renderContextResult renders a written build context and the commands that
// build it.
func renderContextResult(w io.Writer, res pack.Result) error {
	if _, err := fmt.Fprintf(w, "context: %s\n\nBuild it with either of:\n", res.ContextDir); err != nil {
		return err
	}
	for _, step := range res.NextSteps {
		if _, err := fmt.Fprintf(w, "  %s\n", step); err != nil {
			return err
		}
	}
	return nil
}

// renderNextSteps prints the upload commands under a heading.
func renderNextSteps(w io.Writer, steps []string) error {
	if len(steps) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nUpload it with:\n"); err != nil {
		return err
	}
	for _, step := range steps {
		if _, err := fmt.Fprintf(w, "  %s\n", step); err != nil {
			return err
		}
	}
	return nil
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
