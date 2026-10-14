package cmd

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"testing"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// contextPlatform is three-way: no context is Astro, a context that resolves
// is its platform, and one named but not resolvable for a domain that is not
// Astro's, or a home config that cannot be read, is unresolved, never Astro.
func TestContextPlatform(t *testing.T) {
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })
	for _, tc := range []struct {
		name, platform, domain string
		want                   project.Context
	}{
		{name: "no context", platform: testUtil.Initial},
		{name: "an Astro context", platform: testUtil.CloudPlatform},
		{name: "an APC context", platform: testUtil.SoftwarePlatform, want: project.Context{APC: true}},
		{
			name: "ASTRO_DOMAIN naming a saved APC context that is current", platform: testUtil.SoftwarePlatform, domain: "astronomer_dev.com",
			want: project.Context{APC: true, FromASTRODomain: true},
		},
		{
			name: "ASTRO_DOMAIN naming an APC domain with no context", platform: testUtil.Initial, domain: "apc.example.com",
			want: project.Context{Unresolved: true, FromASTRODomain: true, UnsetIsAstro: true},
		},
		{
			name: "ASTRO_DOMAIN naming an unsaved APC domain over a saved Astro context", platform: testUtil.CloudPlatform, domain: "apc.example.com",
			want: project.Context{Unresolved: true, FromASTRODomain: true, UnsetIsAstro: true},
		},
		{
			name: "ASTRO_DOMAIN naming an unsaved APC domain over a saved APC context", platform: testUtil.SoftwarePlatform, domain: "apc.example.com",
			want: project.Context{Unresolved: true, FromASTRODomain: true},
		},
		{name: "ASTRO_DOMAIN naming Astro with no context", platform: testUtil.Initial, domain: "astronomer.io"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(tc.platform)
			t.Setenv("ASTRO_DOMAIN", tc.domain)
			if got := contextPlatform(); got != tc.want {
				t.Errorf("contextPlatform() = %+v; want %+v", got, tc.want)
			}
		})
	}

	t.Run("a home config that cannot be read", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		if err := afero.WriteFile(fs, config.HomeConfigFile, []byte("context: [unclosed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		config.InitConfig(fs)
		if got := contextPlatform(); got != (project.Context{Unresolved: true, UnreadableConfig: config.HomeConfigFile}) {
			t.Errorf("contextPlatform() = %+v; want unresolved", got)
		}
		// ASTRO_DOMAIN naming Astro decides, unreadable config or not.
		t.Setenv("ASTRO_DOMAIN", "astronomer.io")
		if got := contextPlatform(); got != (project.Context{}) {
			t.Errorf("with ASTRO_DOMAIN=astronomer.io: contextPlatform() = %+v; want Astro", got)
		}
	})
}

// contextPlatform reads which context is current and whether it is saved,
// never a login: GetCurrentContext and GetContext resolve the login, which can
// reach the OS keyring or the secrets vault, at startup of every command.
func TestContextPlatformReadsNoLogin(t *testing.T) {
	file, err := parser.ParseFile(gotoken.NewFileSet(), "contextplatform.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "GetCurrentContext", "GetContext", "ListContexts", "IsCloudContext":
				t.Errorf("contextPlatform calls %s, which reads a login", sel.Sel.Name)
			}
		}
		return true
	})
}
