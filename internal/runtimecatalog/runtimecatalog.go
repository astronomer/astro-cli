// Package runtimecatalog is how this CLI reads Astronomer's runtime catalog
// (pkg/runtimeversions): where the copy is cached and what the request says
// about who is asking. One place, because astro init, the Docker-mode image
// lookup, and the runtime-build check that start, deploy and package run all
// read the same catalog, and should read it the same way.
package runtimecatalog

import (
	"context"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/version"
)

// Options is how this CLI reads the catalog: cached in runtime-versions.json
// under the cache root it shares with Astro Desktop, so one fetch warms both,
// and named in the request as astro-cli/<version>. The User-Agent is the only
// thing the request says about who is asking.
//
// A cache root that cannot be resolved is not a failure: an empty directory
// means every lookup asks the catalog, which still works. A zero timeout means
// the catalog's default.
func Options(timeout time.Duration) runtimeversions.Options {
	dir, err := localrt.CacheRoot()
	if err != nil {
		dir = ""
	}
	return runtimeversions.Options{
		CacheDir:  dir,
		Timeout:   timeout,
		UserAgent: "astro-cli/" + version.Current(),
	}
}

// CheckRuntime checks a manifest's [tool.astro] runtime against its Airflow
// pin with the catalog, as runtimeversions.CheckRuntime does, at the default
// timeout: whatever calls it is about to build an image, which takes longer
// than the wait. It never fails for want of the catalog, and an empty runtime
// costs nothing.
func CheckRuntime(ctx context.Context, runtime, airflowPin string) ([]runtimeversions.Finding, error) {
	return runtimeversions.CheckRuntime(ctx, Options(0), runtime, airflowPin)
}
