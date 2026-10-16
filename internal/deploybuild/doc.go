// Package deploybuild holds the build half of a manifest deploy: the parts
// that do not depend on which control plane the image ships to. They are the
// checks on what the image is built from (CheckSource), the container engine
// check (EnsureEngine), the build at the platform a Deployment runs and the
// read of the built image's runtime label (BuildImage, PrebuiltRuntime), and
// the runtime rules a deploy's image is held to before and after the build
// (PlanRuntime).
//
// The Astro manifest deploy (internal/platform/astro/deploy) calls it, and an
// Astro Private Cloud one is meant to, so both build and judge an image by the
// same rules. Each platform supplies what only it knows: the runtime versions
// its Deployment accepts, its Deployment's current version, and how it reads
// an image's labels.
//
// The package keeps to the core's layer rules (docs/architecture.md): it never
// prints, never exits, and reads no config. It does not import
// internal/platform: a platform imports it.
package deploybuild
