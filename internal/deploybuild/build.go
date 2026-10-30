package deploybuild

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Platform is the platform a deploy image is built at: the one a Deployment
// runs.
const Platform = "linux/amd64"

// LabelReader reads one label off a local image. It errors when the image is
// not in the local store or cannot be inspected, and returns "" for a label
// the image does not carry.
type LabelReader func(image, label string) (string, error)

// Image is what BuildImage builds, and what it builds it with.
type Image struct {
	// Request is the image the manifest builds (imagebuild.ForManifest's),
	// with the caller's Secrets, Labels and engine (Bin and Env, as
	// EnsureEngine resolved them), which BuildImage passes on as given. It
	// sets only where the image is built and what it is called: WorkDir, Tag
	// and Platform.
	Request imagebuild.Request
	// ProjectDir is the project's root, which names the built tag (ImageTag).
	ProjectDir string
	// Dockerfile is the declared Dockerfile as the manifest names it, for
	// messages; "" for a generated build.
	Dockerfile string
	// Commander runs the build; Now is its clock.
	Commander imagebuild.Commander
	Now       func() time.Time
	// Progress takes the build's output; nil is stderr.
	Progress io.Writer
	// ReadLabel reads the runtime label off the built image.
	ReadLabel LabelReader
	// ResolveInstall resolves what a generated image installs from the
	// project's lock, into the request's Install, before it is built (the
	// caller's imageinstall.Fill, with the deploy's warnings). nil installs
	// the manifest's dependencies as written.
	ResolveInstall func(ctx context.Context, req *imagebuild.Request) error
}

// BuildImage builds the image at Platform and returns its local tag and the
// runtime version its label carries. A built image without the label is
// refused: it is not an Astro Runtime image.
func BuildImage(ctx context.Context, in *Image) (image, runtimeVersion string, err error) {
	workDir, err := os.MkdirTemp("", "astro-deploy-build-*")
	if err != nil {
		return "", "", fmt.Errorf("creating a build directory: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck // best-effort cleanup of a temp dir

	req := in.Request
	if in.ResolveInstall != nil {
		if err := in.ResolveInstall(ctx, &req); err != nil {
			return "", "", err
		}
	}
	req.WorkDir = workDir
	req.Tag = ImageTag(in.ProjectDir)
	req.Platform = Platform
	// BuildLocal, not Build: the image is inspected, tagged and pushed next, so
	// it has to be a single-platform image in the local store even when there
	// is nothing to install. See BuildLocal for why a pulled base is not.
	// The build's own output goes to Progress, as `astro local start` and
	// `astro package` show theirs: a failed dependency install says "see the
	// build output above", so there has to be some.
	progress := in.Progress
	if progress == nil {
		progress = os.Stderr
	}
	built, err := imagebuild.New(in.Commander, in.Now).BuildLocal(ctx, req, localrt.Callbacks{
		OnLine: func(l localrt.LogLine) { fmt.Fprintf(progress, "[%s] %s\n", l.Component, l.Text) },
	})
	if err != nil {
		return "", "", err
	}

	version, err := in.ReadLabel(built, airflowrt.RuntimeVersionLabel)
	if err != nil {
		return "", "", fmt.Errorf("reading the runtime version off the built image: %w", err)
	}
	if version == "" {
		// Two different mistakes, so two different messages. A generated build
		// missing the label means the base we chose is wrong, which is ours. A
		// declared Dockerfile missing it means the user's own FROM is not an
		// Astro Runtime, and naming the base there would name an empty string
		// and send them at a decision they did not make.
		if req.FromDeclaredDockerfile() {
			return "", "", fmt.Errorf("the image built from %s is missing the %s label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image", in.Dockerfile, airflowrt.RuntimeVersionLabel)
		}
		return "", "", fmt.Errorf("the built image is missing the %s label; the runtime base %s should carry it", airflowrt.RuntimeVersionLabel, req.BaseImage)
	}
	return built, version, nil
}

// PrebuiltRuntime reads the runtime version off a prebuilt local image
// (--image-name). A registry image name says nothing about its base, so the
// label is read off the local image: an inspect that fails is the "not found
// locally" signal, and an image without the label is not Astro Runtime.
func PrebuiltRuntime(image string, readLabel LabelReader) (string, error) {
	version, err := readLabel(image, airflowrt.RuntimeVersionLabel)
	if err != nil {
		return "", fmt.Errorf("image %q was not found locally or could not be inspected; build it or 'docker load' it first: %w", image, err)
	}
	if version == "" {
		return "", fmt.Errorf("image %q is not based on Astro Runtime (the %s label is missing); build it FROM the Astro runtime", image, airflowrt.RuntimeVersionLabel)
	}
	return version, nil
}

// ImageTag is the local tag the built deploy image carries. It appends a
// hash of the full project path (same idea as localdocker's composeProjectName),
// so two same-named projects in different directories never share a tag.
func ImageTag(projectDir string) string {
	label := filepath.Base(projectDir)
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = "project"
	}
	if id, err := localrt.ProjectID(projectDir); err == nil && len(id) >= 6 {
		return "astro-deploy/" + label + "-" + id[:6]
	}
	return "astro-deploy/" + label
}
