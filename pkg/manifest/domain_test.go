package manifest

import "testing"

func loadDomain(t *testing.T, body string) string {
	t.Helper()
	m, err := Load(write(t, "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\n"+body))
	if err != nil {
		t.Fatalf("Load(%q): %v", body, err)
	}
	return m.Astro.WorkspaceDomain()
}

// A workspace link names its host, and a manifest written before the key
// existed means production: the host every earlier link was made against.
func TestWorkspaceDomain(t *testing.T) {
	t.Setenv("ASTRO_DOMAIN", "")
	for body, want := range map[string]string{
		"workspace = \"cmws\"\n":                                     DefaultWorkspaceDomain,
		"workspace = \"cmws\"\ndomain = \"astronomer-dev.io\"\n":     "astronomer-dev.io",
		"workspace = \"cmws\"\ndomain = \" astronomer-stage.io \"\n": "astronomer-stage.io",
	} {
		if got := loadDomain(t, body); got != want {
			t.Errorf("WorkspaceDomain for %q = %q, want %q", body, got, want)
		}
	}
}

// A domain written as a copied URL or the cloud UI's host names the same login
// `astro login` stores, so the lookup meets it and the suggested fix does not
// store the next login under a different key.
func TestWorkspaceDomainIsNormalizedLikeALogin(t *testing.T) {
	t.Setenv("ASTRO_DOMAIN", "")
	for _, written := range []string{
		"https://cloud.astronomer-dev.io/",
		"cloud.astronomer-dev.io",
		"Astronomer-Dev.io",
		"http://astronomer-dev.io",
	} {
		if got := loadDomain(t, "workspace = \"cmws\"\ndomain = \""+written+"\"\n"); got != "astronomer-dev.io" {
			t.Errorf("WorkspaceDomain for %q = %q, want astronomer-dev.io", written, got)
		}
	}
}

// With no domain in the manifest, ASTRO_DOMAIN — the override CI sets beside
// ASTRO_API_TOKEN, and Astro Desktop's own host — is the host. An explicit
// domain still wins over it.
func TestWorkspaceDomainFallsBackToAstroDomain(t *testing.T) {
	t.Setenv("ASTRO_DOMAIN", "cloud.astronomer-dev.io")
	if got := loadDomain(t, "workspace = \"cmws\"\n"); got != "astronomer-dev.io" {
		t.Errorf("WorkspaceDomain = %q, want ASTRO_DOMAIN, normalized", got)
	}
	if got := loadDomain(t, "workspace = \"cmws\"\ndomain = \"astronomer.io\"\n"); got != "astronomer.io" {
		t.Errorf("WorkspaceDomain = %q, want the manifest's own domain over ASTRO_DOMAIN", got)
	}
}
