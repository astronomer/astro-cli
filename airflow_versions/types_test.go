package airflowversions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAirflowVersion(t *testing.T) {
	av, err := NewAirflowVersion("2.0.0-1", []string{"2.0.0-1-buster-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), av.Major())
	assert.Equal(t, uint64(0), av.Minor())
	assert.Equal(t, uint64(0), av.Patch())
	assert.Equal(t, uint64(1), av.postN1)
}

func TestNewAirflowVersionError(t *testing.T) {
	av, err := NewAirflowVersion("-1", []string{"2.0.0-1-buster-onbuild"})
	assert.Error(t, err)
	assert.Nil(t, av)
}

func TestNewAirflowVersionWithoutPostN1(t *testing.T) {
	av, err := NewAirflowVersion("2.0.0", []string{"2.0.0-buster-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), av.Major())
	assert.Equal(t, uint64(0), av.Minor())
	assert.Equal(t, uint64(0), av.Patch())
	assert.Equal(t, uint64(0), av.postN1)
}

func TestNewAirflowVersionWithPostN1(t *testing.T) {
	av, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), av.Major())
	assert.Equal(t, uint64(10), av.Minor())
	assert.Equal(t, uint64(5), av.Patch())
	assert.Equal(t, uint64(11), av.postN1)
}

func TestCompareAirflowVersions(t *testing.T) {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, 1, av1.Compare(av2))
}

func TestCompareAirflowVersionsMajor(t *testing.T) {
	av1, err := NewAirflowVersion("2.10.5-11", []string{"2.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, 1, av1.Compare(av2))
}

func TestCompareAirflowVersionsMinor(t *testing.T) {
	av1, err := NewAirflowVersion("1.11.5-11", []string{"1.11.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, 1, av1.Compare(av2))
}

func TestCompareAirflowVersionsPatch(t *testing.T) {
	av1, err := NewAirflowVersion("1.10.6", []string{"1.11.6-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, 1, av1.Compare(av2))
}

func TestGreaterThan(t *testing.T) {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	assert.NoError(t, err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	assert.NoError(t, err)
	assert.Equal(t, true, av1.GreaterThan(av2))
}

func Test_compareSegment(t *testing.T) {
	assert.Equal(t, 0, compareSegment(1, 1))
	assert.Equal(t, -1, compareSegment(1, 2))
	assert.Equal(t, 1, compareSegment(2, 1))
}
