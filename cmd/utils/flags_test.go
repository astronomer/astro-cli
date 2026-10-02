package utils

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestAddBuildSecretFlag(t *testing.T) {
	newFlagSet := func() (*pflag.FlagSet, *[]string) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		target := []string{}
		AddBuildSecretFlag(flags, &target)
		return flags, &target
	}

	t.Run("collects repeated --build-secret values", func(t *testing.T) {
		flags, target := newFlagSet()
		err := flags.Parse([]string{"--build-secret", "id=one,src=one.txt", "--build-secret", "id=two,src=two.txt"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=one,src=one.txt", "id=two,src=two.txt"}, *target)
	})

	t.Run("the removed --build-secrets alias is an unknown flag", func(t *testing.T) {
		flags, _ := newFlagSet()
		err := flags.Parse([]string{"--build-secrets", "id=one,src=one.txt"})
		assert.ErrorContains(t, err, "unknown flag: --build-secrets")
	})
}
