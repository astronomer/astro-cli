package scaffold

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// ErrInvalidAirflowVersion reports a pin SetAirflowVersion refused before
// reading the manifest, because [tool.astro] airflow would not accept it.
var ErrInvalidAirflowVersion = errors.New("not an Airflow version")

// AirflowPinChange is what SetAirflowVersion did to a manifest.
type AirflowPinChange struct {
	// Previous is the [tool.astro] airflow the manifest had.
	Previous string `json:"previous"`
	// Version is the pin now written.
	Version string `json:"version"`
	// Changed reports that the file was written. An edit that changes no bytes
	// writes nothing.
	Changed bool `json:"changed"`
	// Requirements lists the apache-airflow entries in [project] dependencies
	// rewritten to the new pin, as they now read.
	Requirements []string `json:"requirements,omitempty"`
	// UnreadRequirements lists the apache-airflow entries left as they were,
	// because they pin Airflow in a shape no single version reads out of (a
	// range, a direct URL). They are the user's to reconcile with the pin:
	// standalone mode installs what they say.
	UnreadRequirements []string `json:"unreadRequirements,omitempty"`
	// RequiresPython is the [project] requires-python now written, when it was
	// the bound this package derives from the previous pin and the new pin
	// derives a different one. Empty when it was left alone.
	RequiresPython string `json:"requiresPython,omitempty"`
	// Dockerfile is [tool.astro] dockerfile when the project declares one.
	// Docker mode then builds from that file, so its FROM line, not the pin,
	// decides the image, and changing it is the user's.
	Dockerfile string `json:"dockerfile,omitempty"`
	// SQLAlchemyCapAdded reports that the pin moved onto Airflow 3.1 and
	// [tool.uv] constraint-dependencies gained the sqlalchemy<2.1 entry init
	// writes for that series.
	SQLAlchemyCapAdded bool `json:"sqlalchemyCapAdded,omitempty"`
	// SQLAlchemyCapRemoved reports that the pin moved to one other than Airflow
	// 3.1 and that entry was taken out: one written for 3.1, or one an older
	// init wrote for every pin.
	SQLAlchemyCapRemoved bool `json:"sqlalchemyCapRemoved,omitempty"`
}

// SetAirflowVersion rewrites the [tool.astro] airflow pin of the manifest in
// dir to version, through EditManifest, so wrap and every write rule apply as
// they do there.
//
// The pin is not the only place a project states its Airflow, so it also
// changes what has to move with it for the project to run the new version:
//
//   - Each apache-airflow entry in [project] dependencies that carries a clean
//     "==" pin becomes the requirement the new pin derives ("==3.1.*" for a
//     series, exact for a full version), keeping its extras and marker.
//     Standalone mode installs from that list, so leaving it would run the old
//     Airflow under the new pin. An entry whose version the new pin already
//     covers ("==2.9.1" under "2.9") is left as written, unless it is the one
//     the previous pin derives, which follows a pin that widens ("2.9" to "2"
//     turns "==2.9.*" into "==2.*"). An entry pinned any other way is reported in
//     UnreadRequirements and left alone, because rewriting a range would be
//     guessing what it meant.
//   - requires-python moves to the new pin's bound only when it is exactly a
//     bound this package wrote for the previous pin, today's or the ">=3.10"
//     init once wrote for all of Airflow 3. A bound someone chose stays theirs.
//   - The SQLAlchemy cap follows the new pin. Moving onto 3.1 adds it the way
//     adopting a 3.1 manifest does, unless the manifest already constrains
//     SQLAlchemy. Moving to any other pin removes it, which also clears the cap
//     init once wrote for every pin, but only when it is exactly the entry this
//     package writes, along with a constraint-dependencies or [tool.uv] that
//     removing it leaves empty. Any other SQLAlchemy constraint is the user's
//     and stays.
//
// A declared dockerfile does not stop the write. The pin is still required and
// still read, and a project that later drops the declaration builds from it.
// The file's FROM line is not touched, and Dockerfile reports that it exists so
// the caller can say the image is the user's to move.
//
// Nothing else changes: providers, Dag code and Dockerfile steps an upgrade
// may need are judgment, not a pin, and are left to the user or an agent.
func SetAirflowVersion(dir string, wrap func(run func() error) error, version string) (AirflowPinChange, error) {
	if !manifest.ValidAirflowVersion(version) {
		return AirflowPinChange{}, fmt.Errorf("%w: %q is not a version like 3, 3.1, or 3.1.2", ErrInvalidAirflowVersion, version)
	}
	var change AirflowPinChange
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		change = AirflowPinChange{
			Previous:   before.Astro.AirflowVersion,
			Version:    version,
			Dockerfile: before.Astro.Dockerfile,
		}
		// Setting the value it already has can still requote it, so an
		// unchanged pin is not set, and a repeated call writes nothing.
		if change.Previous != version {
			if err := ed.Set([]string{"tool", "astro", manifestKeyAirflow}, version); err != nil {
				return err
			}
		}
		if err := setAirflowRequirements(ed, version, &change); err != nil {
			return err
		}
		moved := change.Previous != version
		if rp := before.Project.RequiresPython; rp != "" && initWroteRequiresPython(rp, change.Previous, moved) {
			if next := requiresPython(version); next != rp {
				if err := ed.Set([]string{"project", "requires-python"}, next); err != nil {
					return err
				}
				change.RequiresPython = next
			}
		}
		// Decided by the new pin, not by the line it crossed: init used to cap
		// every pin, so a 3.2 project scaffolded then carries the entry too, and
		// a move to 3.3 is the moment to take it out.
		switch was, is := needsSQLAlchemyCap(change.Previous), needsSQLAlchemyCap(version); {
		case is && !was:
			label, err := ensureSQLAlchemyCap(ed, version)
			if err != nil {
				return err
			}
			change.SQLAlchemyCapAdded = label != ""
		case !is && moved:
			change.SQLAlchemyCapRemoved = removeSQLAlchemyCap(ed)
		}
		return nil
	})
	if err != nil {
		return AirflowPinChange{}, err
	}
	// The cap and the bound only move with the pin, so they need no term of
	// their own here.
	change.Changed = change.Previous != version || len(change.Requirements) > 0 || change.RequiresPython != ""
	return change, nil
}

