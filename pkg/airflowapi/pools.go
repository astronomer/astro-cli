package airflowapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// PoolSpec is a pool as a caller wants it to be. Description and
// IncludeDeferred are optional: left unset, the pool keeps whatever value
// Airflow has.
type PoolSpec struct {
	Name        string
	Slots       int
	Description string
	// IncludeDeferred needs Airflow 2.7 or later; an older Airflow refuses a
	// pool that sets it.
	IncludeDeferred *bool
}

// defaultPool is the pool Airflow makes itself. Both generations refuse to
// rename it, and Airflow 3 refuses any change to it but slots and
// include_deferred, and then only through an update mask naming them.
const defaultPool = "default_pool"

// UpsertPool creates the pool, or updates it to match spec when it differs.
// It reads the pool first, so a pool already as spec describes is not
// written, and the fields spec leaves unset keep their values.
func (c *Client) UpsertPool(ctx context.Context, spec PoolSpec) error {
	current, err := c.GetPool(ctx, spec.Name)
	switch {
	case errors.Is(err, ErrNotFound):
		return c.do(ctx, Request{Method: http.MethodPost, Path: "/pools", Body: spec.body()}, nil)
	case err != nil:
		return err
	case spec.matches(current):
		return nil
	}
	generation, err := c.Generation(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, spec.patch(generation, current), nil)
}

func (s PoolSpec) matches(p Pool) bool {
	return s.Slots == p.Slots &&
		(s.Description == "" || s.Description == p.Description) &&
		(s.IncludeDeferred == nil || *s.IncludeDeferred == p.IncludeDeferred)
}

// body is the pool with only the fields spec sets.
func (s PoolSpec) body() map[string]any {
	body := map[string]any{"name": s.Name, "slots": s.Slots}
	if s.Description != "" {
		body["description"] = s.Description
	}
	if s.IncludeDeferred != nil {
		body["include_deferred"] = *s.IncludeDeferred
	}
	return body
}

// patch is the update for each generation's rules.
//
// Airflow 2 reads a patch with no update mask as the whole pool, and fills an
// absent include_deferred with false, so its patch names the fields it sets.
// Its mask is one comma-separated value.
//
// Airflow 3 validates a patch to any pool but default_pool as a whole pool,
// include_deferred included, so the current value rides along when spec sets
// none. default_pool takes a mask instead, one query value per field.
func (s PoolSpec) patch(generation Generation, current Pool) Request {
	body := s.body()
	var query url.Values
	switch {
	case generation == Airflow3 && s.Name == defaultPool:
		query = url.Values{"update_mask": s.mask()}
	case generation == Airflow3:
		if s.IncludeDeferred == nil {
			body["include_deferred"] = current.IncludeDeferred
		}
	default:
		query = url.Values{"update_mask": {strings.Join(s.mask(), ",")}}
	}
	return Request{Method: http.MethodPatch, Path: pathf("/pools/%s", s.Name), Query: query, Body: body}
}

func (s PoolSpec) mask() []string {
	mask := []string{"slots"}
	if s.Description != "" {
		mask = append(mask, "description")
	}
	if s.IncludeDeferred != nil {
		mask = append(mask, "include_deferred")
	}
	return mask
}
