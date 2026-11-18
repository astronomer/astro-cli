package deployment

import (
	"errors"
	"testing"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// A client handed a fetched Deployment answers GetDeployment for it, in its
// Workspace, with no call at all (the mock has no expectations, so any call
// fails the test), and a name that is not its name is refused as ever.
func TestGetDeploymentTakesTheDeploymentItWasHanded(t *testing.T) {
	d := astrov1.Deployment{Id: "dep-1", Name: "etl", WorkspaceId: "ws-1"}
	client := WithFetched(new(astrov1_mocks.ClientWithResponsesInterface), d)

	got, err := GetDeployment("ws-1", "dep-1", "", ToRead, nil, client)
	if err != nil || got.Id != "dep-1" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err = GetDeployment("ws-1", "dep-1", "etl", ToChange, nil, client); err != nil || got.Id != "dep-1" {
		t.Fatalf("with its own name: got %+v, %v", got, err)
	}
	var disagree *TargetsDisagreeError
	if _, err = GetDeployment("ws-1", "dep-1", "other", ToChange, nil, client); !errors.As(err, &disagree) {
		t.Fatalf("with another name: err = %v, want *TargetsDisagreeError", err)
	}
}

// Only that Deployment, in that Workspace, is answered from hand.
func TestFetchedForIsOnlyThatDeploymentInItsWorkspace(t *testing.T) {
	d := astrov1.Deployment{Id: "dep-1", WorkspaceId: "ws-1"}
	client := WithFetched(new(astrov1_mocks.ClientWithResponsesInterface), d)
	for _, tt := range []struct {
		name, ws, id string
		want         bool
	}{
		{"the Deployment", "ws-1", "dep-1", true},
		{"another Deployment", "ws-1", "dep-2", false},
		{"another Workspace", "ws-2", "dep-1", false},
		{"no id", "ws-1", "", false},
	} {
		if _, ok := fetchedFor(client, tt.ws, tt.id); ok != tt.want {
			t.Errorf("%s: fetchedFor = %v, want %v", tt.name, ok, tt.want)
		}
	}
	if _, ok := fetchedFor(new(astrov1_mocks.ClientWithResponsesInterface), "ws-1", "dep-1"); ok {
		t.Error("a plain client was taken for a fetched one")
	}
}
