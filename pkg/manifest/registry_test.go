package manifest

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	for registry, ok := range map[string]bool{
		"registry.example.com/astronomer":  true,
		"registry.example.com/astronomer/": true,
		"localhost:5000":                   true,
		"mirror":                           true,
		"my-corp.jfrog.io/docker-remote/a": true,
		"https://registry.example.com":     false,
		"registry.example.com/runtime:3.3": false,
		"Registry.Example.com":             false,
		"registry.example.com/x@sha256:ab": false,
	} {
		t.Run(registry, func(t *testing.T) {
			m, err := Parse([]byte("[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.3.*\"]\n\n[tool.astro]\nregistry = '" + registry + "'\n"))
			if ok {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if want := strings.TrimRight(registry, "/"); m.Astro.Registry != want {
					t.Fatalf("Registry = %q, want %q", m.Astro.Registry, want)
				}
				return
			}
			var verr *ValidationError
			if !errors.As(err, &verr) || verr.Problems[0].Code != CodeRegistryInvalid {
				t.Fatalf("Parse error = %v, want %s", err, CodeRegistryInvalid)
			}
		})
	}
}