// initWroteRequiresPython reports whether rp is a bound this package wrote for
// the previous pin, and so one a pin change may move. That is today's
// requiresPython, and, once the pin moves, the ">=3.10" init wrote for every
// Airflow 3 before its floor followed the runtime. A same-pin call only
// recognizes today's, so it stays a no-op.
func initWroteRequiresPython(rp, previous string, moved bool) bool {
	if rp == requiresPython(previous) {
		return true
	}
	major, _, _ := strings.Cut(previous, ".")
	return moved && major != "2" && rp == ">=3.10"
}

// removeSQLAlchemyCap takes the sqlalchemyCap entry out of [tool.uv]
// constraint-dependencies, and reports whether it was there. Only the exact
// string this package writes counts: "sqlalchemy<2.0.40" or "SQLAlchemy<2.1"
// was written by someone, for a reason this cannot see. A list or table left
// empty goes too, so a project init scaffolded on 3.1 ends up where one
// scaffolded on the new pin starts.
func removeSQLAlchemyCap(ed tomledit.Editor) bool {
	key := []string{"tool", "uv", "constraint-dependencies"}
	raw, _ := ed.Get(key)
	list, _ := raw.([]any)
	at := slices.IndexFunc(list, func(v any) bool { return v == sqlalchemyCap })
	if at < 0 {
		return false
	}
	if len(list) == 1 {
		ed.Delete(key)
	} else {
		ed.Delete(append(key, strconv.Itoa(at)))
	}
	if uv, ok := ed.Get([]string{"tool", "uv"}); ok {
		if table, isTable := uv.(map[string]any); isTable && len(table) == 0 {
			ed.Delete([]string{"tool", "uv"})
		}
	}
	return true
}

// setAirflowRequirements rewrites each apache-airflow entry of [project]
// dependencies that repinAirflow can read and whose version the new pin does
// not already cover, element by element so the array keeps its layout and
// comments, and records what it did in change.
func setAirflowRequirements(ed tomledit.Editor, version string, change *AirflowPinChange) error {
	raw, ok := ed.Get([]string{"project", "dependencies"})
	if !ok {
		return nil
	}
	deps, ok := raw.([]any)
	if !ok {
		return nil
	}
	for i, d := range deps {
		spec, ok := d.(string)
		if !ok || !namesAirflow(spec) {
			continue
		}
		next, ok := repinAirflow(spec, version)
		if !ok {
			change.UnreadRequirements = append(change.UnreadRequirements, spec)
			continue
		}
		// An entry already inside the pin says the same thing, or something
		// narrower someone chose, like a patch under a series. Rewriting it
		// would respace it or widen it for nothing. The exception is the entry
		// the previous pin derives: it follows a pin that moves, including one
		// that widens ("2.9" to "2"), or it keeps installing the old series.
		stated, _ := pinFromSpec(spec)
		if pinCovers(version, stated) && !followsThePin(stated, change.Previous, version) {
			continue
		}
		if err := ed.Set([]string{"project", "dependencies", strconv.Itoa(i)}, next); err != nil {
			return err
		}
		change.Requirements = append(change.Requirements, next)
	}
	return nil
}

// repinAirflow returns an apache-airflow requirement pinned to version, keeping
// the name as written, its extras and its environment marker. It rewrites only
// the entries pinFromSpec reads, a clean "==" pin, which leaves out a range and
// a direct URL.
func repinAirflow(spec, version string) (string, bool) {
	if _, ok := pinFromSpec(spec); !ok {
		return "", false
	}
	req, marker, hasMarker := strings.Cut(spec, ";")
	head, _, _ := strings.Cut(req, "==")
	out := strings.TrimSpace(head) + strings.TrimPrefix(airflowRequirement(version), airflowDist)
	if hasMarker {
		out += "; " + strings.TrimSpace(marker)
	}
	return out, true
}

// pinCovers reports whether a version an apache-airflow entry states is one the
// pin allows: the same version, or for a series pin ("2.9"), any version in
// that series ("2.9.1"). A full pin covers only itself, because pinFromSpec
// reads no version with more than three parts.
func pinCovers(pin, stated string) bool {
	return stated == pin || strings.HasPrefix(stated, pin+".")
}

// followsThePin reports whether an entry stating stated is the requirement the
// previous pin derives, and the pin is moving. pinFromSpec reads a derived
// requirement back as the pin itself ("==2.9.*" as "2.9"), so comparing the
// read versions ignores how the entry is spaced.
func followsThePin(stated, previous, version string) bool {
	derived, _ := pinFromSpec(airflowRequirement(previous))
	return previous != version && stated == derived
}
