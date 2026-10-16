package airflowrt

// RuntimeVersionLabel is the label an Astro Runtime image carries its runtime
// version in ("3.1-2", "12.1.0"). A deploy reads it off the image it ships,
// and Astro and Astro Private Cloud read it off the image they receive. It is
// the one spelling of the label in the CLI.
const RuntimeVersionLabel = "io.astronomer.docker.runtime.version"

// NotRuntimeImageError reports an image that carries no RuntimeVersionLabel,
// and so is not an Astro Runtime image, whatever it is called.
//
// The label rather than the name is the test, because the label is what the
// image is: a mirror of a Runtime image under a private registry's name
// (registry.corp.example/astronomer/astro-runtime:12.1.0) is an exact copy and
// keeps it, and an image under an Astronomer name that lacks it is not one.
// Every place that refuses an image for not being Runtime says it with this,
// so the reason reads the same from astro local start and astro deploy.
type NotRuntimeImageError struct {
	// Image names what was judged, the way its reader knows it: the base a
	// Dockerfile's FROM names, or a description such as "the image built from
	// Dockerfile" when the file does not name its base outright.
	Image string
}

func (e *NotRuntimeImageError) Error() string {
	return e.Image + " is not an Astro Runtime image: it has no runtime version label (" + RuntimeVersionLabel + ")"
}
