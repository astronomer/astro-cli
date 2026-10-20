package runtimecatalog

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

func pointer(f func(context.Context) *runtimeversions.Catalog) uintptr {
	return reflect.ValueOf(f).Pointer()
}

// Only an Airflow 2 build, whose base has no fallback, waits the catalog's
// default timeout; any other build only picks a Python, and waits seconds.
func TestForBuildWaitsLongerOnlyForAirflow2(t *testing.T) {
	for pin, want := range map[string]func(context.Context) *runtimeversions.Catalog{
		"2":     ImageCatalog,
		"2.10":  ImageCatalog,
		"2.9.3": ImageCatalog,
		"3.1":   Catalog,
		"3":     Catalog,
		"":      Catalog,
		"20.1":  Catalog,
	} {
		assert.Equal(t, pointer(want), pointer(ForBuild(pin)), pin)
	}
}
