package local

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// applyPools creates or updates each [tool.astro.pools] entry in the Airflow
// a start just brought up. Nothing reads pools from anywhere else, so without
// this a Dag naming one waits forever.
//
// A pool that does not go in is a warning, never a failed start: Airflow is
// up, and every Dag not using that pool runs. A pool the manifest does not
// list is left alone.
func (c *cli) applyPools(ctx context.Context, r Renderer, st localrt.Status, pools map[string]manifest.Pool) {
	if len(pools) == 0 {
		return
	}
	transport, err := localInstance(st).Transport(ctx, c.instanceDeps(""))
	if err != nil {
		warnPool(r, "pools were not created or updated", "tool.astro.pools", err)
		return
	}
	client := airflowapi.New(transport)
	for _, name := range slices.Sorted(maps.Keys(pools)) {
		if err := client.UpsertPool(ctx, poolSpec(name, pools[name])); err != nil {
			warnPool(r, "pool "+name+" was not created or updated", "tool.astro.pools."+name, err)
		}
	}
}

func warnPool(r Renderer, what, key string, err error) {
	emitWarning(r, event{
		Event:  "warning",
		Text:   fmt.Sprintf("%s: %v", what, err),
		Key:    key,
		Reason: err.Error(),
	})
}

func poolSpec(name string, p manifest.Pool) airflowapi.PoolSpec {
	return airflowapi.PoolSpec{Name: name, Slots: p.Slots, Description: p.Description, IncludeDeferred: p.IncludeDeferred}
}
