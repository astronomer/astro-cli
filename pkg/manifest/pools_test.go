package manifest

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadPools(t *testing.T) {
	m, err := Load(write(t, `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro.pools]
etl = {slots = 4, description = 'ETL loads'}
ml = {slots = 1, include_deferred = true}
unlimited = {slots = -1}
default_pool = {slots = 64, include_deferred = false}
`))
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	want := map[string]Pool{
		"etl":          {Slots: 4, Description: "ETL loads"},
		"ml":           {Slots: 1, IncludeDeferred: &yes},
		"unlimited":    {Slots: UnlimitedPoolSlots},
		"default_pool": {Slots: 64, IncludeDeferred: &no},
	}
	if !reflect.DeepEqual(m.Astro.Pools, want) {
		t.Errorf("pools = %+v, want %+v", m.Astro.Pools, want)
	}
}

func TestNoPoolsIsNil(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Astro.Pools != nil {
		t.Errorf("pools = %+v, want nil", m.Astro.Pools)
	}
}

func TestPoolNameLongerThanAirflowStoresIsRefused(t *testing.T) {
	name := strings.Repeat("p", maxPoolNameLength+1)
	_, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro.pools]\n"+name+" = {slots = 1}\n"))
	ve := validationError(t, err)
	if codes := problemCodesOf(ve); !reflect.DeepEqual(codes, []ProblemCode{CodePoolNameInvalid}) {
		t.Errorf("codes = %v", codes)
	}
}
