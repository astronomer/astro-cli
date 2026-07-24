package local

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/imagebuild"
	"github.com/astronomer/astro-cli/internal/pack"
	"github.com/astronomer/astro-cli/internal/project"
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
	}
	cmd := &cobra.Command{
		Use:   "package [target]",
		Short: "Build the deployable artifact for an Airflow platform without shipping it",
		Long: "Build the artifact a given Airflow platform consumes — for CI, or to hand a\n" +
			"prebuilt image to `astro deploy --image-name`. TARGET is astro (the default),\n" +
			"mwaa, composer, or oss; only astro builds today.",
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
			})
		},
	}
	cmd.Flags().StringVar(&opts.save, "save", "", "Also write the artifact to this path (a .tar for image targets)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "Image reference for image targets (default astro-package/<name>:<runtime>-<hash>)")
	cmd.Flags().StringVar(&opts.platform, "platform", defaultPackagePlatform, "Build platform for image targets")
	return cmd
}

// packageOptions carries the flag values into the run function.
type packageOptions struct {
	save     string
	tag      string
	platform string
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
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	if err != nil {
		return err
	}
	res, err := target.Build(ctx, pack.Request{
		ProjectDir: dir,
		Manifest:   m,
		Save:       opts.save,
		Tag:        opts.tag,
		Platform:   opts.platform,
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
// Result directly, so this runs only for text.
func renderPackage(w io.Writer, res pack.Result) error {
	if _, err := fmt.Fprintf(w, "target: %s\n", res.Target); err != nil {
		return err
	}
	switch res.Kind {
	case pack.KindImage:
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
		// Point at the seam that consumes it, so the CI story is one copy-paste.
		if _, err := fmt.Fprintf(w, "\nDeploy it with:\n  astro deploy --image-name %s\n", res.Image); err != nil {
			return err
		}
	case pack.KindTree:
		if _, err := fmt.Fprintf(w, "tree:   %s\n", res.TreePath); err != nil {
			return err
		}
	case pack.KindBundle:
		if _, err := fmt.Fprintf(w, "bundle: %s\n", res.BundlePath); err != nil {
			return err
		}
	}
	return nil
}
