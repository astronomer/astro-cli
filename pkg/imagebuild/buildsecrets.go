package imagebuild

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// CheckBuildSecrets runs before an image build. It refuses a secret spec that
// names an unset or empty environment variable: Docker would mount nothing for
// it, and the build would fail later, in whatever step read the secret.
// Otherwise it returns MissingBuildSecrets. dockerfile is "" for a generated
// build, which passes on only the manifest.RuntimeSecretID spec: it checks
// that spec and any spec that does not parse, and names the ids it drops.
func CheckBuildSecrets(projectDir, dockerfile string, secrets []string) (MissingSecrets, error) {
	if dockerfile == "" {
		var read []string
		var missing MissingSecrets
		for _, spec := range secrets {
			s, err := manifest.ParseBuildSecret(spec)
			switch {
			case err != nil || s.ID == manifest.RuntimeSecretID:
				read = append(read, spec)
			default:
				missing.Dropped = append(missing.Dropped, s.ID)
			}
		}
		return missing, checkBuildSecretEnv(read)
	}
	if err := checkBuildSecretEnv(secrets); err != nil {
		return MissingSecrets{}, err
	}
	return MissingBuildSecrets(projectDir, dockerfile, secrets), nil
}

// conventionalEnvName is the only shape of env= an error repeats. A token the
// shell put where the name belongs can still be a valid name, but tokens mix
// in lower case where variable names rarely do.
var conventionalEnvName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// checkBuildSecretEnv refuses a spec that does not parse, since docker would
// echo it in its own error, and one whose env= variable is empty or unset.
func checkBuildSecretEnv(secrets []string) error {
	for _, spec := range secrets {
		s, err := manifest.ParseBuildSecret(spec)
		if err != nil {
			return fmt.Errorf("a build secret spec %w", err)
		}
		if s.Env == "" || os.Getenv(s.Env) != "" {
			continue
		}
		variable := "the environment variable its env= names"
		if conventionalEnvName.MatchString(s.Env) {
			variable = "the environment variable " + s.Env
		}
		return fmt.Errorf("build secret %q reads %s, which is empty or not set. Set it before the build, or give the secret another source", s.ID, variable)
	}
	return nil
}

// MissingSecrets are the build secrets a project's Dockerfile mounts that no
// spec supplies.
type MissingSecrets struct {
	Dockerfile string
	Mounts     []airflowrt.SecretMount
	// Dropped are the ids a generated build does not pass on, since the
	// runtime image mounts only manifest.RuntimeSecretID.
	Dropped []string
	// Hint, when set, says how the caller's tool gives the secrets ids name.
	// Warnings and Explain append it after a semicolon. Without it they name
	// no flag or variable, since how a secret is given depends on the tool.
	Hint func(ids []string) string
}

// MissingBuildSecrets finds each secret the project's Dockerfile mounts that
// no spec in secrets supplies. dockerfile is the project-relative path the
// manifest declares; a file that cannot be read finds none, since the build
// reports that itself.
func MissingBuildSecrets(projectDir, dockerfile string, secrets []string) MissingSecrets {
	mounts, err := airflowrt.SecretMounts(filepath.Join(projectDir, dockerfile))
	if err != nil {
		return MissingSecrets{}
	}
	given := map[string]bool{}
	for _, spec := range secrets {
		if s, err := manifest.ParseBuildSecret(spec); err == nil {
			given[s.ID] = true
		}
	}
	missing := MissingSecrets{Dockerfile: dockerfile}
	for _, m := range mounts {
		if !given[m.ID] {
			missing.Mounts = append(missing.Mounts, m)
		}
	}
	return missing
}

// Warnings are the lines a caller prints before the build, one per missing
// secret. A warning and not a refusal, because a secret mount is optional
// unless it says required=true.
func (m MissingSecrets) Warnings() []string {
	var warnings []string
	for _, id := range m.Dropped {
		warnings = append(warnings, fmt.Sprintf("build secret %q is not used: an image built without a Dockerfile reads only the %s secret", id, manifest.RuntimeSecretID))
	}
	for _, mount := range m.Mounts {
		warnings = append(warnings, fmt.Sprintf("%s mounts build secret %q (line %d) but none was given%s",
			m.Dockerfile, mount.ID, mount.Line, m.hint([]string{mount.ID})))
	}
	return warnings
}

func (m MissingSecrets) hint(ids []string) string {
	if m.Hint == nil {
		return ""
	}
	return "; " + m.Hint(ids)
}

// Explain adds the missing secrets to a failed build of the project's
// Dockerfile, where the warning printed before the build has scrolled out of
// sight. Any other error comes back as it is.
func (m MissingSecrets) Explain(err error) error {
	if len(m.Mounts) == 0 || !errors.Is(err, ErrDockerfileBuild) {
		return err
	}
	ids := make([]string, len(m.Mounts))
	quoted := make([]string, len(m.Mounts))
	for i, mount := range m.Mounts {
		ids[i] = mount.ID
		quoted[i] = strconv.Quote(mount.ID)
	}
	if len(ids) == 1 {
		return fmt.Errorf("%w. %s mounts build secret %s, which was not given%s", err, m.Dockerfile, quoted[0], m.hint(ids))
	}
	return fmt.Errorf("%w. %s mounts build secrets %s, which were not given%s", err, m.Dockerfile, strings.Join(quoted, ", "), m.hint(ids))
}
