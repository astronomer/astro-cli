package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBehindMajor(t *testing.T) {
	assert := assert.New(t)
	cv := "0.17.0"
	dv := "0.18.0"
	act := isBehindMajor(cv, dv)

	assert.True(act)
}
func TestIsBehindPatch(t *testing.T) {
	assert := assert.New(t)
	cv := "0.17.0"
	dv := "0.17.1"
	act := isBehindPatch(cv, dv)

	assert.True(act)
}

func TestIsAheadMajor(t *testing.T) {
	assert := assert.New(t)
	cv := "0.18.0"
	dv := "0.17.0"
	act := isAheadMajor(cv, dv)

	assert.True(act)
}

func TestFormatMajor(t *testing.T) {
	assert := assert.New(t)
	ver := "0.17.0"
	exp := "0.17"
	act := formatMajor(ver)

	assert.Equal(exp, act)
}

func TestFormatLtConstraint(t *testing.T) {
	assert := assert.New(t)
	con := "0.17.0"
	exp := "< 0.17.0"
	act := formatLtConstraint(con)

	assert.Equal(exp, act)
}

func TestFormatDowngradeConstraint(t *testing.T) {
	assert := assert.New(t)
	con := "0.17.0"
	exp := "> 0.17"
	act := formatDowngradeConstraint(con)

	assert.Equal(exp, act)
}

func TestGetConstraint(t *testing.T) {
	assert := assert.New(t)
	ver := getVersion("0.17.1")
	majGt := getConstraint("> 0.18.0")
	majLt := getConstraint("> 0.16.0")
	patchLt := getConstraint("< 0.17")
	patchGt := getConstraint("< 0.18")

	assert.NotNil(majGt)
	assert.False(majGt.Validate(ver))

	assert.NotNil(majLt)
	assert.True(majLt.Validate(ver))

	assert.NotNil(patchLt)
	assert.False(patchLt.Validate(ver))

	assert.NotNil(patchGt)
	assert.True(patchGt.Validate(ver))
}

func TestGetVersion(t *testing.T) {
	assert := assert.New(t)
	ver := getVersion("0.17.1")
	assert.NotNil(ver)
	assert.Equal(uint64(0), ver.Major())
	assert.Equal(uint64(17), ver.Minor())
	assert.Equal(uint64(1), ver.Patch())
}
