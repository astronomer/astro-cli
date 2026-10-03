package connwarehouse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// The analyzing-data skill reads its config from ~/.astro/agents (config.py
// get_config_dir). We own a slice of the warehouse.yml there: warehouse
// entries keyed by name, ours prefixed "airflow_" (namePrefix). Any other entry
// is the user's and is preserved.
//
// The secrets our "${VAR}" refs name are not written anywhere. The skill
// resolves a ref from the process environment (connectors.py
// substitute_env_vars), and it loads ~/.astro/agents/.env without override, so
// a value Otto's environment carries wins over the file. AppendEnv puts the
// values into the environment Otto is started with, where its kernel inherits
// them. The .env file is the user's alone: earlier versions wrote the secrets
// there, as a banner followed by the managed assignments (keys prefixed
// "AIRFLOW_", managedEnvPrefix), so Write and ScrubEnv strip that block and
// nothing else.
const managedEnvPrefix = "AIRFLOW_"

const (
	// dirPerm is the config directory's mode when this creates it.
	dirPerm = 0o750
	// filePerm is both files' mode: warehouse.yml names the accounts the
	// secrets unlock, and .env holds the user's own secrets.
	filePerm = 0o600
	// maxEnvLine bounds one .env line, which may hold a whole PEM key or URL.
	maxEnvLine = 1 << 20
	// envReadBuf is the scanner's initial buffer.
	envReadBuf = 64 << 10
)

const yamlHeader = "# Partly managed by Astro: entries prefixed \"airflow_\" are generated\n" +
	"# from your Airflow connections and rewritten on change. Other entries are yours.\n"

// envBanner and legacyEnvBanner head the block of secrets earlier versions
// wrote to .env: the first by this package, the second by Astro Desktop
// before this package was shared. Both are stripped with the block.
const (
	envBanner       = "# --- Astro: Airflow connection secrets (regenerated on change; do not edit) ---"
	legacyEnvBanner = "# --- Astro Desktop: Airflow connection secrets (regenerated on change; do not edit) ---"
)

// ConfigDir returns the analyzing-data skill's config directory, ~/.astro/agents.
// This is the skill's own config root (config.py), so it is constructed here
// rather than taken from either tool's own config directory.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".astro", "agents"), nil
}

// Write merges the live warehouses into warehouse.yml under dir, replacing
// the managed entries (keys prefixed "airflow_") and preserving everything
// else, and strips any managed secret an earlier version left in .env (see
// ScrubEnv). It writes no secret: the entries' "${VAR}" refs resolve from the
// environment AppendEnv builds for Otto. Writes are atomic (pkg/fsatomic) and
// 0600.
//
// The .env scrub goes first and a failure there does not stop the
// warehouse.yml write, which carries no secret: both errors are returned.
func Write(dir string, live []Materialized) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	scrubErr := ScrubEnv(dir)
	return errors.Join(scrubErr, writeWarehouseYAML(filepath.Join(dir, "warehouse.yml"), live))
}

// ScrubEnv removes the managed secrets an earlier version wrote (a banner and
// the AIRFLOW_* assignments directly below it) from the .env under dir,
// keeping the user's lines, AIRFLOW_* ones outside that block included. A file left
// with nothing is removed; a missing file is not an error. Callers run it to
// clean up the plaintext an earlier version wrote, even when they write no
// warehouses.
func ScrubEnv(dir string) error {
	path := filepath.Join(dir, ".env")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	preserved, changed, err := envPreserving(b)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !changed {
		return nil // nothing of ours: leave the user's file byte for byte
	}
	if len(preserved) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return writeFile(path, []byte(strings.Join(preserved, "\n")+"\n"), filePerm)
}

// AppendEnv returns env with the secret values live's "${VAR}" refs name
// appended as KEY=VALUE, sorted by key, for the environment Otto is started
// with. A key env already sets keeps its value, as the skill's own .env load
// (python-dotenv without override) lets the process environment win; it also
// keeps a launcher's own AIRFLOW_* settings (AIRFLOW_API_URL, say) from being
// replaced by a connection whose id happens to produce the same name. Keys
// compare without case on Windows, where the environment does.
func AppendEnv(env []string, live []Materialized) []string {
	set := make(map[string]bool, len(env))
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok {
			set[envKey(k)] = true
		}
	}
	managed := map[string]string{}
	for _, m := range live {
		for k, v := range m.Env {
			managed[k] = v
		}
	}
	keys := make([]string, 0, len(managed))
	for k := range managed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := slices.Clip(env)
	for _, k := range keys {
		if set[envKey(k)] {
			continue
		}
		out = append(out, k+"="+managed[k])
	}
	return out
}

// isWindows reports whether this is a Windows build.
const isWindows = runtime.GOOS == "windows"

// foldEnvCase reports whether environment keys compare without case, as they
// do on Windows. A var so a test can exercise both rules on any platform.
var foldEnvCase = isWindows

// envKey is an environment key in the form two keys compare equal in.
func envKey(k string) string {
	if foldEnvCase {
		return strings.ToUpper(k)
	}
	return k
}

// writeFile is fsatomic.WriteFile, a var so a test can fail one write.
var writeFile = fsatomic.WriteFile

func writeWarehouseYAML(path string, live []Materialized) error {
	data, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	// Drop our previously-managed entries, then add the current ones. Anything
	// not prefixed namePrefix is user-authored and left untouched.
	for k := range data {
		if strings.HasPrefix(k, namePrefix) {
			delete(data, k)
		}
	}
	for _, m := range live {
		data[m.Name] = m.Config
	}

	// Nothing left to persist: remove the file rather than leave an empty map.
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}

	body, err := yaml.Marshal(data) // yaml.v3 emits map keys sorted → deterministic
	if err != nil {
		return fmt.Errorf("marshal warehouse.yml: %w", err)
	}
	return writeFile(path, []byte(yamlHeader+string(body)), filePerm)
}

func readYAMLMap(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	data := map[string]any{}
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

// envPreserving returns the lines of a .env to keep: everything except our
// banners and the managed assignments (AIRFLOW_*=...) that run directly below
// one, which is the block earlier versions wrote. User comments, blank lines
// and other assignments, an AIRFLOW_* one elsewhere included, are kept
// verbatim; trailing blank lines are trimmed. changed reports whether anything
// of ours was dropped.
func envPreserving(b []byte) (out []string, changed bool, err error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, envReadBuf), maxEnvLine) // tolerate long key/url lines
	inBlock := false
	for sc.Scan() {
		line := sc.Text()
		if line == envBanner || line == legacyEnvBanner {
			changed, inBlock = true, true
			continue
		}
		if inBlock {
			if key, ok := assignmentKey(line); ok && strings.HasPrefix(key, managedEnvPrefix) {
				continue
			}
			inBlock = false
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, false, err
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out, changed, nil
}

// assignmentKey returns the KEY of a "KEY=value" line (ignoring leading
// `export ` and surrounding spaces), and whether the line is an assignment.
func assignmentKey(line string) (string, bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", false
	}
	s = strings.TrimPrefix(s, "export ")
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return "", false
	}
	return strings.TrimSpace(s[:eq]), true
}
