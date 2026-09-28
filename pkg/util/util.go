package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lucsky/cuid"
	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/astroauth"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

type CustomClaims struct {
	OrgAuthServiceID      string   `json:"org_id"`
	Scope                 string   `json:"scope"`
	Permissions           []string `json:"permissions"`
	Version               string   `json:"version"`
	IsAstronomerGenerated bool     `json:"isAstronomerGenerated"`
	RsaKeyID              string   `json:"kid"`
	APITokenID            string   `json:"apiTokenId"`
	jwt.RegisteredClaims
}

func Contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

// exists returns whether the given file or directory exists
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Base64URLEncode delegates to astroauth.Base64URLEncode.
// See https://datatracker.ietf.org/doc/html/rfc4648#section-5
func Base64URLEncode(arg []byte) string {
	return astroauth.Base64URLEncode(arg)
}

func CheckEnvBool(envBool string) bool {
	switch strings.ToLower(envBool) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func ParseAPIToken(astroAPIToken string) (*CustomClaims, error) {
	// Parse the token to peek at the custom claims
	jwtParser := jwt.NewParser()
	parsedToken, _, err := jwtParser.ParseUnverified(astroAPIToken, &CustomClaims{})
	if err != nil {
		return nil, errors.Wrap(err, "token is invalid or malformed")
	}
	claims, ok := parsedToken.Claims.(*CustomClaims)
	if !ok {
		return nil, errors.Wrap(err, "failed to parse auth token")
	}
	return claims, nil
}

// BuildSecretInputEnv holds newline-separated build secret specs, read when
// no --build-secret is given.
const BuildSecretInputEnv = "BUILD_SECRET_INPUT"

// BuildSecretUsage is the help text of every --build-secret flag.
const BuildSecretUsage = "Secret to expose to the build. See https://docs.docker.com/build/building/secrets/. Repeat to specify multiple secrets. (format: \"id=mysecret[,src=/local/secret]\" or \"id=mysecret,env=ENV_VAR\")"

// ErrBuildSecretNeedsDockerfile refuses a --build-secret other than netrc for
// a project whose image is generated rather than built from its own
// Dockerfile.
var ErrBuildSecretNeedsDockerfile = errors.New("a generated image reads only the netrc build secret, which the runtime image mounts as /root/.netrc while it installs your requirements. To use another secret, declare a Dockerfile with `dockerfile` under [tool.astro] in pyproject.toml and mount the secret in a RUN step")

// CheckGeneratedBuildSecrets refuses a --build-secret that a generated
// image's build cannot read: a spec that does not parse, and any id but
// manifest.RuntimeSecretID.
func CheckGeneratedBuildSecrets(specs []string) error {
	for _, spec := range specs {
		s, err := manifest.ParseBuildSecret(spec)
		if err != nil {
			return fmt.Errorf("a build secret spec %w", err)
		}
		if s.ID != manifest.RuntimeSecretID {
			return ErrBuildSecretNeedsDockerfile
		}
	}
	return nil
}

// ResolveBuildSecrets returns the Docker build secrets to use: the secrets
// given on the command line if there are any, otherwise the first fallback
// that provides any. Each element is one complete `docker build --secret`
// specification; fallbacks may be newline-delimited to provide multiple
// secrets.
func ResolveBuildSecrets(flagSecrets []string, fallbacks ...string) []string {
	if len(flagSecrets) > 0 {
		return flagSecrets
	}
	for _, fb := range fallbacks {
		if secrets := splitBuildSecretLines(fb); len(secrets) > 0 {
			return secrets
		}
	}
	return nil
}

// ResolveProjectBuildSecrets returns the build secrets for a build of a v2
// project's own Dockerfile: the --build-secret flags, else BUILD_SECRET_INPUT,
// else declared, the project's [tool.astro] build-secrets (Astro.BuildSecretSpecs). Each source replaces the ones
// after it rather than adding to them.
func ResolveProjectBuildSecrets(flagSecrets, declared []string) []string {
	if secrets := ResolveBuildSecrets(flagSecrets, os.Getenv(BuildSecretInputEnv)); len(secrets) > 0 {
		return secrets
	}
	return declared
}

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
		warnings = append(warnings, fmt.Sprintf("%s mounts build secret %q (line %d) but none was given; pass %s or set %s",
			m.Dockerfile, mount.ID, mount.Line, buildSecretFlag(mount.ID), BuildSecretInputEnv))
	}
	return warnings
}

func buildSecretFlag(id string) string {
	return "--build-secret id=" + id + ",env=<VAR>"
}

// Explain adds the missing secrets to a failed build of the project's
// Dockerfile, where the warning printed before the build has scrolled out of
// sight. Any other error comes back as it is.
func (m MissingSecrets) Explain(err error) error {
	if len(m.Mounts) == 0 || !errors.Is(err, imagebuild.ErrDockerfileBuild) {
		return err
	}
	ids := make([]string, len(m.Mounts))
	flags := make([]string, len(m.Mounts))
	for i, mount := range m.Mounts {
		ids[i] = strconv.Quote(mount.ID)
		flags[i] = buildSecretFlag(mount.ID)
	}
	if len(ids) == 1 {
		return fmt.Errorf("%w — %s mounts build secret %s, which was not given; pass %s or set %s", err, m.Dockerfile, ids[0], flags[0], BuildSecretInputEnv)
	}
	return fmt.Errorf("%w — %s mounts build secrets %s, which were not given; pass %s or set %s", err, m.Dockerfile, strings.Join(ids, ", "), strings.Join(flags, " "), BuildSecretInputEnv)
}

func splitBuildSecretLines(value string) (secrets []string) {
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			secrets = append(secrets, line)
		}
	}
	return secrets
}

func StripOutKeysFromJSONByteArray(jsonData []byte, keys []string) ([]byte, error) {
	var jsonDataStruct map[string]interface{}
	err := json.Unmarshal(jsonData, &jsonDataStruct)
	if err != nil {
		// If not a valid json, return the original data itself
		return jsonData, nil
	}
	for _, key := range keys {
		delete(jsonDataStruct, key)
	}
	resultJSON, _ := json.Marshal(jsonDataStruct) //nolint:errcheck // marshaling a plain struct that does not error in practice
	return resultJSON, nil
}

// Filter returns a new slice holding only the elements of ss that satisfy test.
func Filter[T any](ss []T, test func(T) bool) (ret []T) {
	for _, s := range ss {
		if test(s) {
			ret = append(ret, s)
		}
	}
	return
}

// IsAstronomerRegistry checks if a given registry domain is one of the valid Astronomer registries
func IsAstronomerRegistry(registry string) bool {
	validRegistries := []string{
		"images.astronomer.cloud",
		"images.astronomer-dev.cloud",
		"images.astronomer-stage.cloud",
	}

	for _, validRegistry := range validRegistries {
		if strings.Contains(registry, validRegistry) {
			return true
		}
	}
	return false
}

// IsCUID reports whether s is a syntactically valid CUID
// (c + 24 lowercase alphanumerics).
//
// lucsky/cuid.IsCuid uses an unanchored regex, so it matches a CUID-shaped
// substring anywhere in s. We gate on the exact length so callers passing
// strings that merely contain a CUID don't get false positives.
func IsCUID(s string) bool {
	return len(s) == 25 && cuid.IsCuid(s) == nil
}
