package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	gt := getConstraint("> 0.18.0")
	lt := getConstraint("> 0.16.0")
	plt := getConstraint("< 0.17")
	pgt := getConstraint("< 0.18")

	assert.NotNil(gt)
	assert.False(gt.Validate(ver))

	assert.NotNil(lt)
	assert.True(lt.Validate(ver))

	assert.NotNil(plt)
	assert.False(plt.Validate(ver))

	assert.NotNil(pgt)
	assert.True(pgt.Validate(ver))
}

func TestGetVersion(t *testing.T) {
	assert := assert.New(t)
	ver := getVersion("0.17.1")
	assert.NotNil(ver)
	assert.Equal(uint64(0), ver.Major())
	assert.Equal(uint64(17), ver.Minor())
	assert.Equal(uint64(1), ver.Patch())
}
