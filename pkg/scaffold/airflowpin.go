package scaffold

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// ErrInvalidAirflowVersion reports a pin SetAirflowVersionWith refused before
// reading the manifest, because an Airflow requirement cannot pin it.
var ErrInvalidAirflowVersion = errors.New("not an Airflow version")

// AirflowPinChange is what SetAirflowVersionWith did to a manifest.
type AirflowPinChange struct {
	// Previous is the version the manifest's Airflow requirement pinned.
	Previous string `json:"previous"`
	// Version is the pin now written.
	Version string `json:"version"`
	// Changed reports that the file was written. An edit that changes no bytes
	// writes nothing.
	Changed bool `json:"changed"`
	// Requirements lists the Airflow entries in [project] dependencies
	// rewritten to the new pin, as they now read.
	Requirements []string `json:"requirements,omitempty"`
	// CoreReplaced reports that an apache-airflow-core entry became
	// apache-airflow, because the version is an Airflow 2 and core is
	// published only for Airflow 3. Its extras and marker are kept.
	CoreReplaced bool `json:"coreReplaced,omitempty"`
	// RemovedAirflowKey reports that a leftover [tool.astro] airflow line was
	// deleted. It decided nothing any more, and the manifest does not load
	// while it is there.
	RemovedAirflowKey bool `json:"removedAirflowKey,omitempty"`
	// RequiresPython is the [project] requires-python now written, when it was
	// the bound this package derives from the previous pin and the new pin
	// derives a different one. Empty when it was left alone.
	RequiresPython string `json:"requiresPython,omitempty"`
	// Dockerfile is [tool.astro] dockerfile when the project declares one.
	// Docker mode then builds from that file, so its FROM line, not the pin,
	// decides the image, and changing it is the user's.
	Dockerfile string `json:"dockerfile,omitempty"`
}

// AirflowPinOptions adjust SetAirflowVersionWith.
type AirflowPinOptions struct {
	// Catalog is the runtime catalog, when the caller has one loaded. With it,
	// a requires-python that init derived from the catalog (">=" and the
	// lowest Python an Airflow 3 series' runtime ships) counts as one this
	// package wrote, so it moves with the pin, and the new bound comes from
	// the catalog too. nil means only the built-in rule is known, and a
	// catalog-derived bound that differs from it reads as the user's.
	Catalog *runtimeversions.Catalog
}

// SetAirflowVersionWith moves the Airflow the manifest in dir pins to version,
// through EditManifest, so wrap and every write rule apply as they do there.
//
// The version is the Airflow requirement in [project] dependencies, so that is
// what changes: each apache-airflow or apache-airflow-core entry that does not
// already pin version becomes the requirement version derives ("==3.1.*" for a
// series, exact for a full version), keeping the name as written, its extras
// and its marker, whatever it pinned before: a range or a URL is replaced too.
//
// Because the caller names the version, this also repairs every manifest
// problem about it, which EditManifest reads for that reason
// (manifest.ParseForRepair):
//
//   - A leftover [tool.astro] airflow line is deleted, and RemovedAirflowKey
//     says so.
//   - A missing requirement is added, and a range, a URL or two pins that
//     disagree are replaced. Previous is then what the leftover line said, or
//     empty. Listing both apache-airflow and apache-airflow-core is a choice of
//     distribution a version cannot make, so that result is refused.
//
// Alongside it:
//
//   - requires-python moves to the new pin's bound only when it is exactly a
//     bound this package wrote for the previous pin, today's or the ">=3.10"
//     init once wrote for all of Airflow 3. A bound someone chose stays theirs.
//     With opts.Catalog it also recognizes, and writes, the bound the runtime
//     catalog gives.
//
// [tool.uv] is the project's, and a pin change never touches it.
//
// A declared dockerfile does not stop the write. The requirement is still
// read, for standalone mode and for the runtime's generation. The file's FROM
// line is not touched, and Dockerfile reports that it exists so the caller can
// say the image is the user's to move.
//
// Nothing else changes: providers, Dag code and Dockerfile steps an upgrade
// may need are judgment, not a pin, and are left to the user or an agent.
func SetAirflowVersionWith(dir string, wrap func(run func() error) error, version string, opts AirflowPinOptions) (AirflowPinChange, error) {
	if !manifest.ValidAirflowVersion(version) {
		return AirflowPinChange{}, fmt.Errorf("%w: %q is not a version like 3, 3.1, or 3.1.2", ErrInvalidAirflowVersion, version)
	}
	var change AirflowPinChange
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		change = AirflowPinChange{
			Previous:   before.Airflow().Pin,
			Version:    version,
			Dockerfile: before.Astro.Dockerfile,
		}
		// A manifest whose requirement states no single version has none to
		// read the previous one from; a leftover key is what its image ran.
		if change.Previous == "" || before.AirflowUnclear() {
			change.Previous = before.RemovedAirflowKey()
		}
		change.RemovedAirflowKey = ed.Delete(airflowKeyPath)
		rewritten, switched, err := repinAirflowRequirements(ed, version)
		if err != nil {
			return err
		}
		change.Requirements, change.CoreReplaced = rewritten, switched
		// No Airflow requirement to rewrite, which only a manifest loaded for
		// repair can have: the requirement is written instead.
		added, err := ensureAirflowDependency(ed, version)
		if err != nil {
			return err
		}
		if added != "" {
			change.Requirements = append(change.Requirements, added)
		}
		// With no previous version, no bound can be told apart as the one
		// this package wrote for it, so requires-python stays. A same-pin call
		// never moves it either, so a repeated call writes nothing even when
		// the catalog's bound differs from the one written.
		moved := change.Previous != version
		if rp := before.Project.RequiresPython; rp != "" && change.Previous != "" && moved &&
			initWroteRequiresPython(rp, change.Previous, moved, opts.Catalog) {
			if next := pythonBoundFor(version, opts.Catalog); next != rp {
				if err := ed.Set([]string{"project", "requires-python"}, next); err != nil {
					return err
				}
				change.RequiresPython = next
			}
		}
		return nil
	})
	if err != nil {
		return AirflowPinChange{}, err
	}
	change.Changed = len(change.Requirements) > 0 || change.RemovedAirflowKey || change.RequiresPython != ""
	return change, nil
}

