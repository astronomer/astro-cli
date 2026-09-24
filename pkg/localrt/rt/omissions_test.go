package rt

import (
	"reflect"
	"testing"
)

func TestStandaloneOmissions(t *testing.T) {
	const dockerfile = "docker/Dockerfile"
	packages := []string{"libpq-dev", "unixodbc-dev"}
	cases := []struct {
		name string
		plan Plan
		want []Omission
	}{
		{"neither", Plan{}, nil},
		{
			"packages only",
			Plan{Packages: packages},
			[]Omission{{Kind: OmissionPackages, Packages: packages}},
		},
		{
			"dockerfile only",
			Plan{Dockerfile: dockerfile},
			[]Omission{{Kind: OmissionDockerfile, Dockerfile: dockerfile}},
		},
		{
			"both, Dockerfile first",
			Plan{Dockerfile: dockerfile, Packages: packages},
			[]Omission{
				{Kind: OmissionDockerfile, Dockerfile: dockerfile},
				{Kind: OmissionPackages, Packages: packages},
			},
		},
		{
			"explicit standalone is the same as the zero mode",
			Plan{Mode: ModeStandalone, Dockerfile: dockerfile, Packages: packages},
			[]Omission{
				{Kind: OmissionDockerfile, Dockerfile: dockerfile},
				{Kind: OmissionPackages, Packages: packages},
			},
		},
		{"docker mode honors both", Plan{Mode: ModeDocker, Dockerfile: dockerfile, Packages: packages}, nil},
		{"an empty packages list is not an omission", Plan{Packages: []string{}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.plan.StandaloneOmissions()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("StandaloneOmissions() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// The returned list is the caller's to keep: changing it must not reach back
// into the Plan that is about to be started.
func TestStandaloneOmissionsCopiesPackages(t *testing.T) {
	p := Plan{Packages: []string{"libpq-dev"}}
	got := p.StandaloneOmissions()
	got[0].Packages[0] = "changed"
	if p.Packages[0] != "libpq-dev" {
		t.Errorf("mutating the omission changed the plan: %v", p.Packages)
	}
}

// The kinds are strings a consumer keys its own copy off, so they are part of
// the contract.
func TestOmissionKindValues(t *testing.T) {
	if OmissionDockerfile != "dockerfile" || OmissionPackages != "packages" {
		t.Errorf("omission kinds changed: %q, %q", OmissionDockerfile, OmissionPackages)
	}
}
