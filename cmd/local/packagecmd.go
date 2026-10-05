package local

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/containercfg"
	"github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/util"
)

// defaultPackagePlatform is the build platform `astro package` targets by
// default: a deployable image runs on Astro's linux/amd64 nodes, not the host.
const defaultPackagePlatform = "linux/amd64"

// NewPackageCmd builds `astro package [target]` for a root to mount. It reads
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
		buildSecrets []string
	}
	cmd := &cobra.Command{
		Use:   "package [target]",
		Short: "Build the deployable artifact for an Airflow platform without shipping it",
		Long: "Build the artifact a given Airflow platform consumes — for CI, or to hand a\n" +
			"prebuilt image to `astro deploy --image-name`. TARGET is astro (the default),\n" +
			"mwaa, composer, or oss. astro builds an image; mwaa and composer build a\n" +
			"bucket-shaped directory; only oss is not built yet.",
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
				buildSecrets: opts.buildSecrets,
			})
		},
	}
	cmd.Flags().StringVar(&opts.save, "save", "", "Also write the artifact to this path (a .tar for image targets, a .zip for bucket targets)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "Image reference for image targets (default astro-package/<name>:<runtime>-<hash>)")
	cmd.Flags().StringVar(&opts.platform, "platform", defaultPackagePlatform, "Build platform for image targets")
	cmd.Flags().StringVar(&opts.outDir, "out-dir", "", "Artifact directory for bucket targets (default dist/<target>)")
	addBuildSecretFlag(cmd, &opts.buildSecrets)
	return cmd
}

// packageOptions carries the flag values into the run function.
type packageOptions struct {
	save         string
	tag          string
	platform     string
	outDir       string
	buildSecrets []string
}

func (c *cli) runPackage(ctx context.Context, targetName string, opts packageOptions) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
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
	res, err := target.Build(ctx, pack.Request{
		ProjectDir:     dir,
		Manifest:       m,
		Save:           opts.save,
		Tag:            opts.tag,
		Platform:       opts.platform,
		BuildSecrets:   util.ResolveProjectBuildSecrets(opts.buildSecrets, m.Astro.BuildSecretSpecs()),
		OutDir:         opts.outDir,
		CheckRuntime:   c.d.RuntimeCheck,
		RuntimeCatalog: c.d.RuntimeCatalog,
	}, c.callbacks(r))
	if err != nil {
		return err
	}
	// What the project gets from this machine without declaring it runs locally
	// and is carried by no artifact, so every target says so beside its own
	// warnings. Best effort: a listing that cannot be read costs only the note.
	var undeclared []string
	if names, uerr := plan.UndeclaredLocal(dir, m); uerr == nil {
		undeclared = names
	}
	note := plan.UndeclaredNote(undeclared, m.Astro.Workspace)
	if note != "" {
		res.Warnings = append(res.Warnings, note)
	}
	return r.Emit(res, func(w io.Writer) error {
		if err := renderPackage(w, res, m.Astro.Deployments); err != nil {
			return err
		}
		// A tree result prints its Warnings itself. An image target's own
		// warnings were streamed as the build ran, so only the note is left.
		if res.Kind == pack.KindImage && note != "" {
			return renderWarnings(w, []string{note})
		}
		return nil
	})
}

// packageRegistry builds the target registry with the production astro target:
// the shared image builder and a container command runner, both backed by
// os/exec, driving the engine container.binary picks for the project.
func (c *cli) packageRegistry() *pack.Registry {
	cmd := imagebuild.NewExecCommander()
	astro := pack.NewAstroTarget(imagebuild.New(cmd, time.Now), cmd, containercfg.Engine)
	return pack.NewRegistry(astro)
}

// renderPackage renders a finished build as text. json mode serializes the
// Result directly, so this runs only for text. Each artifact shape has its own
// renderer. links are the project's deployment links, for the deploy command
// an image hands off to.
func renderPackage(w io.Writer, res pack.Result, links map[string]manifest.Link) error {
	if _, err := fmt.Fprintf(w, "target: %s\n", res.Target); err != nil {
		return err
	}
	switch res.Kind {
	case pack.KindImage:
		return renderImageResult(w, res, links)
	case pack.KindTree:
		return renderTreeResult(w, res)
	case pack.KindBundle:
		_, err := fmt.Fprintf(w, "bundle: %s\n", res.BundlePath)
		return err
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
	_, err := fmt.Fprintf(w, "\nDeploy it with:\n%s\n", deployImageHint(res.Image, links))
	return err
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
	if err := renderWarnings(w, res.Warnings); err != nil {
		return err
	}
	return renderNextSteps(w, res.NextSteps)
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

// renderWarnings prints any non-fatal findings under a heading. The build still
// produced a valid artifact, so these inform rather than stop.
func renderWarnings(w io.Writer, warnings []string) error {
	if len(warnings) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nwarnings:\n"); err != nil {
		return err
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(w, "  - %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}