// airflowKeyPath is the leftover [tool.astro] airflow key.
var airflowKeyPath = []string{"tool", "astro", manifestKeyAirflow}

// AirflowKeyMigration is what MigrateAirflowKey did to a manifest.
type AirflowKeyMigration struct {
	// Removed reports that the [tool.astro] airflow line was there and is
	// gone. False means the manifest had none, and nothing was written.
	Removed bool `json:"removed"`
	// Requirements lists the Airflow requirements written from the line, as
	// they now read, when the manifest's own did not state one version:
	// "apache-airflow==3.1.*" added from airflow = '3.1' where there was
	// none, or an "apache-airflow>=3" rewritten to it. Empty when the
	// manifest already pinned one version, which is then what the project
	// runs, and is left as it is.
	Requirements []string `json:"requirements,omitempty"`
	// CoreReplaced reports that an apache-airflow-core entry became
	// apache-airflow, as AirflowPinChange.CoreReplaced does.
	CoreReplaced bool `json:"coreReplaced,omitempty"`
}

// MigrateAirflowKey repairs a manifest carrying the leftover [tool.astro]
// airflow line (manifest.CodeAirflowRemoved), through EditManifest, and
// changes nothing else. It is the one call behind the desktop's fix, for every
// shape such a manifest takes:
//
//   - Beside a requirement that pins one version, the line decides nothing, so
//     it is deleted and the requirement stays what the project runs. When the
//     line named a different series, moving the requirement to it is
//     SetAirflowVersionWith's job, and the caller's to offer. An
//     apache-airflow-core entry on an Airflow 2 moves to apache-airflow at
//     its own version, which CoreReplaced reports.
//   - Beside a requirement that does not (manifest.CodeAirflowMissing,
//     CodeAirflowUnpinned, or two pins that disagree), the line was the
//     project's statement of its version, so it moves: every Airflow
//     requirement is rewritten to the one the line derives, keeping its name,
//     extras and marker, or that requirement is added where there is none, and
//     then the line is deleted. A line that is not a version cannot move, and
//     is refused.
//
// Listing both apache-airflow and apache-airflow-core is not something the
// line can settle, since it names a version and not a distribution: both are
// moved to its version, and the result is refused, as EditManifest refuses
// any edit that does not load, with the manifest's own "keep one of them".
//
// A manifest without the line is left as it is, and reports Removed false.
func MigrateAirflowKey(dir string, wrap func(run func() error) error) (AirflowKeyMigration, error) {
	var out AirflowKeyMigration
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		out = AirflowKeyMigration{}
		raw, ok := ed.Get(airflowKeyPath)
		if !ok {
			return nil
		}
		if before.AirflowUnclear() {
			v := before.RemovedAirflowKey()
			if v == "" {
				return fmt.Errorf("[tool.astro] airflow = %v is not a version like 3, 3.1, or 3.1.2, so it cannot become the "+
					"apache-airflow requirement: pin one in [project] dependencies and delete the airflow line", raw)
			}
			rewritten, switched, err := repinAirflowRequirements(ed, v)
			if err != nil {
				return err
			}
			out.CoreReplaced = switched
			added, err := ensureAirflowDependency(ed, v)
			if err != nil {
				return err
			}
			out.Requirements = rewritten
			if added != "" {
				out.Requirements = append(out.Requirements, added)
			}
		} else {
			// The requirement states the version and keeps it. Its one
			// fixable fault left is an apache-airflow-core entry on an Airflow
			// 2, which moves to apache-airflow at the same version; every
			// other entry already says that version, and is left alone.
			rewritten, switched, err := repinAirflowRequirements(ed, before.Airflow().Pin)
			if err != nil {
				return err
			}
			out.Requirements, out.CoreReplaced = rewritten, switched
		}
		out.Removed = ed.Delete(airflowKeyPath)
		return nil
	})
	if err != nil {
		return AirflowKeyMigration{}, err
	}
	return out, nil
}

