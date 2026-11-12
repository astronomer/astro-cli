package airflowrt

import (
	"strings"
)

// A project's own Dockerfile builds on the install base by naming it in its
// final stage: FROM localhost/astro-base (InstallBaseRepo), the same line in
// every project. The install base is the runtime's -base image with the
// project's packages installed from pyproject.toml and uv.lock, which the CLI
// builds and tags locally. localhost/ names no registry, so a build without
// the base fails rather than pulling someone else's image.

// InstallBaseRepo is the repository the install base is tagged under:
// localhost/astro-base:<digest> by every build, and plain localhost/astro-base
// by `astro package --base`, for a hand-run `docker build` of the project.
const InstallBaseRepo = "localhost/astro-base"

// The labels an install base carries, which an image built on it inherits,
// so a deploy of a prebuilt image can tell what it was built on: the project
// it was built for ([project] name), the digest of what it installs
// (imagebuild.InstallDigest), and the runtime image it was built over.
const (
	InstallBaseProjectLabel = "io.astronomer.install-base.project"
	InstallBaseDigestLabel  = "io.astronomer.install-base.digest"
	InstallBaseFromLabel    = "io.astronomer.install-base.from"
)

// InstallBaseRef is the install base a Dockerfile, data, builds its final
// stage on, as written (localhost/astro-base, with or without a tag), and
// whether it builds on it at all. A build of such a file has to build the
// base first, and a build of any other Dockerfile has no use for one.
func InstallBaseRef(data []byte) (string, bool) {
	ref, ok := finalRef(data)
	if !ok || strings.Contains(ref, "@") {
		return "", false
	}
	if image, _ := splitImageRef(ref); image != InstallBaseRepo {
		return "", false
	}
	return ref, true
}

// UsesInstallBase reports whether a Dockerfile, data, builds its final stage on
// the install base (InstallBaseRef).
func UsesInstallBase(data []byte) bool {
	_, ok := InstallBaseRef(data)
	return ok
}

// ReplaceFrom is a Dockerfile, data, with every FROM line that names from
// naming to instead, flags and stage name kept, and every other line as it
// was: how a build pins the install base a file names (InstallBaseRef) to the
// exact one it built.
func ReplaceFrom(data []byte, from, to string) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		at := 1
		for at < len(fields) && strings.HasPrefix(fields[at], "--") {
			at++
		}
		if at < len(fields) && fields[at] == from {
			fields[at] = to
			lines[i] = strings.Join(fields, " ")
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
