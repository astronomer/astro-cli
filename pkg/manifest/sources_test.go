package manifest

import (
	"reflect"
	"testing"
)

func TestRequirementsWriteGitAndURLSourcesAsDirectReferences(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\n"+
		"  \"apache-airflow==3.3.*\",\n"+
		"  \"example-lib\",\n"+
		"  \"Example_Dbt_Tool[cli]>=0.7 ; python_version >= '3.12'\",\n"+
		"  \"tagged\",\n"+
		"  \"wheel\",\n"+
		"  \"local\",\n"+
		"  \"linux-only\",\n"+
		"  \"pinned @ https://example.com/pinned.whl\",\n"+
		"  \"pandas<3\",\n"+
		"]\n\n[tool.astro]\n\n"+
		"[tool.uv.sources]\n"+
		"example-lib = { git = \"https://github.com/example-org/example-lib.git\", rev = \"4bfeaf8\" }\n"+
		"example-dbt-tool = { git = \"https://github.com/example-org/example-dbt.git\", subdirectory = \"clients/python\", rev = \"f64d7b9\" }\n"+
		"tagged = { git = \"git+ssh://git@github.com/acme/tagged\", tag = \"v1\" }\n"+
		"wheel = { url = \"https://example.com/wheel-1.0-py3-none-any.whl\" }\n"+
		"local = { path = \"../local\" }\n"+
		"linux-only = { git = \"https://github.com/acme/linux-only\", marker = \"sys_platform == 'linux'\" }\n"+
		"pinned = { git = \"https://github.com/acme/pinned\" }\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"apache-airflow==3.3.*",
		"example-lib @ git+https://github.com/example-org/example-lib.git@4bfeaf8",
		"Example_Dbt_Tool[cli] @ git+https://github.com/example-org/example-dbt.git@f64d7b9#subdirectory=clients/python ; python_version >= '3.12'",
		"tagged @ git+ssh://git@github.com/acme/tagged@v1",
		"wheel @ https://example.com/wheel-1.0-py3-none-any.whl",
		"local",
		"linux-only",
		"pinned @ https://example.com/pinned.whl",
		"pandas<3",
	}
	if got := m.Requirements(); !reflect.DeepEqual(got, want) {
		t.Errorf("Requirements() =\n%#v\nwant\n%#v", got, want)
	}
}

func TestRequirementsWithoutSourcesAreTheDependencies(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\"apache-airflow==3.3.*\", \"pandas<3\"]\n\n[tool.astro]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Requirements(); !reflect.DeepEqual(got, m.Project.Dependencies) {
		t.Errorf("Requirements() = %#v, want the dependencies %#v", got, m.Project.Dependencies)
	}
}

func TestANetrcBuildSecretNeedsNoDockerfile(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\nbuild-secrets = ['id=netrc,env=NETRC_CONTENT']\n"))
	if err != nil {
		t.Fatalf("a netrc secret without a dockerfile: %v", err)
	}
	if want := []string{"id=netrc,env=NETRC_CONTENT"}; !reflect.DeepEqual(m.Astro.BuildSecretSpecs(), want) {
		t.Errorf("BuildSecretSpecs() = %#v, want %#v", m.Astro.BuildSecretSpecs(), want)
	}
}

func TestRequirementsMatchNamesTheWayPEP503Does(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\ndependencies = [\"apache-airflow==3.3.*\", \"acme.lib [cli]>=1\"]\n\n[tool.astro]\n\n"+
		"[tool.uv.sources]\nacme-lib = { git = \"https://github.com/acme/lib\" }\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"apache-airflow==3.3.*", "acme.lib[cli] @ git+https://github.com/acme/lib"}
	if got := m.Requirements(); !reflect.DeepEqual(got, want) {
		t.Errorf("Requirements() = %#v, want %#v", got, want)
	}
}
