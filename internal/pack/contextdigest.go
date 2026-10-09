package pack

import (
	"crypto/sha256"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/shipcontext"
)

// contextDigest is a digest of what a generated build copies from the project
// at dir into the image: every path the build's ignore file
// (imagebuild.ProjectIgnore) leaves in, with its kind, whether it is
// executable, and its bytes or link text (shipcontext.Take, one walk). It
// also returns what the package warns about those files: no DAG file reaching
// the image, and files git ignores that the image will carry; and it refuses
// gitignored files that look like credentials (shipcontext.SecretsError).
func contextDigest(dir string) (digest string, warnings []string, err error) {
	ignore, err := imagebuild.ProjectIgnore(dir, nil)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	survey, err := shipcontext.Take(dir, shipcontext.Options{Ignore: ignore, Digest: h, Git: true})
	if err != nil {
		return "", nil, err
	}
	if err := shipcontext.SecretsError(survey.Secrets); err != nil {
		return "", nil, err
	}
	if survey.DagsOnDisk > 0 && survey.DagsShipped == 0 {
		warnings = append(warnings, dagsIgnoredWarning)
	}
	if w := imagebuild.GitignoredWarning(survey.Gitignored); w != "" {
		warnings = append(warnings, w)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), warnings, nil
}
