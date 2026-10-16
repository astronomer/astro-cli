package airflowrt

import "testing"

// The reason is plain text that names the image and the label it lacks, with
// no markup a terminal or Astro Desktop would show literally.
func TestNotRuntimeImageError(t *testing.T) {
	err := &NotRuntimeImageError{Image: "python:3.12-slim"}
	const want = "python:3.12-slim is not an Astro Runtime image: it has no runtime version label (io.astronomer.docker.runtime.version)"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
