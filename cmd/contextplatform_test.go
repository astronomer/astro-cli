package cmd

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// contextPlatform is three-way: no context is Astro, a context that resolves
// is its platform, and one named but not resolvable for a domain that is not
// Astro's, or a home config that cannot be read, is unresolved, never Astro.
func TestContextPlatform(t *testing.T) {
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })
	for _, tc := range []struct {
		name, platform, domain string
		apc, unresolved        bool
	}{
		{name: "no context", platform: testUtil.Initial},
		{name: "an Astro context", platform: testUtil.CloudPlatform},
		{name: "an APC context", platform: testUtil.SoftwarePlatform, apc: true},
		{name: "ASTRO_DOMAIN naming an APC domain with no context", platform: testUtil.Initial, domain: "apc.example.com", unresolved: true},
		{name: "ASTRO_DOMAIN naming Astro with no context", platform: testUtil.Initial, domain: "astronomer.io"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(tc.platform)
			t.Setenv("ASTRO_DOMAIN", tc.domain)
			apc, unresolved := contextPlatform()
			if apc != tc.apc || unresolved != tc.unresolved {
				t.Errorf("contextPlatform() = %v, %v; want %v, %v", apc, unresolved, tc.apc, tc.unresolved)
			}
		})
	}

	t.Run("a home config that cannot be read", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		if err := afero.WriteFile(fs, config.HomeConfigFile, []byte("context: [unclosed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		config.InitConfig(fs)
		if apc, unresolved := contextPlatform(); apc || !unresolved {
			t.Errorf("contextPlatform() = %v, %v; want false, true", apc, unresolved)
		}
		// ASTRO_DOMAIN naming Astro decides, unreadable config or not.
		t.Setenv("ASTRO_DOMAIN", "astronomer.io")
		if apc, unresolved := contextPlatform(); apc || unresolved {
			t.Errorf("with ASTRO_DOMAIN=astronomer.io: contextPlatform() = %v, %v; want false, false", apc, unresolved)
		}
	})
}
