package astro

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
)

var snakeCaseKey = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// TestCloudListJSONKeysAreSnakeCase walks the json tags of every type the
// cloud list commands publish through pkg/output, nested types included, and
// fails on a key that is not snake_case. These shapes carried 1.x's camelCase
// until 2.0; a field added later with a camelCase tag, or a row that goes back
// to embedding a generated API type, fails here.
func TestCloudListJSONKeysAreSnakeCase(t *testing.T) {
	for _, v := range []any{
		deployment.DeploymentList{}, // deployment list
		deployment.BundleList{},     // deployment bundle list
		team.TeamList{},             // deployment, workspace, organization team list
		user.UserList{},             // deployment, workspace, organization user list
		organization.OrganizationList{},
		organization.ClusterList{},
		workspace.WorkspaceList{},
	} {
		typ := reflect.TypeOf(v)
		t.Run(typ.String(), func(t *testing.T) {
			for _, key := range jsonKeys(typ, typ.Name(), map[reflect.Type]bool{}) {
				name := key[strings.LastIndex(key, ".")+1:]
				assert.Regexp(t, snakeCaseKey, name, "key %s", key)
			}
		})
	}
}

// jsonKeys returns every json key a value of typ can publish, as dotted paths.
func jsonKeys(typ reflect.Type, path string, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || typ.PkgPath() == "time" || seen[typ] {
		return nil
	}
	seen[typ] = true
	var keys []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" && f.Anonymous {
			keys = append(keys, jsonKeys(f.Type, path, seen)...) // embedded: its fields are promoted
			continue
		}
		if name == "" {
			name = f.Name // encoding/json's default, which is never snake_case
		}
		keys = append(keys, path+"."+name)
		keys = append(keys, jsonKeys(f.Type, path+"."+name, seen)...)
	}
	return keys
}