// initWroteRequiresPython reports whether rp is a bound this package wrote for
// the previous pin, and so one a pin change may move. That is today's
// requiresPython; the bound the catalog gives the previous series, when the
// caller has the catalog; and, once the pin moves, the ">=3.10" init wrote for
// every Airflow 3 before its floor followed the runtime. A same-pin call only
// recognizes today's, so it stays a no-op.
func initWroteRequiresPython(rp, previous string, moved bool, catalog *runtimeversions.Catalog) bool {
	if rp == requiresPython(previous) {
		return true
	}
	if bound, ok := catalogBound(previous, catalog); ok && rp == bound {
		return true
	}
	major, _, _ := strings.Cut(previous, ".")
	return moved && major != "2" && rp == ">=3.10"
}

// pythonBoundFor is the requires-python a pin gets: the catalog's for an
// Airflow 3 series it lists, else the built-in rule.
func pythonBoundFor(version string, catalog *runtimeversions.Catalog) string {
	if bound, ok := catalogBound(version, catalog); ok {
		return bound
	}
	return requiresPython(version)
}

// catalogBound is the catalog's requires-python for the series an Airflow 3
// pin names. Airflow 2 keeps the built-in rule whatever the catalog says: its
// ceilings exist for a real failure, and the catalog lists no Python for it. A
// bare "3" names no series, so it has no catalog bound either.
func catalogBound(pin string, catalog *runtimeversions.Catalog) (string, bool) {
	if catalog == nil {
		return "", false
	}
	major, rest, ok := strings.Cut(pin, ".")
	if !ok || major != "3" {
		return "", false
	}
	minor, _, _ := strings.Cut(rest, ".")
	if minor == "" {
		return "", false
	}
	return catalog.RequiresPython(major + "." + minor)
}

// repinAirflowRequirements rewrites each Airflow entry of [project]
// dependencies that does not already pin version, pinned or not, element by
// element so the array keeps its layout and comments, and returns the entries
// it wrote, as they now read.
//
// An apache-airflow-core entry becomes apache-airflow when version is an
// Airflow 2, since core is published only for Airflow 3; switched reports it.
func repinAirflowRequirements(ed tomledit.Editor, version string) (rewritten []string, switched bool, err error) {
	raw, ok := ed.Get([]string{"project", "dependencies"})
	if !ok {
		return nil, false, nil
	}
	deps, ok := raw.([]any)
	if !ok {
		return nil, false, nil
	}
	for i, d := range deps {
		spec, ok := d.(string)
		if !ok || !manifest.NamesAirflow(spec) {
			continue
		}
		// An entry that already says version is left as written, so a repeated
		// call writes nothing and does not respace it. "Says" is AirflowPin's
		// reading, so ==3.1 (exactly 3.1.0) does not already say the series 3.1.
		toFull := coreOnAirflow2(spec, version)
		if stated, _ := manifest.AirflowPin(spec); stated == version && !toFull {
			continue
		}
		next, _ := repinAirflow(spec, version)
		if err := ed.Set([]string{"project", "dependencies", strconv.Itoa(i)}, next); err != nil {
			return nil, false, err
		}
		rewritten = append(rewritten, next)
		switched = switched || toFull
	}
	return rewritten, switched, nil
}

// coreOnAirflow2 reports whether spec is an apache-airflow-core entry and
// version an Airflow 2, which that distribution does not publish.
func coreOnAirflow2(spec, version string) bool {
	return manifest.DistName(spec) == "apache-airflow-core" && (manifest.Airflow{Pin: version}).Major() == "2"
}

// repinAirflow returns an Airflow requirement pinned to version, keeping the
// name as written, its extras and its environment marker, whatever the entry
// pinned before: a clean "==" pin, a range, a direct URL, or nothing. An
// apache-airflow-core entry becomes apache-airflow for an Airflow 2, which
// core is not published for. It refuses anything that is not an Airflow
// requirement.
func repinAirflow(spec, version string) (string, bool) {
	if !manifest.NamesAirflow(spec) {
		return "", false
	}
	req, marker, hasMarker := strings.Cut(spec, ";")
	head := strings.TrimSpace(req)
	// The name and its extras end where the first specifier, URL or space
	// after the extras begins.
	from := 0
	if i := strings.Index(head, "]"); i >= 0 {
		from = i + 1
	}
	if j := strings.IndexAny(head[from:], "<>=!~@ \t("); j >= 0 {
		head = head[:from+j]
	}
	if coreOnAirflow2(spec, version) {
		// The name as written, with its extras, swapped for the distribution
		// Airflow 2 is published as.
		extras := ""
		if i := strings.Index(head, "["); i >= 0 {
			extras = head[i:]
		}
		head = "apache-airflow" + extras
	}
	_, pin, _ := strings.Cut(airflowRequirement(version), "==")
	out := head + "==" + pin
	if hasMarker {
		out += "; " + strings.TrimSpace(marker)
	}
	return out, true
}
