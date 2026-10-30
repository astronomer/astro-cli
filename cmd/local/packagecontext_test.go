package local

import (
	"path/filepath"
	"strings"
	"testing"
)

// --context-dir writes an image's build context, so it is refused where no
// image is built, and beside --save, which has no image to write.
func TestPackageRefusesAContextDirItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "bucket target", args: []string{"package", "mwaa"}, want: "--context-dir writes an image's build context, and the mwaa target builds no image"},
		{name: "with --save", args: []string{"package", "--save", "image.tar"}, want: "nothing for --save to write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			wiringProject(t, &d, "", "")

			err := execute(t, d, append(tc.args, "--context-dir", filepath.Join(t.TempDir(), "build"))...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
