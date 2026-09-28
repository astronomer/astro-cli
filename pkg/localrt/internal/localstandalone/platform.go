//go:build !windows

package localstandalone

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// ErrPlatformExcluded reports a project whose [tool.uv] environments leave out
// this machine's platform, so uv refuses to sync its environment here. Intel
// macOS is the usual one: `astro init` writes environments for Linux and Apple
// silicon macOS only.
var ErrPlatformExcluded = errors.New("this project's [tool.uv] environments leave out this machine")

// checkPlatform refuses, before uv runs, a project whose environments do not
// cover goos and goarch. uv's own refusal names neither the setting nor what
// to do instead.
//
// Only markers of the shape `astro init` writes are read: `and`s of
// sys_platform and platform_machine compared with == or !=. An environment it
// cannot read counts as covering the machine, so the check never refuses what
// uv would allow.
func checkPlatform(projectPath, goos, goarch string) error {
	m, err := manifest.Load(filepath.Join(projectPath, manifest.Marker))
	if err != nil || len(m.UV.Environments) == 0 {
		return nil
	}
	facts := map[string]string{"sys_platform": goos, "platform_machine": machine(goos, goarch)}
	for _, env := range m.UV.Environments {
		if covers(env, facts) {
			return nil
		}
	}
	name := goos + " on " + facts["platform_machine"]
	if goos == goosDarwin && goarch == goarchIntel {
		name = "Intel macOS"
	}
	return fmt.Errorf("%w (%s): uv will not build its environment here. Run it in Docker mode with `astro local start --docker`, "+
		"or add \"sys_platform == '%s' and platform_machine == '%s'\" to [tool.uv] environments in %s",
		ErrPlatformExcluded, name, goos, facts["platform_machine"], manifest.Marker)
}

// goarchIntel is Go's name for x86_64.
const goarchIntel = "amd64"

// machine is Python's platform.machine() for a Go architecture.
func machine(goos, goarch string) string {
	switch {
	case goarch == goarchIntel:
		return "x86_64"
	case goarch == "arm64" && goos == "linux":
		return "aarch64"
	}
	return goarch
}

// covers reports whether a marker holds for facts, and true for one it cannot
// read.
func covers(marker string, facts map[string]string) bool {
	for _, clause := range strings.Split(marker, " and ") {
		name, op, value, ok := comparison(clause)
		fact, known := facts[name]
		if !ok || !known {
			return true
		}
		if (op == "==") != (fact == value) {
			return false
		}
	}
	return true
}

func comparison(clause string) (name, op, value string, ok bool) {
	for _, op := range []string{"==", "!="} {
		if l, r, found := strings.Cut(clause, op); found {
			value = strings.Trim(strings.TrimSpace(r), `'"`)
			return strings.TrimSpace(l), op, value, !strings.ContainsAny(value, " ()")
		}
	}
	return "", "", "", false
}
