package cmd

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var removedV1FlagTrees = []string{testUtil.CloudPlatform, testUtil.SoftwarePlatform}

func TestRemovedV1FlagsResolve(t *testing.T) {
	found := map[int]bool{}
	for _, platform := range removedV1FlagTrees {
		testUtil.InitTestConfig(platform)
		root := NewRootCmd()
		for i, f := range removedV1Flags {
			c := findCommand(root, f.path)
			if c == nil {
				continue
			}
			found[i] = true
			flag := c.Flags().Lookup(f.name)
			require.NotNil(t, flag, "astro %s --%s (%s)", f.path, f.name, platform)
			assert.True(t, flag.Hidden, "astro %s --%s is listed as removed but shows in help (%s)", f.path, f.name, platform)
		}
	}
	for i, f := range removedV1Flags {
		assert.True(t, found[i], "astro %s: no such command in the cloud or APC tree, so --%s guards nothing", f.path, f.name)
	}
}

func TestRemovedV1FlagsSayWhatReplacedThem(t *testing.T) {
	for _, platform := range removedV1FlagTrees {
		for _, f := range removedV1Flags {
			spellings := []string{"--" + f.name}
			if !f.isBool {
				spellings[0] += "=x"
			}
			if f.shorthand != "" {
				spellings = append(spellings, "-"+f.shorthand)
			}
			for _, spelling := range spellings {
				testUtil.InitTestConfig(platform)
				root := NewRootCmd()
				if findCommand(root, f.path) == nil {
					continue
				}
				root.SetArgs(append(strings.Fields(f.path), spelling))
				root.SetOut(io.Discard)
				root.SetErr(io.Discard)
				err := root.Execute()
				assert.ErrorContains(t, err, f.msg, "astro %s %s (%s)", f.path, spelling, platform)
			}
		}
	}
}
