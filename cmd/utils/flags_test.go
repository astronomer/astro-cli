package utils

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestAddBuildSecretFlags(t *testing.T) {
	newFlagSet := func() (*pflag.FlagSet, *[]string) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		target := []string{}
		AddBuildSecretFlags(flags, &target)
		return flags, &target
	}

	t.Run("collects repeated --build-secret values", func(t *testing.T) {
		flags, target := newFlagSet()
		err := flags.Parse([]string{"--build-secret", "id=one,src=one.txt", "--build-secret", "id=two,src=two.txt"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=one,src=one.txt", "id=two,src=two.txt"}, *target)
	})

	t.Run("deprecated --build-secrets feeds the same value", func(t *testing.T) {
		flags, target := newFlagSet()
		err := flags.Parse([]string{"--build-secrets", "id=one,src=one.txt"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=one,src=one.txt"}, *target)
	})

	t.Run("mixing both flags merges values in command-line order", func(t *testing.T) {
		flags, target := newFlagSet()
		err := flags.Parse([]string{
			"--build-secret", "id=one,src=one.txt",
			"--build-secrets", "id=two,src=two.txt",
			"--build-secret", "id=three,src=three.txt",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=one,src=one.txt", "id=two,src=two.txt", "id=three,src=three.txt"}, *target)
	})
}
