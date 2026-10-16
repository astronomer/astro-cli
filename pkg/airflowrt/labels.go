package airflowrt

// RuntimeVersionLabel is the label an Astro Runtime image carries its runtime
// version in ("3.1-2", "12.1.0"). A deploy reads it off the image it ships,
// and Astro and Astro Private Cloud read it off the image they receive. It is
// the one spelling of the label in the CLI.
const RuntimeVersionLabel = "io.astronomer.docker.runtime.version"
