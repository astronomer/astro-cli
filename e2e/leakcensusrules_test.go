//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// The census's rules, checked without docker.
//
// These are tier 0 on purpose. The axes need an engine, but what the census
// DECIDES — whose objects it claims, and what it does when an axis could not
// look — is arithmetic, and the two ways of getting it wrong both pass a run
// that only exercises the happy path: watching every compose project on the
// machine, and treating an axis that errored as an axis that found nothing.

// The suite claims astro's compose objects and leaves everyone else's alone.
//
// The label filter finds every compose project on the machine, which on a
// developer's laptop includes their own work. Real names, from a machine that
// had both kinds on it.
func TestTheCensusClaimsOnlyAstroProjects(t *testing.T) {
	tier(t, 0)
	for _, name := range []string{
		"astro-ospkgs-f08447-api-server-1",
		"astro-ospkgs-f08447_postgres_data",
		"astro-ospkgs-f08447_airflow",
	} {
		if !ours(name) {
			t.Errorf("%q is this suite's and was not claimed", name)
		}
	}
	for _, name := range []string{
		"example_abc123-api-server-1",
		"example-two_def456_postgres_data",
		"example-three_789abc_airflow_logs",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef_postgres_data",
	} {
		if ours(name) {
			t.Errorf("%q belongs to somebody else and was claimed anyway", name)
		}
	}
}

// The container axis records astro's containers and ignores the rest.
//
// Checked at the axis rather than only on ours(), because the two being right
// separately does not make the wiring between them right: an axis that forgot
// its keep would pass every test above while watching the whole machine.
func TestAnAxisRecordsOnlyWhatItClaims(t *testing.T) {
	tier(t, 0)
	var container censusAxis
	for _, a := range censusAxes() {
		if a.kind == "container" {
			container = a
			break
		}
	}
	if container.kind == "" {
		t.Fatal("no container axis to check")
	}

	c := newCensus()
	c.record(container, []string{
		"astro-ospkgs-f08447-scheduler-1",
		"example_abc123-api-server-1",
	})

	if _, ok := c.found["container astro-ospkgs-f08447-scheduler-1"]; !ok {
		t.Error("the suite's own container was not recorded")
	}
	if _, ok := c.found["container example_abc123-api-server-1"]; ok {
		t.Error("somebody else's container was recorded")
	}
	if len(c.found) != 1 {
		t.Errorf("want exactly the one container, got %v", c.found)
	}
	if !c.covered[container.id()] {
		t.Error("an axis that answered should be marked covered")
	}
}

// Below tier 3 the census asks docker nothing.
//
// Nothing under that tier can make a docker object, and the Windows PR job runs
// tier 0 on a machine whose engine is stopped, where every query would pay its
// full timeout to learn nothing. Covered means "this axis answered", so a run
// that asked nothing covers no axes.
func TestTheCensusDoesNotAskDockerBelowTierThree(t *testing.T) {
	tier(t, 0)
	c := takeCensus(t.Context(), dockerTier-1)
	if len(c.covered) != 0 {
		t.Errorf("tier %d should ask docker nothing; it covered %v", dockerTier-1, c.covered)
	}
	if len(c.found) != 0 {
		t.Errorf("tier %d should find nothing; it found %v", dockerTier-1, c.found)
	}
}

// censusOf builds a snapshot covering one axis, for the diff tests.
func censusOf(axis string, names ...string) census {
	c := newCensus()
	c.covered[axis] = true
	for _, n := range names {
		c.found[n] = axis
	}
	return c
}

// What the run added is reported, and what was already there is not.
func TestTheCensusReportsOnlyWhatTheRunAdded(t *testing.T) {
	tier(t, 0)
	before := censusOf("container", "container astro-old-1")
	after := censusOf("container", "container astro-old-1", "container astro-new-1")

	left, blind := leaks(before, after)
	if len(blind) != 0 {
		t.Fatalf("both snapshots covered the axis; got blind %v", blind)
	}
	if len(left) != 1 || left[0] != "container astro-new-1" {
		t.Fatalf("want the one new container, got %v", left)
	}
}

// An axis that could not look is not an axis that found nothing.
//
// The mirror cases fail in opposite directions, so both are checked. A before
// that failed and an after that worked would report every pre-existing object as
// this run's leak, and the nightly files an issue for that. A before that worked
// and an after that failed would hide real leaks and pass.
func TestAnAxisThatCouldNotLookIsSaidSoRatherThanGuessed(t *testing.T) {
	tier(t, 0)

	t.Run("no before-snapshot", func(t *testing.T) {
		before := newCensus() // the engine was still starting
		after := censusOf("image", "image astro-local/someone-elses:latest")

		left, blind := leaks(before, after)
		if len(left) != 0 {
			t.Errorf("nothing can be called a leak on an axis with no before; got %v", left)
		}
		if len(blind) != 1 || !strings.Contains(blind[0], "no before-snapshot") {
			t.Errorf("the uncomparable axis should be reported; got %v", blind)
		}
	})

	t.Run("axis went blind after the run", func(t *testing.T) {
		before := censusOf("image", "image astro-local/a:latest")
		after := newCensus() // the engine went away mid-run

		left, blind := leaks(before, after)
		if len(left) != 0 {
			t.Errorf("an axis that stopped answering reports no leaks; got %v", left)
		}
		if len(blind) != 1 || !strings.Contains(blind[0], "could not be checked") {
			t.Errorf("the axis that stopped answering should be reported; got %v", blind)
		}
	})
}

// Every axis carries its own id, so that the two image queries do not pass for
// each other when one of them fails.
func TestEveryAxisIsDistinguishable(t *testing.T) {
	tier(t, 0)
	seen := map[string]bool{}
	for _, a := range censusAxes() {
		if seen[a.id()] {
			t.Errorf("two axes share the id %q, so coverage cannot tell them apart", a.id())
		}
		seen[a.id()] = true
	}
	if len(seen) != 5 {
		t.Errorf("want 5 axes (container, volume, network, and the two image repositories), got %d", len(seen))
	}
}
