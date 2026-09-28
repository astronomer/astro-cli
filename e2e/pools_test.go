//go:build e2e && !windows

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// [tool.astro.pools] reaches Airflow at start, and a changed slot count
// reaches it at restart. default_pool and a pool with no description ride
// along, because Airflow 3 validates each of their updates differently, and a
// stub cannot prove either.
//
// Tier 2: a real Airflow has to take the calls.
func TestStartAndRestartApplyTheManifestPools(t *testing.T) {
	tier(t, 2)

	p := airflowProject(t)
	manifestPath := filepath.Join(p.Dir, "pyproject.toml")
	base, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writePools := func(slots, defaultPool string) {
		t.Helper()
		pools := "\n[tool.astro.pools]\n" +
			"etl = {slots = " + slots + ", description = 'ETL loads'}\n" +
			"ml = {slots = " + slots + "}\n" +
			"default_pool = {slots = " + defaultPool + "}\n"
		if err := os.WriteFile(manifestPath, append(append([]byte{}, base...), pools...), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writePools("4", "16")
	start := p.runSlow("local", "start")
	t.Cleanup(func() { p.runSlow("local", "stop") })
	start.requireSuccess()
	if strings.Contains(start.Stdout, "warning: pool") {
		t.Errorf("the start warned about a pool\n%s", start.output())
	}
	p.requirePool("etl", 4, "ETL loads")
	p.requirePool("ml", 4, "")
	p.requirePool("default_pool", 16, "")

	writePools("5", "17")
	restart := p.runSlow("local", "restart").requireSuccess()
	if strings.Contains(restart.Stdout, "warning: pool") {
		t.Errorf("the restart warned about a pool\n%s", restart.output())
	}
	p.requirePool("etl", 5, "ETL loads")
	p.requirePool("ml", 5, "")
	p.requirePool("default_pool", 17, "")
}

func (p *project) requirePool(name string, slots int, description string) {
	p.t.Helper()
	var got struct {
		Slots       int    `json:"slots"`
		Description string `json:"description"`
	}
	p.run("local", "af", "pools", "get", name, "--output", "json").requireSuccess().requireJSON(&got)
	if got.Slots != slots {
		p.t.Errorf("pool %s has %d slots, want %d", name, got.Slots, slots)
	}
	if description != "" && got.Description != description {
		p.t.Errorf("pool %s has description %q, want %q", name, got.Description, description)
	}
}
