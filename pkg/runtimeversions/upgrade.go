package runtimeversions

import (
	"strconv"
	"strings"
)

// AirflowUpgradeTargets are the two Airflow versions an upgrade can offer a
// project pinned to pin, at the pin's own precision. Either may be empty,
// meaning nothing newer on that axis.
//
//   - SameGen is the newest Airflow of the pin's own generation.
//   - CrossGen is the newest Airflow 3, offered only to an Airflow 2 pin: an
//     Airflow 3 project has nowhere higher to go.
//
// "Newest" for a generation is the Airflow carried by its highest runtime
// build, by tag, that is neither yanked nor on the deprecated channel. Release
// date is not consulted, and neither is any other channel. A pin with no minor
// ("3") names no series and is offered nothing, and so is any generation but 2
// and 3.
//
// This is the rule Astro Desktop's upgrade offer makes, and both tools call it.
// Which of the two a caller acts on is the caller's choice.
func (c *Catalog) AirflowUpgradeTargets(pin string) (sameGen, crossGen string) {
	parts := pinComponents(pin)
	if len(parts) < 2 {
		return "", ""
	}
	switch parts[0] {
	case "3":
		return airflowPinTarget(pin, c.latestOfferedAirflow("3")), ""
	case "2":
		return airflowPinTarget(pin, c.latestOfferedAirflow("2")),
			airflowPinTarget(pin, c.latestOfferedAirflow("3"))
	}
	return "", ""
}

// channelDeprecated retires a whole line of builds from being offered.
const channelDeprecated = "deprecated"

// latestOfferedAirflow is the Airflow the highest offerable build of a
// generation carries: by tag, skipping yanked builds and the deprecated
// channel. Ties on tag order break by the tag itself, so the answer does not
// depend on map order.
func (c *Catalog) latestOfferedAirflow(major string) string {
	var best *Runtime
	for _, r := range c.runtimes {
		if r.Yanked || r.Channel == channelDeprecated || majorOf(r.AirflowVersion) != major {
			continue
		}
		if best == nil {
			best = r
			continue
		}
		if cmp := compareVersions(r.Tag, best.Tag); cmp > 0 || (cmp == 0 && r.Tag > best.Tag) {
			best = r
		}
	}
	if best == nil {
		return ""
	}
	return best.AirflowVersion
}

// airflowPinTarget is the version a pin has to move to in order to run latest,
// or "" when the pin already covers it. The target is latest cut to the pin's
// precision, because a pin resolves at the precision it was written at: a
// series pin ("3.1") already runs the newest patch of its series, so only a
// newer series is news to it, while an exact pin ("3.1.2") is behind on a newer
// patch. Comparison is numeric, so "3.10" ranks above "3.9".
func airflowPinTarget(pin, latest string) string {
	parts := pinComponents(pin)
	if len(parts) == 0 {
		return ""
	}
	target := truncateVersion(latest, len(parts))
	if target == "" || compareVersions(target, strings.Join(parts, ".")) <= 0 {
		return ""
	}
	return target
}

// truncateVersion is v's first n components, normalized ("03" reads 3), or ""
// when v has fewer or is not a dotted number. Fewer is "" because a coarser
// latest cannot say whether a finer pin is behind.
func truncateVersion(v string, n int) string {
	parts := pinComponents(v)
	if len(parts) < n {
		return ""
	}
	out := make([]string, n)
	for i, p := range parts[:n] {
		c, _ := strconv.Atoi(p) //nolint:errcheck // digits only, checked by pinComponents
		out[i] = strconv.Itoa(c)
	}
	return strings.Join(out, ".")
}

// maxPinComponents is the most a pin may name: major, minor, patch.
const maxPinComponents = 3

// pinComponents splits a dotted version of one to three digit-only components,
// or returns nil for anything else: a sign, a prefix, a suffix, an empty part.
func pinComponents(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	if len(parts) > maxPinComponents {
		return nil
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return nil
		}
	}
	return parts
}
