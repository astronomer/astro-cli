package connwarehouse

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// The analyzing-data skill reads its config from ~/.astro/agents (config.py
// get_config_dir). We own a slice of two shared files there:
//   - warehouse.yml: warehouse entries keyed by name. Ours are prefixed
//     "airflow_" (namePrefix); any other entry is the user's and is preserved.
//   - .env: the secret values our "${VAR}" refs resolve to. Ours are prefixed
//     "AIRFLOW_" (managedEnvPrefix); other lines are preserved.
const managedEnvPrefix = "AIRFLOW_"

const (
	// dirPerm is the config directory's mode when this creates it.
	dirPerm = 0o750
	// filePerm is both files' mode: .env holds secrets, and warehouse.yml
	// names the accounts they unlock.
	filePerm = 0o600
	// maxEnvLine bounds one .env line, which may hold a whole PEM key or URL.
	maxEnvLine = 1 << 20
	// envReadBuf is the scanner's initial buffer.
	envReadBuf = 64 << 10
)

const yamlHeader = "# Partly managed by Astro: entries prefixed \"airflow_\" are generated\n" +
	"# from your Airflow connections and rewritten on change. Other entries are yours.\n"

const envBanner = "# --- Astro: Airflow connection secrets (regenerated on change; do not edit) ---"

// legacyEnvBanner is the banner Astro Desktop wrote before this package was
// shared. Recognized so a file it wrote does not keep a stray copy above ours.
const legacyEnvBanner = "# --- Astro Desktop: Airflow connection secrets (regenerated on change; do not edit) ---"

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

// Write merges the live warehouses into .env and warehouse.yml under dir,
// replacing the managed entries (env keys prefixed "AIRFLOW_", warehouse keys
// prefixed "airflow_") and preserving everything else. Writes are atomic
// (pkg/fsatomic), and both files are 0600: .env holds secrets.
//
// .env goes first. If it cannot be written, the managed lines already in it
// are stripped as a best effort and warehouse.yml is left alone: the old
// secrets may belong to connections that no longer reach the checkout, and a
// plaintext credential outliving its reach is the failure worth avoiding. A
// warehouse.yml entry whose secret is gone only fails to connect.
func Write(dir string, live []Materialized) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	envPath := filepath.Join(dir, ".env")
	if err := writeEnv(envPath, live); err != nil {
		_ = writeEnv(envPath, nil) //nolint:errcheck // best effort; the write error is what the caller needs
		return err
	}
	return writeWarehouseYAML(filepath.Join(dir, "warehouse.yml"), live)
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

func writeEnv(path string, live []Materialized) error {
	preserved, err := readEnvPreserving(path)
	if err != nil {
		return err
	}

	// Collect our managed assignments across all live warehouses, sorted for
	// stable output.
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

	var b strings.Builder
	for _, line := range preserved {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(keys) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(envBanner)
		b.WriteByte('\n')
		for _, k := range keys {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(dotenvQuote(managed[k]))
			b.WriteByte('\n')
		}
	}

	if b.Len() == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	return writeFile(path, []byte(b.String()), filePerm)
}

// readEnvPreserving returns the lines of an existing .env to keep: everything
// except our managed assignments (AIRFLOW_*=...) and our banner. User comments,
// blank lines, and other assignments are preserved verbatim. Trailing blank
// lines are trimmed so re-runs don't accumulate them.
func readEnvPreserving(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, envReadBuf), maxEnvLine) // tolerate long key/url lines
	for sc.Scan() {
		line := sc.Text()
		if line == envBanner || line == legacyEnvBanner {
			continue
		}
		if key, ok := assignmentKey(line); ok && strings.HasPrefix(key, managedEnvPrefix) {
			continue // a previously-managed secret; will be rewritten
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out, nil
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

// dotenvQuote wraps a value in double quotes and escapes backslash, double
// quote, and newline as \n. python-dotenv unescapes these inside double quotes,
// so multi-line secrets (e.g. a PEM private key) round-trip correctly.
func dotenvQuote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}
