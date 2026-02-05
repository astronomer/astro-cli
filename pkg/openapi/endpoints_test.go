package openapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractEndpoints(t *testing.T) {
	spec := &OpenAPISpec{
		Paths: map[string]PathItem{
			"/organizations": {
				Get: &Operation{
					OperationID: "listOrganizations",
					Summary:     "List organizations",
					Tags:        []string{"Organizations"},
				},
				Post: &Operation{
					OperationID: "createOrganization",
					Summary:     "Create organization",
					Tags:        []string{"Organizations"},
				},
			},
			"/organizations/{organizationId}": {
				Get: &Operation{
					OperationID: "getOrganization",
					Summary:     "Get organization",
					Tags:        []string{"Organizations"},
				},
				Delete: &Operation{
					OperationID: "deleteOrganization",
					Summary:     "Delete organization",
					Tags:        []string{"Organizations"},
					Deprecated:  true,
				},
			},
		},
	}

	endpoints := ExtractEndpoints(spec)

	assert.Len(t, endpoints, 4)

	// Check that endpoints are sorted by path, then method
	assert.Equal(t, "/organizations", endpoints[0].Path)
	assert.Equal(t, "GET", endpoints[0].Method)
	assert.Equal(t, "/organizations", endpoints[1].Path)
	assert.Equal(t, "POST", endpoints[1].Method)
	assert.Equal(t, "/organizations/{organizationId}", endpoints[2].Path)
	assert.Equal(t, "GET", endpoints[2].Method)
	assert.Equal(t, "/organizations/{organizationId}", endpoints[3].Path)
	assert.Equal(t, "DELETE", endpoints[3].Method)
	assert.True(t, endpoints[3].Deprecated)
}

func TestExtractEndpointsNilSpec(t *testing.T) {
	endpoints := ExtractEndpoints(nil)
	assert.Nil(t, endpoints)
}

func TestFilterEndpoints(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/organizations", Summary: "List organizations", Tags: []string{"Organizations"}},
		{Method: "POST", Path: "/organizations", Summary: "Create organization", Tags: []string{"Organizations"}},
		{Method: "GET", Path: "/deployments", Summary: "List deployments", Tags: []string{"Deployments"}},
		{Method: "GET", Path: "/users", Summary: "List users", Tags: []string{"Users"}},
	}

	// Filter by path
	filtered := FilterEndpoints(endpoints, "organization")
	assert.Len(t, filtered, 2)

	// Filter by method
	filtered = FilterEndpoints(endpoints, "POST")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "POST", filtered[0].Method)

	// Filter by tag
	filtered = FilterEndpoints(endpoints, "Deployments")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "/deployments", filtered[0].Path)

	// Filter by summary
	filtered = FilterEndpoints(endpoints, "users")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "/users", filtered[0].Path)

	// Empty filter returns all
	filtered = FilterEndpoints(endpoints, "")
	assert.Len(t, filtered, 4)

	// No matches
	filtered = FilterEndpoints(endpoints, "nonexistent")
	assert.Len(t, filtered, 0)
}

func TestFindEndpoint(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/organizations"},
		{Method: "POST", Path: "/organizations"},
		{Method: "GET", Path: "/deployments"},
	}

	// Find existing endpoint
	ep := FindEndpoint(endpoints, "GET", "/organizations")
	assert.NotNil(t, ep)
	assert.Equal(t, "GET", ep.Method)
	assert.Equal(t, "/organizations", ep.Path)

	// Find with lowercase method
	ep = FindEndpoint(endpoints, "get", "/organizations")
	assert.NotNil(t, ep)

	// Not found - wrong method
	ep = FindEndpoint(endpoints, "PUT", "/organizations")
	assert.Nil(t, ep)

	// Not found - wrong path
	ep = FindEndpoint(endpoints, "GET", "/nonexistent")
	assert.Nil(t, ep)
}

func TestFindEndpointByPath(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/organizations"},
		{Method: "POST", Path: "/organizations"},
		{Method: "GET", Path: "/deployments"},
	}

	// Find all endpoints for path
	matches := FindEndpointByPath(endpoints, "/organizations")
	assert.Len(t, matches, 2)

	// No matches
	matches = FindEndpointByPath(endpoints, "/nonexistent")
	assert.Len(t, matches, 0)
}

func TestExtractEndpointsAllMethods(t *testing.T) {
	spec := &OpenAPISpec{
		Paths: map[string]PathItem{
			"/resource": {
				Get:     &Operation{OperationID: "getResource"},
				Post:    &Operation{OperationID: "postResource"},
				Put:     &Operation{OperationID: "putResource"},
				Patch:   &Operation{OperationID: "patchResource"},
				Delete:  &Operation{OperationID: "deleteResource"},
				Options: &Operation{OperationID: "optionsResource"},
				Head:    &Operation{OperationID: "headResource"},
			},
		},
	}

	endpoints := ExtractEndpoints(spec)
	assert.Len(t, endpoints, 7)

	// Verify method ordering: GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD
	expected := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	for i, exp := range expected {
		assert.Equal(t, exp, endpoints[i].Method)
	}
}

func TestExtractEndpointsEmptyPaths(t *testing.T) {
	spec := &OpenAPISpec{Paths: map[string]PathItem{}}
	endpoints := ExtractEndpoints(spec)
	assert.Empty(t, endpoints)
}

func TestFindEndpointByOperationID(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/orgs", OperationID: "listOrganizations"},
		{Method: "POST", Path: "/orgs", OperationID: "createOrganization"},
		{Method: "GET", Path: "/deployments", OperationID: "listDeployments"},
	}

	t.Run("found exact", func(t *testing.T) {
		ep := FindEndpointByOperationID(endpoints, "listOrganizations")
		assert.NotNil(t, ep)
		assert.Equal(t, "GET", ep.Method)
		assert.Equal(t, "/orgs", ep.Path)
	})

	t.Run("found case-insensitive", func(t *testing.T) {
		ep := FindEndpointByOperationID(endpoints, "LISTORGANIZATIONS")
		assert.NotNil(t, ep)
		assert.Equal(t, "listOrganizations", ep.OperationID)
	})

	t.Run("not found", func(t *testing.T) {
		ep := FindEndpointByOperationID(endpoints, "nonexistent")
		assert.Nil(t, ep)
	})

	t.Run("empty list", func(t *testing.T) {
		ep := FindEndpointByOperationID(nil, "anything")
		assert.Nil(t, ep)
	})
}

func TestFilterEndpoints_ByOperationID(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/orgs", OperationID: "listOrganizations", Tags: []string{"Orgs"}},
		{Method: "POST", Path: "/orgs", OperationID: "createOrganization", Tags: []string{"Orgs"}},
		{Method: "GET", Path: "/deployments", OperationID: "listDeployments", Tags: []string{"Deployments"}},
	}

	filtered := FilterEndpoints(endpoints, "listOrg")
	assert.Len(t, filtered, 1)
	assert.Equal(t, "listOrganizations", filtered[0].OperationID)
}

func TestGetPathParameters(t *testing.T) {
	tests := []struct {
		path     string
		expected []string
	}{
		{"/organizations", nil},
		{"/organizations/{organizationId}", []string{"organizationId"}},
		{"/organizations/{organizationId}/deployments/{deploymentId}", []string{"organizationId", "deploymentId"}},
		{"/users/{userId}/roles/{roleId}/permissions/{permissionId}", []string{"userId", "roleId", "permissionId"}},
		{"/{a}/{b}/{c}", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			params := GetPathParameters(tt.path)
			assert.Equal(t, tt.expected, params)
		})
	}
}
