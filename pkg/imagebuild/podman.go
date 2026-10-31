package imagebuild

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"time"

	rt "github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// podmanName is podman's binary name, and the word its --version prints.
const podmanName = "podman"

// IsPodman reports whether the engine CLI bin is podman, by asking it through
// cmd: `<bin> --version` names podman whatever the binary is called, so the
// podman-docker shim and a docker alias for podman count too. env is the
// connection env to run it with. An engine that does not answer is podman
// when its binary is named podman; the caller's own probe reports it.
//
// It decides which ignore file a declared Dockerfile's build reads
// (SurveyProject): podman and buildah read a .containerignore first.
func IsPodman(ctx context.Context, cmd Commander, bin string, env []string) bool {
	if bin == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := cmd.Run(ctx, env, rt.Stdio{Out: &out}, bin, "--version"); err != nil {
		name := strings.TrimSuffix(strings.ToLower(filepath.Base(bin)), ".exe")
		return name == podmanName
	}
	return strings.Contains(strings.ToLower(out.String()), podmanName)
}
