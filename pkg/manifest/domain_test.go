package manifest

import "testing"

func loadDomain(t *testing.T, body string) string {
	t.Helper()
	m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n"+body))
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

// A domain needs no workspace when a link proves itself with the Astro login:
// the domain then picks that login.
func TestDomainWithoutWorkspaceServesAnAstroLink(t *testing.T) {
	t.Setenv("ASTRO_DOMAIN", "")
	for name, link := range map[string]string{
		"astro link, default auth": "[tool.astro.deployments.prod]\nworkspace = \"cmws\"\ndeployment = \"dep\"\n",
		"url link, astro auth":     "[tool.astro.deployments.prod]\nurl = \"https://airflow.example.com\"\nauth = { method = \"astro\" }\n",
	} {
		m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\ndomain = \"astronomer-dev.io\"\n\n"+link))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := m.Astro.LoginDomain(); got != "astronomer-dev.io" {
			t.Errorf("%s: LoginDomain = %q, want astronomer-dev.io", name, got)
		}
	}
}

// LoginDomain has no default: a project that names no host, with no
// ASTRO_DOMAIN, uses whichever login is current.
func TestLoginDomain(t *testing.T) {
	for _, tc := range []struct{ domain, env, want string }{
		{"", "", ""},
		{"", "cloud.astronomer-dev.io", "astronomer-dev.io"},
		{"https://cloud.astronomer.io/", "astronomer-dev.io", "astronomer.io"},
	} {
		t.Setenv("ASTRO_DOMAIN", tc.env)
		a := Astro{Domain: tc.domain}
		if got := a.LoginDomain(); got != tc.want {
			t.Errorf("LoginDomain with domain %q, ASTRO_DOMAIN %q = %q, want %q", tc.domain, tc.env, got, tc.want)
		}
	}
}
