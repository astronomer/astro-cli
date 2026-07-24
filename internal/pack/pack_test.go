package pack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// nopTarget is a stand-in astro target for registry tests that never build.
type nopTarget struct{ name string }

func (n nopTarget) Name() string { return n.name }
func (n nopTarget) Build(context.Context, Request, localrt.Callbacks) (Result, error) {
	return Result{Target: n.name, Kind: KindImage}, nil
}

func TestRegistryNamesInListingOrder(t *testing.T) {
	r := NewRegistry(nopTarget{name: TargetAstro})
	assert.Equal(t, []string{TargetAstro, TargetMWAA, TargetComposer, TargetOSS}, r.Names())
}

func TestRegistryLookupAstro(t *testing.T) {
	astro := nopTarget{name: TargetAstro}
	r := NewRegistry(astro)
	got, err := r.Lookup(TargetAstro)
	require.NoError(t, err)
	assert.Equal(t, TargetAstro, got.Name())
}

func TestRegistryLookupUnknownListsKnown(t *testing.T) {
	r := NewRegistry(nopTarget{name: TargetAstro})
	_, err := r.Lookup("nope")
	require.Error(t, err)
	// The message names every registered target so the user can pick a real one.
	for _, name := range []string{TargetAstro, TargetMWAA, TargetComposer, TargetOSS} {
		assert.Contains(t, err.Error(), name)
	}
}

func TestStagedTargetsRefuseToBuild(t *testing.T) {
	r := NewRegistry(nopTarget{name: TargetAstro})
	for _, name := range []string{TargetMWAA, TargetComposer, TargetOSS} {
		target, err := r.Lookup(name)
		require.NoError(t, err)
		_, err = target.Build(context.Background(), Request{}, localrt.Callbacks{})
		var staged *StagedError
		require.ErrorAs(t, err, &staged, "target %s should return a StagedError", name)
		assert.Equal(t, name, staged.Target)
	}
}

func TestSortedCopyLeavesInputUntouched(t *testing.T) {
	in := []string{"c", "a", "b"}
	got := sortedCopy(in)
	assert.Equal(t, []string{"a", "b", "c"}, got)
	assert.Equal(t, []string{"c", "a", "b"}, in, "input must not be mutated")
}

// Sanity: the staged stub satisfies Target, so the registry is fully typed.
var _ Target = stagedTarget{}
