package localdocker

import (
	"context"
	"crypto/md5" //nolint:gosec // reproduces astro-cli v1's project-name hash; not a security boundary
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Everything in this file exists for one transition: a project that ran under
// `astro dev` before this runtime existed, starting here for the first time.
// It has a death date: when no project has a v1 volume left, the whole file
// goes.
//
// Compose namespaces a named volume as <compose-project>_<volume>, and the two
// tools derive the compose project name differently and can never coincide: v1
// hashes the working directory with md5 behind the configured project name
// (airflow/container.go's ProjectNameUnique), this runtime uses
// composeProjectName. So a project's first start here gets a new, empty
// metadata database, and the v1 one stays exactly where it was, which is where
// Astro CLI v1's `astro dev start` still finds it. v2 removed that command, so
// the note names the tool rather than a command this binary refuses.
//
// That is deliberate. An earlier version copied the v1 database across, and
// every guard it needed (a live v1 stack, the Postgres major, custom
// credentials, an Airflow the copy was too new for, a reset that copied it
// back) was another way to lose or wedge a database that is local dev data.
// What is left is telling the user, once, where their old database is, so a
// fresh one does not read as "my local Airflow lost everything".

// metadataVolumeKey is the volume this runtime's compose spec declares for the
// Postgres data directory, and the one v1's spec declared too. Compose
// namespaces it as <compose-project>_<key>.
//
// It must stay equal to the volume key in compose.yaml.tmpl: the note below
// asks about the name derived from it, and compose mounts the name derived from
// the template. TestMetadataVolumeKeyMatchesComposeTemplate pins them together.
const metadataVolumeKey = "postgres_data"

// v1Config is the part of astro-cli v1's config this reads: the project name,
// which is half of the v1 volume name.
//
// Read directly rather than through astro-cli's config package because a pkg/
// sub-module may not import the parent module (docs/v2-architecture.md), and
// because this is one field of a file that is on its way out.
type v1Config struct {
	Project struct {
		Name string `yaml:"name"`
	} `yaml:"project"`
}

// readV1Config loads a project's config the way v1 resolved it: the project's
// own .astro/config.yaml, falling back to the home config for the name, which
// is what cfg.GetString does.
//
// A project without a config never ran under v1, so an absent file is "nothing
// to say" rather than an error.
func readV1Config(projectPath string) (v1Config, bool) {
	cfg, ok := readV1ConfigFile(filepath.Join(projectPath, ".astro", "config.yaml"))
	if !ok {
		return cfg, false
	}
	if cfg.Project.Name == "" {
		if home, ok := readV1ConfigFile(v1HomeConfigPath()); ok {
			cfg.Project.Name = home.Project.Name
		}
	}
	return cfg, true
}

// v1HomeConfigPath mirrors config.initHome: $ASTRO_HOME/.astro/config.yaml when
// that is set, ~/.astro/config.yaml otherwise.
func v1HomeConfigPath() string {
	base := os.Getenv("ASTRO_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = home
	}
	return filepath.Join(base, ".astro", "config.yaml")
}

func readV1ConfigFile(path string) (v1Config, bool) {
	var cfg v1Config
	if path == "" {
		return cfg, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}

// validV1ImageName matches docker's grammar for a single image-name path
// component, copied from airflow/container.go so this reproduces v1's naming
// exactly rather than approximately.
var validV1ImageName = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// sanitizeV1ImageName is airflow/container.go's sanitizeImageName.
func sanitizeV1ImageName(s string) string {
	s = strings.ToLower(s)
	if validV1ImageName.MatchString(s) {
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

// normalizeV1Name is airflow/container.go's normalizeName, which v1 applied on
// top of the above when it handed the name to compose.
var v1NameRunes = regexp.MustCompile("[a-z0-9_-]")

func normalizeV1Name(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(v1NameRunes.FindAllString(s, -1), "")
	return strings.TrimLeft(s, "_-")
}

// legacyComposeProject is the compose project name `astro dev` would have used
// for this project directory.
//
// v1 hashed os.Getwd() rather than a project path, which is the same thing
// here: its config loader looks for .astro/config.yaml in the working directory
// and does not search upward (config.initProject), so `astro dev start` only
// ever ran from a project root.
func legacyComposeProject(projectPath, projectName string) string {
	sum := md5.Sum([]byte(projectPath)) //nolint:gosec // G401: reproducing v1's hash, not protecting anything
	hash := hex.EncodeToString(sum[:])[:6]
	return normalizeV1Name(sanitizeV1ImageName(projectName + "_" + hash))
}

// legacyMetadataVolume is the volume `astro dev` would have used.
func legacyMetadataVolume(projectPath, projectName string) string {
	return legacyComposeProject(projectPath, projectName) + "_" + metadataVolumeKey
}

// legacyPathSpellings are the spellings of a project path that could have been
// hashed into a v1 volume name.
//
// composeProjectName resolves symlinks (rt.ProjectID -> CanonicalPath), so this
// runtime's name is stable across spellings while v1's is not: v1 hashed
// whatever os.Getwd() returned. A project reached through a symlink therefore
// has its v1 volume under one spelling and may be handed the other here, and a
// name that does not exist is a silent no-op — the worst outcome this file has.
// Both are tried; the second costs one lookup and only on a miss.
func legacyPathSpellings(projectPath string) []string {
	out := []string{projectPath}
	if resolved, err := filepath.EvalSymlinks(projectPath); err == nil && resolved != projectPath {
		out = append(out, resolved)
	}
	return out
}

// volumeLookup is the three-way answer to "does this volume exist". The third
// case is kept apart because the note below speaks only on a definite answer:
// a daemon hiccup read as "absent" would announce a fresh database to a project
// that has had one here all along.
type volumeLookup int

const (
	volumeAbsent volumeLookup = iota
	volumePresent
	volumeUnknown
)

// lookupVolume asks one engine for a volume by exact name.
//
// `volume ls --filter` rather than `volume inspect`: inspect exits non-zero for
// both "no such volume" and "cannot reach the daemon", and Output discards
// stderr, so the two are indistinguishable. `ls` exits 0 whenever the daemon
// answered, which makes the distinction structural. The filter is a substring
// match on every engine, so the result is compared exactly rather than trusted.
func (e *Engine) lookupVolume(ctx context.Context, conn engineConn, name string) volumeLookup {
	out, err := e.cmd.Output(ctx, conn.env, conn.bin, "volume", "ls", "--quiet", "--filter", "name="+name)
	if err != nil {
		return volumeUnknown
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return volumePresent
		}
	}
	return volumeAbsent
}

// findLegacyVolume looks for a v1 metadata volume across the engines and path
// spellings it could be under, and reports the engine that has it.
//
// Both engines, because v1 honored container.binary: a podman user's `astro
// dev` volumes live in podman, and e.preferred() may now resolve to docker.
// Asking only the preferred engine would make their database invisible with no
// message at all. Same order findProject uses — preferred first, the other only
// on a miss.
func (e *Engine) findLegacyVolume(ctx context.Context, conn engineConn, projectPath, projectName string) (engineConn, string, volumeLookup) {
	other := binPodman
	if conn.bin == binPodman {
		other = binDocker
	}
	unknown := false
	for i, c := range []engineConn{conn, e.connFor(other)} {
		for _, spelling := range legacyPathSpellings(projectPath) {
			vol := legacyMetadataVolume(spelling, projectName)
			switch e.lookupVolume(ctx, c, vol) {
			case volumePresent:
				return c, vol, volumePresent
			case volumeUnknown:
				// Only the preferred engine's silence is worth reporting: a
				// second engine that is simply not installed errors every time,
				// which is normal and not a reason to warn anybody.
				if i == 0 {
					unknown = true
				}
			case volumeAbsent:
			}
		}
	}
	if unknown {
		return conn, "", volumeUnknown
	}
	return conn, "", volumeAbsent
}

// noteLegacyDatabase tells a project arriving from `astro dev` that it is
// starting on a new metadata database, and where its old one is.
//
// Once: only when this runtime has no volume of its own for the project, which
// is its first start here (and the first after a reset, when "new database" is
// again what is about to happen). A project that never ran under v1 stops at
// the second check, so the common case costs one `volume ls`.
//
// Silent on anything it cannot tell. It changes nothing, so the worst a missed
// note costs is the explanation, and a wrong one would send someone looking for
// a database that is not there.
func (e *Engine) noteLegacyDatabase(ctx context.Context, conn engineConn, projectPath, composeProject string, cb rt.Callbacks) {
	if cb.OnLine == nil {
		return
	}
	if e.lookupVolume(ctx, conn, composeProject+"_"+metadataVolumeKey) != volumeAbsent {
		return
	}
	cfg, ok := readV1Config(projectPath)
	if !ok || cfg.Project.Name == "" {
		return
	}
	legacyConn, legacy, found := e.findLegacyVolume(ctx, conn, projectPath, cfg.Project.Name)
	if found != volumePresent {
		return
	}
	cb.OnLine(rt.LogLine{
		Component: "system",
		Time:      e.now(),
		Text: fmt.Sprintf("starting with a new local Airflow database. The one `astro dev` used is left untouched in the %s volume %s, "+
			"and Astro CLI v1 can still start it", legacyConn.bin, legacy),
	})
}
