package manifest

import (
	"reflect"
	"testing"
)

// An apc link is an Astro Private Cloud Deployment, linked beside Astro's:
// it names its own installation and workspace and inherits neither from
// [tool.astro], which describes Astro.
func TestAPCLink(t *testing.T) {
	m, err := Load(write(t, `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-astro"
domain = "astronomer.io"

[tool.astro.deployments.dev]
deployment = "dep-astro"

[tool.astro.deployments.prod-apc]
target = "apc"
domain = "https://Astro.Corp.Example/"
workspace = "ckws-apc"
deployment = "ckdep-apc"
default = true
`))
	if err != nil {
		t.Fatal(err)
	}
	got := m.Astro.Deployments["prod-apc"]
	want := Link{
		Target:     "apc",
		Workspace:  "ckws-apc",
		Deployment: "ckdep-apc",
		// Normalized the way astro login stores an APC login.
		Domain:  "astro.corp.example",
		Auth:    Auth{Method: AuthAPC},
		Default: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("prod-apc = %#v, want %#v", got, want)
	}
	if got.Kind() != KindAPC {
		t.Errorf("Kind() = %q, want %q", got.Kind(), KindAPC)
	}
	// The Astro link beside it is untouched: it still inherits [tool.astro]
	// workspace and names no domain of its own.
	if dev := m.Astro.Deployments["dev"]; dev.Kind() != KindAstro || dev.Workspace != "ws-astro" || dev.Domain != "" {
		t.Errorf("dev = %#v, want an astro link in ws-astro with no domain", dev)
	}
	// An apc link does not use the Astro login, so it alone would not give
	// [tool.astro] domain a use; the workspace here does.
	if name, _, ok := DefaultLink(m.Astro.Deployments); !ok || name != "prod-apc" {
		t.Errorf("DefaultLink = %q, %v, want prod-apc", name, ok)
	}
}

// An apc link proves itself with the APC login unless it says otherwise, and
// [tool.astro] domain is not justified by it: that names Astro's host, for a
// workspace or an astro-auth link.
func TestAPCLinkDoesNotJustifyTheAstroDomain(t *testing.T) {
	_, err := Load(write(t, `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
domain = "astronomer.io"

[tool.astro.deployments.prod-apc]
target = "apc"
domain = "astro.corp.example"
workspace = "ckws-apc"
deployment = "ckdep-apc"
`))
	ve := validationError(t, err)
	if codes := problemCodesOf(ve); !reflect.DeepEqual(codes, []ProblemCode{CodeDomainWithoutWorkspace}) {
		t.Errorf("codes = %v, want domain_without_workspace", codes)
	}
}

// warningCases are manifests that load with a warning, for the codes only a
// warning raises (TestEveryCodeIsReachable).
var warningCases = []string{astroLinkWithDomain}

// astroLinkWithDomain is an astro link carrying a domain, which nothing reads.
const astroLinkWithDomain = `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.dev]
deployment = "dep-xyz"
domain = "astronomer-dev.io"
`

// A domain on another kind of link is ignored with a warning, never refused:
// a manifest that carries one keeps loading.
func TestADomainOnAnAstroLinkIsAWarning(t *testing.T) {
	m, err := Load(write(t, astroLinkWithDomain))
	if err != nil {
		t.Fatalf("an astro link with a domain must still load: %v", err)
	}
	dev := m.Astro.Deployments["dev"]
	if dev.Kind() != KindAstro || dev.Deployment != "dep-xyz" || dev.Workspace != "ws-abc" || dev.Domain != "" {
		t.Errorf("dev = %#v, want the astro link with its domain dropped", dev)
	}
	if len(m.Warnings) != 1 || m.Warnings[0].Code != CodeDomainOnNonAPCLink || m.Warnings[0].Key != "tool.astro.deployments.dev.domain" {
		t.Errorf("warnings = %+v, want one domain_on_non_apc_link on the link's domain", m.Warnings)
	}
}

func TestKindWithArticle(t *testing.T) {
	for kind, want := range map[LinkKind]string{
		KindAstro: "an astro", KindAPC: "an apc", KindMWAA: "an mwaa", KindComposer: "a composer", KindEndpoint: "a url",
	} {
		if got := KindWithArticle(kind); got != want {
			t.Errorf("KindWithArticle(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestNormalizeAPCDomain(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"astro.corp.example", "astro.corp.example"},
		{" Astro.Corp.Example ", "astro.corp.example"},
		{"https://astro.corp.example/", "astro.corp.example"},
		{"http://astro.corp.example", "astro.corp.example"},
		{"cloud.corp.example", "cloud.corp.example"},
		{"app.astro.corp.example", "app.astro.corp.example"},
		{"https://", ""},
		{"", ""},
	} {
		if got := NormalizeAPCDomain(tc.in); got != tc.want {
			t.Errorf("NormalizeAPCDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
