package manifest

import (
	"testing"
)

// unknownKeyCases are manifests carrying a key their table does not define,
// and is no near-miss of one it does (a near-miss is refused: nearmiss_test.go).
// Each loads, with a warning naming the key: a newer CLI or Astro Desktop may
// write a key this build does not know, and an older reader of a committed,
// shared file must still open the project.
var unknownKeyCases = []struct {
	name, content, wantKey string
}{
	{
		name:    "unknown key in [tool.astro]",
		content: "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\npalette = \"blue\"\n",
		wantKey: "tool.astro.palette",
	},
	{
		name: "unknown key on a link",
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "ws-abc"

[tool.astro.deployments.prod]
deployment = "dep-xyz"
notes = "the prod link"
`,
		wantKey: "tool.astro.deployments.prod.notes",
	},
	{
		name: "apc link with an organization",
		content: `
[project]
name = "p"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod-apc]
target = "apc"
domain = "astro.corp.example"
workspace = "ckws-apc"
deployment = "ckdep-apc"
organization = "clorg"
`,
		wantKey: "tool.astro.deployments.prod-apc.organization",
	},
	{
		name:    "a pool with a key Airflow has no field for",
		content: "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.pools]\netl = { slots = 4, open_slots = 2 }\n",
		wantKey: "tool.astro.pools.etl.open_slots",
	},
}

func TestAnUnknownKeyIsAWarningNotAProblem(t *testing.T) {
	for _, tc := range unknownKeyCases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(write(t, tc.content))
			if err != nil {
				t.Fatalf("an unknown key must not refuse the manifest: %v", err)
			}
			for _, w := range m.Warnings {
				if w.Code == CodeUnknownKey && w.Key == tc.wantKey {
					return
				}
			}
			t.Errorf("want an %s warning on %s, got %+v", CodeUnknownKey, tc.wantKey, m.Warnings)
		})
	}
}
