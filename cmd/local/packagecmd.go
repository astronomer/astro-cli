package local

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// defaultPackagePlatform is the build platform `astro package` targets by
// default: a deployable image runs on Astro's linux/amd64 nodes, not the host.
const defaultPackagePlatform = "linux/amd64"

// NewPackageCmd builds `astro package [target]` for a root to mount. It reads
// the manifest and the project files, needs no account and no deployment link,
// and touches the network only to pull a base image — the CI build stage
// (docs/v2-deploy.md, section 4). Only the astro target builds in the MVP; the
// rest are staged.
func NewPackageCmd(d Deps) *cobra.Command {
	c := &cli{d: d}
	cmd := newPackageCmd(c)
	addOutputFlag(cmd, &c.output)
	markSkipPreRun(cmd)
	return cmd
}

func newPackageCmd(c *cli) *cobra.Command {
	var opts struct {
		save     string
		tag      string
		platform string
		outDir   string
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
				save:     opts.save,
				tag:      opts.tag,
				platform: opts.platform,
				outDir:   opts.outDir,
			})
		},
	}
	cmd.Flags().StringVar(&opts.save, "save", "", "Also write the artifact to this path (a .tar for image targets, a .zip for bucket targets)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "Image reference for image targets (default astro-package/<name>:<runtime>-<hash>)")
	cmd.Flags().StringVar(&opts.platform, "platform", defaultPackagePlatform, "Build platform for image targets")
	cmd.Flags().StringVar(&opts.outDir, "out-dir", "", "Artifact directory for bucket targets (default dist/<target>)")
	return cmd
}

// packageOptions carries the flag values into the run function.
type packageOptions struct {
	save     string
	tag      string
	platform string
	outDir   string
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
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return err
	}
	res, err := target.Build(ctx, pack.Request{
		ProjectDir: dir,
		Manifest:   m,
		Save:       opts.save,
		Tag:        opts.tag,
		Platform:   opts.platform,
		OutDir:     opts.outDir,
	}, c.callbacks(r))
	if err != nil {
		return err
	}
	return r.Emit(res, func(w io.Writer) error {
		return renderPackage(w, res)
	})
}

// packageRegistry builds the target registry with the production astro target:
// the shared image builder and a docker command runner, both backed by os/exec.
func (c *cli) packageRegistry() *pack.Registry {
	cmd := imagebuild.NewExecCommander()
	astro := pack.NewAstroTarget(imagebuild.New(cmd, time.Now), cmd, "docker", nil)
	return pack.NewRegistry(astro)
}

// renderPackage renders a finished build as text. json mode serializes the
// Result directly, so this runs only for text. Each artifact shape has its own
// renderer.
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
	}
	return nil
}

// renderImageResult renders an image target's tag, version, and the deploy
// command that consumes it, so the CI story is one copy-paste.
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
	_, err := fmt.Fprintf(w, "\nDeploy it with:\n  astro deploy --image-name %s\n", res.Image)
	return err
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
