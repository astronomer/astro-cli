package deploybuild

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// labelsOf is a LabelReader over one image's labels.
func labelsOf(labels map[string]string) LabelReader {
	return func(_, label string) (string, error) { return labels[label], nil }
}

// resolveTo resolves a project to lists installing pandas at version.
func resolveTo(version string) func(context.Context, *imagebuild.Request) error {
	return func(_ context.Context, req *imagebuild.Request) error {
		req.Install = &imagebuild.InstallLists{Requirements: []string{"pandas==" + version}}
		return nil
	}
}

// A prebuilt image built on an install base is held to the project as it
// stands: built for another project, or on a base the lock has since moved
// past, it is refused, never naming the other project. Images without the
// labels, and projects that cannot be resolved again, go through as before.
func TestCheckInstallBase(t *testing.T) {
	const from = "astrocrpublic.azurecr.io/runtime:3.3-11"
	built := imagebuild.Request{BaseImage: from}
	require.NoError(t, resolveTo("2.2.3")(t.Context(), &built))
	labels := map[string]string{
		airflowrt.InstallBaseProjectLabel: "orders",
		airflowrt.InstallBaseDigestLabel:  built.InstallDigest(),
		airflowrt.InstallBaseFromLabel:    from,
	}
	m := &imagebuild.ManifestBuild{ProjectDir: t.TempDir(), Name: "orders"}

	require.NoError(t, CheckInstallBase(t.Context(), "img", labelsOf(labels), m, resolveTo("2.2.3")))

	err := CheckInstallBase(t.Context(), "img", labelsOf(labels), m, resolveTo("2.2.4"))
	require.EqualError(t, err, "this image was built on an install base that no longer matches uv.lock. Rebuild it with: astro package --base && docker build .")

	other := &imagebuild.ManifestBuild{ProjectDir: t.TempDir(), Name: "billing"}
	err = CheckInstallBase(t.Context(), "img", labelsOf(labels), other, resolveTo("2.2.3"))
	require.EqualError(t, err, "this image was built on an install base for a different project. Rebuild it with: astro package --base && docker build .")
	assert.NotContains(t, err.Error(), "orders")

	assert.NoError(t, CheckInstallBase(t.Context(), "img", labelsOf(nil), m, resolveTo("2.2.4")), "no labels, no check")
	assert.NoError(t, CheckInstallBase(t.Context(), "img", labelsOf(labels), &imagebuild.ManifestBuild{}, resolveTo("2.2.4")), "no project")
	unresolved := func(context.Context, *imagebuild.Request) error { return nil }
	assert.NoError(t, CheckInstallBase(t.Context(), "img", labelsOf(labels), m, unresolved), "no lock to compare")
}
