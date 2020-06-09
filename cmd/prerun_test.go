package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConstraint(t *testing.T) {
	assert := assert.New(t)
	v := getVersion("0.17.1")
	gt := getConstraint("> 0.18.0")
	lt := getConstraint("> 0.16.0")
	p := getConstraint("< 0.17")
	pgt := getConstraint("< 0.18")

	assert.NotNil(gt)
	assert.False(gt.Validate(v))

	assert.NotNil(lt)
	assert.True(lt.Validate(v))

	assert.NotNil(p)
	assert.False(p.Validate(v))

	assert.NotNil(pgt)
	assert.True(pgt.Validate(v))
}

func TestGetVersion(t *testing.T) {
	assert := assert.New(t)
	v := getVersion("0.17.1")
	assert.NotNil(t, v)
	assert.Equal(uint64(0), v.Major())
	assert.Equal(uint64(17), v.Minor())
	assert.Equal(uint64(1), v.Patch())
}
