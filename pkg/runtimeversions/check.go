package runtimeversions

import (
	"context"
	"fmt"
	"strings"
)

// A manifest's [tool.astro] runtime picks one Astro Runtime build for the
// image, inside the Airflow its requirement pins. pkg/manifest checks what the
// tag alone says, on every load and offline: an Airflow 3 tag's series, an
// Airflow 2 tag's generation. The rest needs the catalog, so it is checked here,
// by whatever builds an image (a Docker-mode start, astro deploy, astro
// package), and never when a manifest is only read:
//
//   - The exact Airflow the build carries, against the requirement. A build of
//     the right series under an exact pin it excludes (3.3-8 carries 3.3.2,
//     beside ==3.3.1) is a warning: the image runs one patch and standalone
//     installs another.
//   - A yanked build, with the catalog's reason, is a warning.
//   - An Airflow 2 build carrying another series than the requirement pins is
//     an error. Its tag names a runtime version, so this is the first place the
//     series can be read at all.
//   - Offline, an Airflow 2 build proceeds, and a finding says its series went
//     unchecked. An Airflow 3 build says nothing: its series was already read
//     off the tag.

// FindingKind names what a runtime check found. The values are stable, for a
// caller to branch on; the Message beside them is for a person.
type FindingKind string

const (
	// FindingAirflowExcluded is a build of the requirement's series whose exact
	// Airflow the requirement excludes. A warning.
	FindingAirflowExcluded FindingKind = "runtime_airflow_excluded"
	// FindingYanked is a build the catalog has withdrawn. A warning.
	FindingYanked FindingKind = "runtime_yanked"
	// FindingSeriesMismatch is a build carrying another Airflow series than the
	// requirement pins. Blocking.
	FindingSeriesMismatch FindingKind = "runtime_series_mismatch"
	// FindingUnknown is a build the catalog does not list, so nothing about it
	// could be checked. A warning: a build published after the catalog was
	// cached is the ordinary cause.
	FindingUnknown FindingKind = "runtime_unknown"
	// FindingSkipped is an Airflow 2 build checked with no catalog to read, so
	// its series went unchecked. A note.
	FindingSkipped FindingKind = "runtime_check_skipped"
)

// Finding is one thing a runtime check found.
type Finding struct {
	Kind    FindingKind `json:"kind"`
	Message string      `json:"message"`
	// Blocking means the image must not be built: the build and the
	// requirement disagree on the Airflow series.
	Blocking bool `json:"blocking"`

	// The structured facts the Message is written from, for a caller that
	// phrases the finding itself (Astro Desktop translates it) and should not
	// have to look them up again. Each is empty where the kind has none.

	// Runtime is the build checked, as [tool.astro] runtime names it.
	Runtime string `json:"runtime,omitempty"`
	// AirflowPin is the requirement's pin the build was checked against.
	AirflowPin string `json:"airflowPin,omitempty"`
	// AirflowVersion is the exact Airflow the catalog says the build carries.
	// Set for FindingAirflowExcluded, FindingSeriesMismatch and FindingYanked.
	AirflowVersion string `json:"airflowVersion,omitempty"`
	// YankedReason is the catalog's reason for withdrawing the build, trimmed,
	// for FindingYanked. Empty when the catalog gives none.
	YankedReason string `json:"yankedReason,omitempty"`
	// Suggested is a build carrying AirflowPin that the Message names instead,
	// for FindingSeriesMismatch when the catalog has one.
	Suggested string `json:"suggested,omitempty"`
}

// RuntimeError is a blocking Finding, as the error CheckRuntime returns.
type RuntimeError struct {
	Finding Finding
}

func (e *RuntimeError) Error() string { return e.Finding.Message }

