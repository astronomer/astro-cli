package airflowversions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAirflowVersion(t *testing.T) {
	av, err := NewAirflowVersion("2.0.0-1", []string{"2.0.0-1-buster-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, av.Major(), uint64(2))
	assert.Equal(t, av.Minor(), uint64(0))
	assert.Equal(t, av.Patch(), uint64(0))
	assert.Equal(t, av.post_n1, uint64(1))
}

func TestNewAirflowVersionWithoutPostN1(t *testing.T) {
	av, err := NewAirflowVersion("2.0.0", []string{"2.0.0-buster-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, av.Major(), uint64(2))
	assert.Equal(t, av.Minor(), uint64(0))
	assert.Equal(t, av.Patch(), uint64(0))
	assert.Equal(t, av.post_n1, uint64(0))
}

func TestNewAirflowVersionWithPostN1(t *testing.T) {
	av, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, av.Major(), uint64(1))
	assert.Equal(t, av.Minor(), uint64(10))
	assert.Equal(t, av.Patch(), uint64(5))
	assert.Equal(t, av.post_n1, uint64(11))
}

func TestCompareAirflowVersions(t *testing.T) {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, av1.Compare(av2), 1)
}
