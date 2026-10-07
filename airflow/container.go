package airflow

import (
	"crypto/md5" //nolint:gosec // reviewed; not a new risk in this shell code
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
)

// DAGCheck checks a project's DAGs in its image, the parse and
// pytest steps of `astro deploy`.
type DAGCheck interface {
	Pytest(pytestFile, customImageName, deployImageName, pytestArgsString string, buildSecrets []string) (string, error)
	Parse(customImageName, deployImageName string, buildSecrets []string) error
}

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(dockerfile string, buildSecrets []string, config types.ImageBuildConfig) error
	Push(remoteImage, username, token string, getImageRepoSha bool) (string, error)
	GetLabel(altImageName, labelName string) (string, error)
	TagLocalImage(localImage string) error
	Pytest(pytestFile, airflowHome, envFile, testHomeDirectory string, pytestArgs []string, htmlReport bool, config types.ImageBuildConfig) (string, error)
}

type DockerRegistryAPI interface {
	client.APIClient
}

func DAGCheckInit(airflowHome, envFile, dockerfile, projectName string) (DAGCheck, error) {
	return NewDAGChecker(airflowHome, envFile, dockerfile, projectName)
}

func RegistryHandlerInit(registry string) (RegistryHandler, error) {
	return DockerRegistryInit(registry)
}

func ImageHandlerInit(image string) ImageHandler {
	return DockerImageInit(image)
}

// ProjectNameUnique creates a reasonably unique project name based on the hashed
// path of the project. This prevents collisions of projects with identical dir names
// in different paths. ie (~/dev/project1 vs ~/prod/project1)
func ProjectNameUnique() (string, error) {
	projectName := config.CFG.ProjectName.GetString()

	pwd, err := fileutil.GetWorkingDir()
	if err != nil {
		return "", errors.Wrap(err, "error retrieving working directory")
	}

	// #nosec
	b := md5.Sum([]byte(pwd))
	s := fmt.Sprintf("%x", b[:])

	return sanitizeImageName(projectName + "_" + s[0:6]), nil
}

// validImageName matches docker's grammar for a single image-name path
// component: alphanumeric runs joined by single "." / "_" / "-" separators (or
// a double "__"), starting and ending with an alphanumeric.
var validImageName = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// sanitizeImageName turns an arbitrary project name into a name docker will
// accept as an image tag. Names docker already accepts are returned unchanged,
// so cached image tags keep their names. Anything else is lowercased, has every
// run of non-alphanumeric characters collapsed to a single "-", and its leading
// and trailing separators trimmed. The result is always a non-empty valid name.
func sanitizeImageName(s string) string {
	s = strings.ToLower(s)
	if validImageName.MatchString(s) {
		return s
	}

	var b strings.Builder
	prevSep := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSep = false
		} else if !prevSep {
			b.WriteByte('-')
			prevSep = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	return out
}
