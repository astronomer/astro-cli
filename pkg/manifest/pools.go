package manifest

import (
	"fmt"
	"strings"
)

// [tool.astro.pools] declares the Airflow pools a project's Dags use, so a
// local Airflow has them from its first start. v1 created pools from
// airflow_settings.yaml on every start; v2 reads that file nowhere, and a task
// assigned to a pool Airflow does not have waits in the queue forever.
//
// Each entry is keyed by the pool's name, and `astro local start` creates or
// updates it once Airflow answers. A pool the table does not list is left
// alone, so a pool made by hand in the UI survives a restart.

const poolsKey = astroRoot + ".pools"

// DefaultPoolName is the pool Airflow creates itself and every task uses when
// it names none. A manifest may set its slots and include_deferred, the two
// fields Airflow lets a caller change on it.
const DefaultPoolName = "default_pool"

// maxPoolNameLength is the width of Airflow's pool name column, which the
// Airflow 3 API also enforces on a new pool.
const maxPoolNameLength = 256

// UnlimitedPoolSlots is the slots value Airflow reads as no limit.
const UnlimitedPoolSlots = -1

var poolKeys = []string{"description", "include_deferred", "slots"}

// Pool is one [tool.astro.pools] entry. The optional fields are left out of
// the update when unset, so Airflow keeps whatever value it has.
type Pool struct {
	// Slots is how many task instances may run in the pool at once, or
	// UnlimitedPoolSlots.
	Slots int
	// Description is shown in the Airflow UI. Empty when unset.
	Description string
	// IncludeDeferred counts deferred tasks against the pool's slots. Nil when
	// unset. Airflow 2.7 and later.
	IncludeDeferred *bool
}

// pools decodes [tool.astro.pools].
func (p *parser) pools(v any) map[string]Pool {
	table := p.table(poolsKey, v)
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]Pool, len(table))
	for name, raw := range table {
		key := poolsKey + "." + name
		if reason := PoolNameProblem(name); reason != "" {
			p.add(CodePoolNameInvalid, key, reason)
			continue
		}
		fields := p.table(key, raw)
		if fields == nil {
			continue
		}
		out[name] = p.pool(key, name, fields)
	}
	return out
}

func (p *parser) pool(key, name string, fields map[string]any) Pool {
	p.unknownKeys(key, fields, poolKeys)
	pool := Pool{
		Slots:       p.poolSlots(key+".slots", fields["slots"]),
		Description: p.str(key+".description", fields["description"]),
	}
	if v, ok := fields["include_deferred"]; ok {
		b := p.boolean(key+".include_deferred", v)
		pool.IncludeDeferred = &b
	}
	if name == DefaultPoolName && pool.Description != "" {
		p.add(CodeDefaultPoolDescription, key+".description",
			"Airflow does not let default_pool's description change: set only slots and include_deferred on it")
	}
	return pool
}

// PoolNameProblem says why name cannot be a pool, or "" when it can. A name
// with a slash would be created but could never be read or updated again,
// since the API addresses a pool by its name in the URL path.
func PoolNameProblem(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "a pool needs a name"
	case len(name) > maxPoolNameLength:
		return fmt.Sprintf("a pool name is at most %d characters", maxPoolNameLength)
	case strings.Contains(name, "/"):
		return "a pool name cannot contain a slash"
	}
	return ""
}

// poolSlots decodes a pool's slots: required, and a whole number above zero,
// or UnlimitedPoolSlots.
func (p *parser) poolSlots(key string, v any) int {
	if v == nil {
		p.add(CodeRequired, key, "required: how many tasks may run in the pool at once")
		return 0
	}
	n, ok := v.(int64)
	if !ok {
		p.add(CodeExpectedInteger, key, "expected a whole number")
		return 0
	}
	if !ValidPoolSlots(int(n)) {
		p.add(CodePoolSlotsInvalid, key, fmt.Sprintf("%d is not a slot count: use a number above zero, or -1 for no limit", n))
		return 0
	}
	return int(n)
}

// ValidPoolSlots reports whether n can be a pool's slots: above zero, or
// UnlimitedPoolSlots.
func ValidPoolSlots(n int) bool {
	return n >= 1 || n == UnlimitedPoolSlots
}
