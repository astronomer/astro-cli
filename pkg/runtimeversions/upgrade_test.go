package runtimeversions

import (
	"fmt"
	"testing"
)

func equal[T comparable](t *testing.T, want, got T, msg ...any) {
	t.Helper()
	if want != got {
		t.Errorf("%s: got %v, want %v", fmt.Sprint(msg...), got, want)
	}
}

func parseCatalog(t *testing.T, doc string) *Catalog {
	t.Helper()
	c, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// upgradeCatalog's newest offerable builds carry Airflow 2.10.5 and 3.2.1,
// the per-generation latest Astro Desktop's TestAirflowPinTargets measures
// against. The builds above them are each disqualified one way: yanked,
// deprecated, or, for 3.1-20, merely older by tag though shipped later.
const upgradeCatalogDoc = `{
  "runtimeVersions": {
    "13.6.0": {"metadata": {"airflowVersion": "2.10.4", "channel": "stable"}},
    "13.7.0": {"metadata": {"airflowVersion": "2.10.5", "channel": "stable"}},
    "13.8.0": {"metadata": {"airflowVersion": "2.11.0", "channel": "stable", "yanked": true}}
  },
  "runtimeVersionsV3": {
    "3.1-20": {"metadata": {"airflowVersion": "3.1.9", "channel": "stable", "releaseDate": "2026-09-01"}},
    "3.2-4":  {"metadata": {"airflowVersion": "3.2.1", "channel": "stable", "releaseDate": "2026-05-01"}},
    "3.3-1":  {"metadata": {"airflowVersion": "3.3.0", "channel": "deprecated"}},
    "3.4-1":  {"metadata": {"airflowVersion": "3.4.0", "channel": "stable", "yanked": true}}
  }
}`

// The cases of Astro Desktop's TestAirflowPinTargets, against a catalog that
// yields the same per-generation latest.
func TestAirflowUpgradeTargets(t *testing.T) {
	c := parseCatalog(t, upgradeCatalogDoc)
	for _, tc := range []struct {
		name, pin, sameGen, crossGen string
	}{
		{"airflow 3 series pin behind a series", "3.1", "3.2", ""},
		{"airflow 3 series pin on the latest series", "3.2", "", ""},
		{"airflow 3 patch pin behind a patch", "3.2.0", "3.2.1", ""},
		{"airflow 3 bare major floats and has nothing to offer", "3", "", ""},
		{"airflow 2 bare major is offered nothing", "2", "", ""},
		{"airflow 3 pin far behind still gets no cross-generation offer", "3.0", "3.2", ""},
		{"airflow 2 pin offered both axes", "2.9", "2.10", "3.2"},
		{"airflow 2 pin on the latest 2 is still offered 3", "2.10", "", "3.2"},
		{"unknown generation offers nothing", "4.1", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sameGen, crossGen := c.AirflowUpgradeTargets(tc.pin)
			equal(t, tc.sameGen, sameGen, "sameGen")
			equal(t, tc.crossGen, crossGen, "crossGen")
		})
	}
}

// A generation with nothing offerable has no latest, and offers nothing.
func TestAirflowUpgradeTargetsWithNothingOfferable(t *testing.T) {
	c := parseCatalog(t, `{"runtimeVersionsV3": {
		"3.3-1": {"metadata": {"airflowVersion": "3.3.0", "channel": "deprecated"}},
		"3.4-1": {"metadata": {"airflowVersion": "3.4.0", "channel": "stable", "yanked": true}}}}`)
	for _, pin := range []string{"3.1", "2.9", "3.1.2"} {
		sameGen, crossGen := c.AirflowUpgradeTargets(pin)
		equal(t, "", sameGen+crossGen, pin)
	}
}

// Highest by tag, numerically, not by release date or string order: 3.10-1
// outranks 3.9-5, which shipped later.
func TestLatestOfferedAirflowIsByTag(t *testing.T) {
	c := parseCatalog(t, `{"runtimeVersionsV3": {
		"3.9-5":  {"metadata": {"airflowVersion": "3.9.4", "channel": "stable", "releaseDate": "2027-02-01"}},
		"3.10-1": {"metadata": {"airflowVersion": "3.10.0", "channel": "stable", "releaseDate": "2027-01-01"}}}}`)
	equal(t, "3.10.0", c.latestOfferedAirflow("3"))
	equal(t, "", c.latestOfferedAirflow("2"))
}

// The cases of Astro Desktop's updates.TestAirflowPinTarget.
func TestAirflowPinTarget(t *testing.T) {
	for _, tc := range []struct{ name, pin, latest, want string }{
		{"series pin already on the latest series", "3.1", "3.1.3", ""},
		{"series pin behind a series", "3.1", "3.2.0", "3.2"},
		{"series pin offered a series, never a patch", "3.0", "3.2.4", "3.2"},
		{"patch pin behind a patch", "3.1.2", "3.1.3", "3.1.3"},
		{"patch pin on the latest patch", "3.1.3", "3.1.3", ""},
		{"patch pin behind a series keeps its precision", "3.1.2", "3.2.0", "3.2.0"},
		{"major pin has no same-generation upgrade", "3", "3.4.1", ""},
		{"major pin crossing generations", "2", "3.1.3", "3"},
		{"airflow 2 series pin offered airflow 3", "2.10", "3.1.3", "3.1"},
		{"airflow 2 patch pin offered airflow 3", "2.10.5", "3.1.3", "3.1.3"},
		{"double-digit minor outranks single", "3.9", "3.10.0", "3.10"},
		{"single-digit minor is not an upgrade over double", "3.10", "3.9.4", ""},
		{"pin ahead of the latest", "3.4", "3.2.1", ""},
		{"no latest version", "3.1", "", ""},
		{"no pin", "", "3.1.3", ""},
		{"blank pin", "  ", "3.1.3", ""},
		{"latest too coarse for a patch pin", "3.1.2", "3.1", ""},
		{"latest too coarse for a series pin", "3.1", "3", ""},
		{"non-numeric pin", "3.x", "3.1.3", ""},
		{"prefixed pin", "v3.1", "3.2.0", ""},
		{"pin with too many components", "3.1.2.4", "3.2.0", ""},
		{"non-numeric latest", "3.1", "3.2.0-rc1", ""},
		{"signed component in pin", "3.-1", "3.2.0", ""},
		{"explicitly positive pin", "+3.1", "3.2.0", ""},
		{"empty component in pin", "3..1", "3.2.0", ""},
		{"zero-padded pin still gets an offer", "03.1", "3.2.0", "3.2"},
		{"zero-padded latest is normalised", "3.1", "03.2.0", "3.2"},
		{"zero-padded pin on the latest series", "03.2", "3.2.0", ""},
		// Beyond the desktop's table: the cap holds when the latest is as long
		// as the pin, and surrounding space is trimmed as its parser trims it.
		{"pin and latest both with too many components", "3.1.2.4", "3.2.0.1", ""},
		{"space around the pin", " 3.1 ", "3.2.0", "3.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			equal(t, tc.want, airflowPinTarget(tc.pin, tc.latest))
		})
	}
}