// CheckRuntime checks a runtime build against the Airflow a requirement pins,
// with what the catalog says about the build: the exact Airflow it carries, and
// whether it is yanked. airflowPin is the manifest's pin ("3.3", "3.3.1",
// "2.11"). A nil catalog is one that could not be read.
//
// It reports every finding, blocking or not, and nothing for an empty runtime.
func (c *Catalog) CheckRuntime(runtime, airflowPin string) []Finding {
	runtime, airflowPin = strings.TrimSpace(runtime), strings.TrimSpace(airflowPin)
	if runtime == "" || airflowPin == "" {
		return nil
	}
	airflow2 := !strings.HasPrefix(runtime, "3.")
	if c == nil {
		if !airflow2 {
			return nil
		}
		return []Finding{{Kind: FindingSkipped, Runtime: runtime, AirflowPin: airflowPin, Message: fmt.Sprintf(
			"the runtime catalog could not be read, so which Airflow 2 series runtime %s carries was not checked against the requirement's %s",
			runtime, airflowPin)}}
	}
	r, ok := c.Runtime(runtime)
	if !ok || r.AirflowVersion == "" {
		return []Finding{{Kind: FindingUnknown, Runtime: runtime, AirflowPin: airflowPin, Message: fmt.Sprintf(
			"the runtime catalog lists no build %s, so the Airflow it carries was not checked against the requirement's %s", runtime, airflowPin)}}
	}
	var out []Finding
	if !pinCovers(airflowPin, r.AirflowVersion) {
		out = append(out, c.disagreement(&r, airflowPin))
	}
	if r.Yanked {
		msg := fmt.Sprintf("runtime %s is yanked", runtime)
		reason := strings.TrimSuffix(strings.TrimSpace(r.YankedReason), ".")
		if reason != "" {
			msg += ": " + reason
		}
		msg += ". Pick another build of the series in [tool.astro] runtime, or delete the line to build from the newest one"
		out = append(out, Finding{
			Kind: FindingYanked, Message: msg,
			Runtime: runtime, AirflowPin: airflowPin, AirflowVersion: r.AirflowVersion, YankedReason: reason,
		})
	}
	return out
}

// disagreement is the finding for a build whose Airflow the pin does not
// cover: another series is blocking, and another patch of the same series is a
// warning.
func (c *Catalog) disagreement(r *Runtime, pin string) Finding {
	carried := r.AirflowVersion
	pinSeries := seriesOf(pin)
	sameSeries := seriesOf(carried) == pinSeries
	if pinSeries == "" { // a pin naming only the generation
		sameSeries = majorOf(carried) == pin
	}
	if !sameSeries {
		msg := fmt.Sprintf("runtime %s carries Airflow %s, and the requirement pins Airflow %s: the image would run one series and standalone install another",
			r.Tag, carried, pin)
		other, ok := c.NewestRuntimeFor(pin)
		if ok {
			msg += fmt.Sprintf(". Name a build carrying Airflow %s, like %s, in [tool.astro] runtime, or change the requirement", pin, other)
		} else {
			msg += ". Change [tool.astro] runtime or the requirement so they name the same series"
		}
		return Finding{
			Kind: FindingSeriesMismatch, Message: msg, Blocking: true,
			Runtime: r.Tag, AirflowPin: pin, AirflowVersion: carried, Suggested: other,
		}
	}
	return Finding{Kind: FindingAirflowExcluded, Runtime: r.Tag, AirflowPin: pin, AirflowVersion: carried, Message: fmt.Sprintf(
		"runtime %s carries Airflow %s, which the requirement's pin %s excludes: Docker mode, deploy and package run %s while standalone installs %s. "+
			"Pin the requirement to %s or to the series, or pick a build carrying %s",
		r.Tag, carried, pin, carried, pin, carried, pin)}
}

// CheckRuntime loads the catalog as o says and checks runtime against
// airflowPin with it (Catalog.CheckRuntime). It never fails for want of the
// catalog: a catalog that cannot be read is checked as a nil one.
//
// The blocking finding, when there is one, comes back as a *RuntimeError, and
// the rest as warnings, for the caller to show. An empty runtime loads nothing
// and finds nothing.
func CheckRuntime(ctx context.Context, o Options, runtime, airflowPin string) ([]Finding, error) {
	if strings.TrimSpace(runtime) == "" {
		return nil, nil
	}
	c, _, err := Load(ctx, o)
	if err != nil {
		c = nil
	}
	var (
		warnings []Finding
		blocking error
	)
	for _, f := range c.CheckRuntime(runtime, airflowPin) {
		switch {
		case !f.Blocking:
			warnings = append(warnings, f)
		case blocking == nil:
			blocking = &RuntimeError{Finding: f}
		}
	}
	return warnings, blocking
}
