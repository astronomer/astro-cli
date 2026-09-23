package astro

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// Connection never had the upsert that variable and airflow-variable did, so
// `update` on a key that did not exist simply failed. This is the behavior
// that changed, and the one thing in this change that is not a rename.
func TestEnvConnSetCreatesWhenAbsent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "db_main")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
			return body.ObjectKey == "db_main" &&
				body.ObjectType == astrov1.CreateEnvironmentObjectRequestObjectTypeCONNECTION
		}),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: cuid.New()},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--type", "postgres", "--host", "db.example.com")
	assert.NoError(t, err)
	assert.Contains(t, out, "Created db_main")
	mc.AssertExpectations(t)
}

// --no-create is the typo guard: it gets back the old strict behavior without
// asking `set` to mean something at odds with its name. No create call is
// mocked, so the test fails if one is made.
func TestEnvConnSetNoCreateRefusesToCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "db_typo")
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_typo", "--workspace-id", "ws-test",
		"--type", "postgres", "--no-create")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	mc.AssertExpectations(t)
}

// Metrics-export is the one noun where update and create do not need the same
// flags, so a set that has to create can be missing something an update never
// wanted. The error has to name what and say why, or the user is left to infer
// it from an API rejection.
func TestEnvMetricsSetNamesWhatCreatingNeeds(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "prom_main")
	astroV1Client = mc

	_, err := execEnvCmd("metrics-export", "set", "prom_main", "--workspace-id", "ws-test",
		"--label", "env=prod")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist and could not be created")
	assert.Contains(t, err.Error(), "--endpoint")
	// And deliberately no --no-create hint: this arm means the create was
	// genuine and a required flag is missing, so suggesting the flag that
	// suppresses creating tells the user to abandon what they were doing.
	assert.NotContains(t, err.Error(), "--no-create")
	mc.AssertExpectations(t)
}

// `update` is gone on every noun, and it is a tombstone rather than nothing
// because cobra answers an unknown subcommand by printing help and exiting 0
// — a stale `astro env variable update FOO` would have looked like success.
//
// Every noun is covered, not just the two whose semantics changed, because
// the point of dropping the alias is that the one write verb is one verb
// everywhere.
func TestUpdateIsATombstoneOnEveryNoun(t *testing.T) {
	for _, tc := range []struct{ noun, want string }{
		{"var", "astro env variable set"},
		{"connection", "astro env connection set"},
		{"airflow-variable", "astro env airflow-variable set"},
		{"metrics-export", "astro env metrics-export set"},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(tc.noun, "update", "FOO", "--value", "bar")
			assert.Error(t, err, "update must fail, not print help and exit 0")
			assert.Contains(t, err.Error(), "was removed in v2")
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), "--no-create",
				"the guidance has to name the flag that restores the old failure")
			mc.AssertExpectations(t)
		})
	}
}

// The short alias goes with the verb.
func TestUpShortAliasIsAlsoATombstone(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "up", "FOO", "--value", "bar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "was removed in v2")
	mc.AssertExpectations(t)
}

// An unknown subcommand has to fail. Cobra's default for a group with no Run
// is to print help and exit 0, which reads as success to a script — the whole
// reason the removed verbs need tombstones rather than simply being absent.
func TestUnknownEnvSubcommandFailsAndSuggests(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"bogus"}, ""},
		//nolint:misspell // a deliberate typo: the case exists to prove the suggestion fires, and misspell --fix would otherwise repair it into a valid command
		{[]string{"conection", "set", "x"}, "connection"},
		{[]string{"variable", "seet", "x"}, "set"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(tc.args...)
			assert.Error(t, err, "%v: want an error, not help with a zero exit", tc.args)
			assert.Contains(t, err.Error(), "unknown command")
			if tc.want != "" {
				assert.Contains(t, err.Error(), tc.want, "should suggest the near miss")
			}
			mc.AssertExpectations(t)
		})
	}
}

// A create prints the new object's id. `create` did and `set` has to keep
// doing it, because it is the only way to learn the id without a second call
// and scripts read it out of this line.
func TestSetPrintsTheIDOfWhatItCreated(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	const newID = "cabc12def0123456789012345"
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "FOO")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.CreateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.CreateEnvironmentObject{Id: newID},
		}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "set", "FOO", "--workspace-id", "ws-test", "--value", "bar")
	assert.NoError(t, err)
	assert.Contains(t, out, "Created FOO (id: "+newID+")")
	mc.AssertExpectations(t)
}

// `create` is a tombstone rather than an alias, so it must fail and say what
// to run instead. Flag parsing is off on the stub, which is what lets an old
// invocation carrying --key reach the guidance instead of dying on the flag.
func TestEnvCreateIsATombstoneNamingTheNewForm(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	for _, tc := range []struct{ noun, want string }{
		{"var", "astro env variable set"},
		{"connection", "astro env connection set"},
		{"airflow-variable", "astro env airflow-variable set"},
		{"metrics-export", "astro env metrics-export set"},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(tc.noun, "create", "--key", "FOO", "--value", "bar")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "was removed in v2")
			assert.Contains(t, err.Error(), tc.want)
			mc.AssertExpectations(t)
		})
	}
}

// The bulk invocation passed no key, so the general "the key is now
// positional" advice would send the user at a form `set` rejects.
func TestCreateTombstonePointsBulkCallersAtFromFile(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "create", "--workspace-id", "ws-test", "--from-file", ".env")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "astro env variable set --from-file")
	assert.NotContains(t, err.Error(), "positional")
	mc.AssertExpectations(t)
}

// --value takes the whole connection, in the shape `astro local env connection
// set` takes it. The assertion that matters is not that a URI parses, but that
// it lands as the same object the field flags would produce — the two shapes
// have to describe one connection, or offering both is worse than offering one.
func TestEnvConnSetAcceptsAWholeConnectionURI(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	var viaURI, viaFlags *astrov1.CreateEnvironmentObjectConnectionRequest

	capture := func(into **astrov1.CreateEnvironmentObjectConnectionRequest) *astrov1_mocks.ClientWithResponsesInterface {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		expectAbsent(mc, "db_main")
		mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
				if body.Connection != nil {
					*into = body.Connection
				}
				return true
			}),
		).Return(&astrov1.CreateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
		}, nil).Once()
		return mc
	}

	mc := capture(&viaURI)
	astroV1Client = mc
	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--value", "postgres://admin:pw@db.example.com:5432/warehouse")
	assert.NoError(t, err)
	mc.AssertExpectations(t)

	resetEnvFlags()
	mc = capture(&viaFlags)
	astroV1Client = mc
	_, err = execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--type", "postgres", "--host", "db.example.com", "--login", "admin",
		"--password", "pw", "--port", "5432", "--schema", "warehouse")
	assert.NoError(t, err)
	mc.AssertExpectations(t)

	if assert.NotNil(t, viaURI) && assert.NotNil(t, viaFlags) {
		assert.Equal(t, viaFlags.Type, viaURI.Type, "conn type")
		assert.Equal(t, deref(viaFlags.Host), deref(viaURI.Host), "host")
		assert.Equal(t, deref(viaFlags.Login), deref(viaURI.Login), "login")
		assert.Equal(t, deref(viaFlags.Password), deref(viaURI.Password), "password")
		assert.Equal(t, deref(viaFlags.Port), deref(viaURI.Port), "port")
		assert.Equal(t, deref(viaFlags.Schema), deref(viaURI.Schema), "schema")
	}
}

// deref reads an optional request field for comparison. Comparing the pointers
// themselves passes or fails correctly but prints addresses, which says nothing
// about which field drifted.
func deref[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// Connection JSON is the other half of what local accepts, and it is what
// `astro local env connection get` prints — so it has to round-trip into the
// cloud side without an edit.
func TestEnvConnSetAcceptsConnectionJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "http_api")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
			return body.Connection != nil && body.Connection.Type == "http" &&
				body.Connection.Host != nil && *body.Connection.Host == "api.example.com"
		}),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "http_api", "--workspace-id", "ws-test",
		"--value", `{"conn_type":"http","host":"api.example.com"}`)
	assert.NoError(t, err)
	mc.AssertExpectations(t)
}

// --value and the field flags describe the same thing two ways, so taking both
// at once has no meaning. Cobra refuses the combination.
func TestEnvConnSetRefusesValueWithFieldFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--value", "postgres://h/db", "--host", "other.example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "none of the others can be")
	mc.AssertExpectations(t)
}

// Neither shape given: the old MarkFlagRequired("type") could not express
// "type or value", so the either/or moved into the runner and has to say both.
func TestEnvConnSetNeedsATypeFromSomewhere(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test", "--host", "h")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--type")
	assert.Contains(t, err.Error(), "--value")
	mc.AssertExpectations(t)
}

// Creating a connection needs nothing that updating one does not: both
// CreateConn and UpdateConn require exactly a type. That equivalence is why
// the connection upsert carries no completeness check while the metrics-export
// one does, so it is pinned rather than left as a reading of the code.
//
// If the platform ever makes another field required to create a connection,
// the first case here starts failing — which is the signal to add the guard
// runEnvMetricsSet already has.
func TestCreatingAConnectionNeedsNothingUpdatingDoesNot(t *testing.T) {
	t.Run("a type alone is enough to create", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		defer resetEnvFlags()

		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		expectAbsent(mc, "bare")
		mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
				return body.Connection != nil && body.Connection.Type == "fs"
			}),
		).Return(&astrov1.CreateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
		}, nil).Once()
		astroV1Client = mc

		out, err := execEnvCmd("connection", "set", "bare", "--workspace-id", "ws-test", "--type", "fs")
		assert.NoError(t, err, "a type-only connection is legitimate (fs, aws-with-instance-role); "+
			"if this now fails, creating needs more than updating and the upsert needs a guard")
		assert.Contains(t, out, "Created bare")
		mc.AssertExpectations(t)
	})

	t.Run("and without a type neither path is reachable", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		defer resetEnvFlags()

		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		astroV1Client = mc

		_, err := execEnvCmd("connection", "set", "bare", "--workspace-id", "ws-test", "--host", "h")
		assert.Error(t, err)
		mc.AssertExpectations(t)
	})
}

// The mistyped-key hazard is what --no-create exists for, and it is the same
// hazard on every noun. This is the rotation case that motivated the question:
// patching a password against a key that turns out not to exist must fail
// rather than leave a new connection behind.
func TestNoCreateMakesAPasswordRotationSafe(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "db_mian")
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_mian", "--workspace-id", "ws-test",
		"--type", "postgres", "--password", "newpw", "--no-create")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	mc.AssertExpectations(t)
}

// Platform connection keys are not environment-variable names — they may be
// hyphenated or dotted — so --value must not impose that rule. It did: routing
// through airflowenv.NormalizeConn ended in the AIRFLOW_CONN_<ID> encode, which
// rejects such a key with "could not encode value", naming neither cause nor fix,
// while the field flags accepted the same key happily.
func TestEnvConnSetValueAcceptsKeysThatAreNotEnvVarNames(t *testing.T) {
	for _, key := range []string{"db-main", "my.db"} {
		t.Run(key, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			expectAbsent(mc, key)
			mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
				mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
					return body.ObjectKey == key && body.Connection != nil && body.Connection.Type == "postgres"
				}),
			).Return(&astrov1.CreateEnvironmentObjectResponse{
				HTTPResponse: &http.Response{StatusCode: 200},
				JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
			}, nil).Once()
			astroV1Client = mc

			_, err := execEnvCmd("connection", "set", key, "--workspace-id", "ws-test",
				"--value", "postgres://admin:pw@db.example.com:5432/warehouse")
			assert.NoError(t, err)
			mc.AssertExpectations(t)
		})
	}
}

// A URI that names no port must not invent one. Taking the address of the
// decoded zero sent port 0, which Airflow then dials; the field-flag path
// leaves it unset. The existing parity test passes --port on both sides and
// so cannot see this.
func TestEnvConnSetValueSendsNoPortWhenTheURIHasNone(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	var got *astrov1.CreateEnvironmentObjectConnectionRequest
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "http_api")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
			got = body.Connection
			return true
		}),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "http_api", "--workspace-id", "ws-test",
		"--value", `{"conn_type":"http","host":"api.example.com"}`)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Nil(t, got.Port, "a connection with no port must send no port, not port 0")
	}
	mc.AssertExpectations(t)
}

// Every URI in the help has to parse. The documented example did not: a
// <password> placeholder added to quiet gosec is not valid userinfo, so the
// one command the help tells a user to copy failed with "value is neither
// JSON nor a URI".
func TestConnectionExamplesParse(t *testing.T) {
	for _, line := range strings.Split(envConnExamples, "\n") {
		idx := strings.Index(line, "--value '")
		if idx < 0 {
			continue
		}
		rest := line[idx+len("--value '"):]
		end := strings.Index(rest, "'")
		if !assert.GreaterOrEqual(t, end, 0, "unterminated --value in example: %s", line) {
			continue
		}
		value := rest[:end]
		_, err := parseWholeConn("db_main", value)
		assert.NoError(t, err, "example value does not parse: %s", value)
	}
}

// stdin not being a terminal is not consent to clear a password. Every
// non-interactive run without --password used to read zero bytes and send an
// explicit empty password, which this API treats as "clear this field".
func TestEnvConnSetLeavesThePasswordAloneWithoutTheFlag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	var got *astrov1.UpdateEnvironmentObjectConnectionRequest
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
				{Id: &id, ObjectKey: "db_main"},
			}, TotalCount: 1},
		}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id,
		mock.MatchedBy(func(body astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			got = body.Connection
			return true
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "db_main"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--type", "postgres", "--host", "new.example.com")
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Nil(t, got.Password, "no --password means leave it alone, not clear it")
	}
	mc.AssertExpectations(t)
}

// A --no-create refusal has to name the flag that caused it, or it is
// indistinguishable from any other lookup miss.
func TestNoCreateFailuresNameTheFlag(t *testing.T) {
	for _, tc := range []struct {
		noun, key string
		args      []string
	}{
		{"connection", "db_typo", []string{"--type", "postgres"}},
		{"var", "KEY_TYPO", []string{"--value", "x"}},
		{"airflow-variable", "av_typo", []string{"--value", "x"}},
		{"metrics-export", "prom_typo", []string{"--endpoint", "https://p.example.com", "--exporter-type", "PROMETHEUS"}},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			expectAbsent(mc, tc.key)
			astroV1Client = mc

			args := append([]string{tc.noun, "set", tc.key, "--workspace-id", "ws-test", "--no-create"}, tc.args...)
			_, err := execEnvCmd(args...)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "--no-create")
			mc.AssertExpectations(t)
		})
	}
}

// The tombstone's bulk branch only applies to the nouns that have --from-file.
// Pointing a connection caller at `set --from-file` trades a dead verb for a
// dead flag.
func TestCreateTombstoneOnlyOffersFromFileWhereItExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	for _, noun := range []string{"connection", "metrics-export"} {
		t.Run(noun, func(t *testing.T) {
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(noun, "create", "--from-file", "x.env")
			assert.Error(t, err)
			assert.NotContains(t, err.Error(), "--from-file")
			assert.Contains(t, err.Error(), "set <id-or-key>")
			mc.AssertExpectations(t)
		})
	}
}

// --strict was renamed, not removed. A stale script should keep working and
// be told the new name, rather than dying on "unknown flag" — the care the
// tombstone gives `create`, for the path that actually survived.
func TestStrictStillWorksAsADeprecatedAliasOfNoCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "KEY_TYPO")
	astroV1Client = mc

	_, err := execEnvCmd("var", "set", "KEY_TYPO", "--workspace-id", "ws-test", "--value", "x", "--strict")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--no-create", "the deprecated flag must still refuse to create")
	mc.AssertExpectations(t)
}

// A piped password still reaches the connection: the fix for the blanking bug
// narrows what an EMPTY read means, it does not remove the pipe as a way to
// supply a secret without putting it in shell history.
func TestEnvConnSetReadsAPipedPassword(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	go func() {
		_, _ = w.WriteString("s3cr3t\n")
		_ = w.Close()
	}()

	var got *astrov1.CreateEnvironmentObjectConnectionRequest
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "db_main")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
			got = body.Connection
			return true
		}),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--type", "postgres", "--host", "h")
	assert.NoError(t, err)
	if assert.NotNil(t, got) && assert.NotNil(t, got.Password) {
		assert.Equal(t, "s3cr3t", *got.Password)
	}
	mc.AssertExpectations(t)
}

// The connection password bug had a twin here with a narrower trigger: gated
// on --auth-type BASIC rather than on any non-TTY stdin, but the same fault.
// Re-asserting the auth type while changing something else must not clear the
// stored password just because CI has no terminal.
func TestEnvMetricsSetLeavesThePasswordAloneWithoutTheFlag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	var got *astrov1.UpdateEnvironmentObjectMetricsExportRequest
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
				{Id: &id, ObjectKey: "prom_main"},
			}, TotalCount: 1},
		}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id,
		mock.MatchedBy(func(body astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			got = body.MetricsExport
			return true
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "prom_main"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("metrics-export", "set", "prom_main", "--workspace-id", "ws-test",
		"--auth-type", "BASIC", "--username", "newuser")
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Nil(t, got.Password, "no --password means leave it alone, not clear it")
	}
	mc.AssertExpectations(t)
}

// Help reads the same on every noun and, deliberately, the same as
// `astro local env`: reads first, then the write, then the destructive one.
// Order here is AddCommand order, which nothing else would catch drifting —
// and the two trees were shipped in different orders until this was pinned.
func TestEnvVerbOrderIsUniform(t *testing.T) {
	// AddCommand order is what help renders only because cmd/root.go:78 sets
	// cobra.EnableCommandSorting = false process-wide. This test builds the env
	// group directly, without the real root, so it has to say so itself —
	// otherwise Commands() comes back alphabetical and this measures nothing
	// that a user ever sees.
	sorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	defer func() { cobra.EnableCommandSorting = sorting }()

	root := newEnvRootCmd(new(bytes.Buffer))
	for _, tc := range []struct {
		noun string
		want []string
	}{
		{"variable", []string{"list", "get", "export", "set", "delete", "link"}},
		{"connection", []string{"list", "get", "set", "delete"}},
		{"airflow-variable", []string{"list", "get", "set", "delete"}},
		{"metrics-export", []string{"list", "get", "set", "delete"}},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			cmd, _, err := root.Find([]string{tc.noun})
			if !assert.NoError(t, err) {
				return
			}
			var got []string
			for _, sub := range cmd.Commands() {
				if sub.IsAvailableCommand() {
					got = append(got, sub.Name())
				}
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// Every group in the env tree must fail an unknown subcommand.
//
// Cobra's default for a group with no Run is to print help and exit 0, so
// `astro env variable link creat …` reported success and created nothing.
// The env root and the four nouns were each fixed by hand and the fifth group
// was missed, which is what a structural test prevents and five hand-placed
// RunE lines do not. cmd/local's TestTreeInvariants does the same job there.
func TestEveryEnvGroupRejectsAnUnknownSubcommand(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		subs := cmd.Commands()
		if len(subs) > 0 {
			// A group: it must not answer an unknown child with help+exit 0.
			if cmd.Run == nil && cmd.RunE == nil {
				t.Errorf("%q has subcommands but no Run/RunE: cobra will print help and exit 0 for an unknown one",
					cmd.CommandPath())
			}
			if cmd.Args == nil {
				t.Errorf("%q has subcommands but no Args: it cannot see what was typed",
					cmd.CommandPath())
			}
		}
		for _, sub := range subs {
			walk(sub)
		}
	}
	walk(newEnvRootCmd(new(bytes.Buffer)))
}

// The env group lists its nouns and then the cross-kind `list`, the same order
// `astro local env` uses — where the list is last because it is the odd one
// out rather than a fifth noun. cmd/local pins the same sequence, and the two
// drifted the moment this command was added.
func TestEnvGroupListsTheNounsThenTheCrossKindList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	sorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	defer func() { cobra.EnableCommandSorting = sorting }()

	var got []string
	for _, sub := range newEnvRootCmd(new(bytes.Buffer)).Commands() {
		if sub.IsAvailableCommand() {
			got = append(got, sub.Name())
		}
	}
	assert.Equal(t,
		[]string{"variable", "connection", "airflow-variable", "metrics-export", "list"},
		got)
}

// stdin not being a terminal is not a value. In CI `readSecretValue` read
// zero bytes and `set` wrote an explicit empty value over the stored one,
// printing "Updated" and exiting 0 — and once `set` upserted, the same
// invocation against a mistyped key created an empty variable.
func TestEnvVarSetRefusesToInventAnEmptyValue(t *testing.T) {
	for _, noun := range []string{"var", "airflow-variable"} {
		t.Run(noun, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(noun, "set", "API_TOKEN", "--workspace-id", "ws-test")
			assert.Error(t, err, "no --value and nothing piped must not write an empty value")
			assert.Contains(t, err.Error(), "no value supplied")
			mc.AssertExpectations(t)
		})
	}
}

// An explicit empty value is still legitimate — that is setting a variable to
// the empty string, which is different from supplying nothing.
func TestEnvVarSetAllowsAnExplicitEmptyValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
				{Id: &id, ObjectKey: "BLANK"},
			}, TotalCount: 1},
		}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id, mock.Anything).
		Return(&astrov1.UpdateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "BLANK"},
		}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("var", "set", "BLANK", "--workspace-id", "ws-test", "--value", "")
	assert.NoError(t, err)
	mc.AssertExpectations(t)
}

// `export` without --include-secrets writes `KEY=  # secret, …` for every
// secret, which parses back as "". Importing that would set each secret to the
// empty string — and `set --from-file` is the advertised partner of `export`.
func TestBulkSetSkipsEmptyValuesRatherThanBlankingSecrets(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	dir := t.TempDir()
	path := dir + "/.env"
	body := "PLAIN=value\nSECRET_KEY=  # secret, use --include-secrets\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	id := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	// Only PLAIN is looked up and written; SECRET_KEY must not be touched.
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ObjectKey != nil && *p.ObjectKey == "PLAIN"
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{Id: &id, ObjectKey: "PLAIN"},
		}, TotalCount: 1},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id, mock.Anything).
		Return(&astrov1.UpdateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "PLAIN"},
		}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "set", "--workspace-id", "ws-test", "--from-file", path)
	assert.NoError(t, err)
	assert.Contains(t, out, "Updated PLAIN")
	mc.AssertExpectations(t)
}

// A URI is the one shape with nowhere safe to put a secret, so --password has
// to be allowed alongside it — and a URI that carries no password must not
// clear the stored one.
func TestEnvConnSetValueTakesAPasswordAlongside(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	var got *astrov1.CreateEnvironmentObjectConnectionRequest
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectAbsent(mc, "db_main")
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
			got = body.Connection
			return true
		}),
	).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
	}, nil).Once()
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--value", "postgres://admin@db.example.com:5432/warehouse", "--password", "hunter2")
	assert.NoError(t, err)
	if assert.NotNil(t, got) && assert.NotNil(t, got.Password) {
		assert.Equal(t, "hunter2", *got.Password)
	}
	mc.AssertExpectations(t)
}

// `db.example.com:5432/warehouse` parses as scheme "db.example.com", so
// ConnFromURI would return a connection whose type is a hostname — and
// CreateConn accepts it, because the type is non-empty.
func TestEnvConnSetValueRejectsASchemelessURI(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("connection", "set", "db_main", "--workspace-id", "ws-test",
		"--value", "db.example.com:5432/warehouse")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no scheme")
	mc.AssertExpectations(t)
}

// The tombstone may only offer --strict to the nouns that have it; the other
// two never did, so following the advice produced "unknown flag".
func TestUpdateTombstoneOnlyOffersStrictWhereItExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	for _, tc := range []struct {
		noun string
		has  bool
	}{
		{"var", true},
		{"airflow-variable", true},
		{"connection", false},
		{"metrics-export", false},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mc

			_, err := execEnvCmd(tc.noun, "update", "FOO")
			assert.Error(t, err)
			if tc.has {
				assert.Contains(t, err.Error(), "--strict")
			} else {
				assert.NotContains(t, err.Error(), "--strict")
			}
			mc.AssertExpectations(t)
		})
	}
}

// `link create` is a tombstone like the nouns', but with its own guidance: a
// link is addressed by two flags, not a positional, and the semantics of
// omitting --value changed at the same time.
func TestLinkCreateIsATombstoneNamingTheLinkForm(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("var", "link", "create", "--variable-key", "K", "--deployment-id", "d")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "was removed in v2")
	assert.Contains(t, err.Error(), "astro env variable link set --variable-key")
	assert.Contains(t, err.Error(), "CLEARS")
	mc.AssertExpectations(t)
}

// Mapping the id branch's 404 to ErrNotFound let --no-create and the upsert
// see a miss, and handed the create arm a CUID to use as the new key — so a
// stale id would have created an object literally named cl9abc…
func TestSetRefusesToCreateAnObjectNamedAfterAnID(t *testing.T) {
	staleID := cuid.New()
	for _, tc := range []struct {
		noun string
		args []string
	}{
		{"var", []string{"--value", "x"}},
		{"airflow-variable", []string{"--value", "x"}},
		{"connection", []string{"--type", "postgres"}},
		{"metrics-export", []string{"--endpoint", "https://p.example.com", "--exporter-type", "PROMETHEUS"}},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			defer resetEnvFlags()

			// No create is mocked: the test fails if one is attempted.
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mc.On("GetEnvironmentObjectWithResponse", mock.Anything, mock.Anything, staleID).
				Return(&astrov1.GetEnvironmentObjectResponse{
					HTTPResponse: &http.Response{StatusCode: 404},
				}, nil).Once()
			astroV1Client = mc

			args := append([]string{tc.noun, "set", staleID, "--workspace-id", "ws-test"}, tc.args...)
			_, err := execEnvCmd(args...)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "an ID cannot be created")
			mc.AssertExpectations(t)
		})
	}
}

// The listing asks for each type in turn rather than omitting the filter and
// trusting the endpoint to default to "all". Nothing in this repo has ever
// called the list without a type, and no spec is vendored here to settle what
// omitting it does — while the failure mode of guessing wrong is a confident,
// complete-looking table missing three kinds. One mocked call per type is that
// contract: a regression to a single unfiltered call fails here.
func expectListOfType(mc *astrov1_mocks.ClientWithResponsesInterface, typ astrov1.ListEnvironmentObjectsParamsObjectType, objs ...astrov1.EnvironmentObject) {
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.ObjectType != nil && *p.ObjectType == typ
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
			EnvironmentObjects: objs,
			TotalCount:         len(objs),
		},
	}, nil).Once()
}

func obj(key string, typ astrov1.EnvironmentObjectObjectType) astrov1.EnvironmentObject {
	return astrov1.EnvironmentObject{ObjectKey: key, ObjectType: typ, Scope: astrov1.EnvironmentObjectScopeWORKSPACE}
}

func TestEnvListAsksForEveryKindExplicitly(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectListOfType(mc, astrov1.ENVIRONMENTVARIABLE, obj("API_TOKEN", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE))
	expectListOfType(mc, astrov1.CONNECTION, obj("db_main", astrov1.EnvironmentObjectObjectTypeCONNECTION))
	expectListOfType(mc, astrov1.AIRFLOWVARIABLE, obj("region", astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE))
	expectListOfType(mc, astrov1.METRICSEXPORT, obj("prom_main", astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT))
	astroV1Client = mc

	out, err := execEnvCmd("list", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	for _, want := range []string{"KIND", "API_TOKEN", "db_main", "region", "prom_main"} {
		assert.Contains(t, out, want)
	}
	mc.AssertExpectations(t)
}

// Rows group by kind in the order the help lists the nouns, then by key.
// Server order is unspecified, so without sorting the kinds interleave and the
// numbered column means something different on every run.
func TestEnvListGroupsByKindThenKey(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectListOfType(mc, astrov1.ENVIRONMENTVARIABLE,
		obj("ZULU", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE),
		obj("ALPHA", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE))
	expectListOfType(mc, astrov1.CONNECTION, obj("db_main", astrov1.EnvironmentObjectObjectTypeCONNECTION))
	expectListOfType(mc, astrov1.AIRFLOWVARIABLE)
	expectListOfType(mc, astrov1.METRICSEXPORT)
	astroV1Client = mc

	out, err := execEnvCmd("list", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	alpha, zulu, conn := strings.Index(out, "ALPHA"), strings.Index(out, "ZULU"), strings.Index(out, "db_main")
	assert.Less(t, alpha, zulu, "keys sort within a kind")
	assert.Less(t, zulu, conn, "variables come before connections, as the help lists them")
	mc.AssertExpectations(t)
}

// --resolve-linked=false is how every other env listing reveals IDs, so the
// cross-kind one carries the column too — otherwise the flag costs a
// non-resolving fetch and returns nothing extra.
func TestEnvListShowsIDsWhenNotResolvingLinks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	withID := obj("API_TOKEN", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE)
	withID.Id = &id

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectListOfType(mc, astrov1.ENVIRONMENTVARIABLE, withID)
	expectListOfType(mc, astrov1.CONNECTION)
	expectListOfType(mc, astrov1.AIRFLOWVARIABLE)
	expectListOfType(mc, astrov1.METRICSEXPORT)
	astroV1Client = mc

	out, err := execEnvCmd("list", "--workspace-id", "ws-test", "--resolve-linked=false")
	assert.NoError(t, err)
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, id)
	mc.AssertExpectations(t)
}

// The KIND column names the subcommand that manages the object, not the API's
// enum — a listing that names the command you would type next is worth more
// than one that names the wire constant. This is the same rule the local tree
// follows for its prose.
func TestEnvListNamesTheNounNotTheWireType(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	expectListOfType(mc, astrov1.ENVIRONMENTVARIABLE, obj("K", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE))
	expectListOfType(mc, astrov1.CONNECTION)
	expectListOfType(mc, astrov1.AIRFLOWVARIABLE, obj("A", astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE))
	expectListOfType(mc, astrov1.METRICSEXPORT)
	astroV1Client = mc

	out, err := execEnvCmd("list", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	assert.Contains(t, out, "variable")
	assert.Contains(t, out, "airflow-variable")
	assert.NotContains(t, out, "ENVIRONMENT_VARIABLE")
	assert.NotContains(t, out, "AIRFLOW_VARIABLE")
	mc.AssertExpectations(t)
}

// No value column survives the intersection of the four per-kind listings, so
// this one is value-free by construction. A secret value must not appear even
// masked — there is nowhere for it to go.
func TestEnvListPrintsNoValues(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			// Never asks the platform to unmask: there is no column for it.
			return p != nil && p.ShowSecrets != nil && !*p.ShowSecrets
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
			EnvironmentObjects: []astrov1.EnvironmentObject{
				{
					ObjectKey: "API_TOKEN", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
					Scope:               astrov1.EnvironmentObjectScopeWORKSPACE,
					EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "s3cr3t"},
				},
			},
			TotalCount: 1,
		},
	}, nil).Times(4)
	astroV1Client = mc

	out, err := execEnvCmd("list", "--workspace-id", "ws-test")
	assert.NoError(t, err)
	assert.Contains(t, out, "API_TOKEN")
	assert.NotContains(t, out, "s3cr3t")
	mc.AssertExpectations(t)
}

// dotenv is KEY=VALUE and this listing has no values, so emitting it would
// produce a file whose re-import is the blank-every-secret shape
// `set --from-file` now refuses.
func TestEnvListRefusesDotenv(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	// No call is mocked: the format is refused before anything is fetched.
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mc

	_, err := execEnvCmd("list", "--workspace-id", "ws-test", "--format", "dotenv")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has none of")
}

// --extra and --value now reach the same parser, so an account id above 2^53
// survives either shape. Before the codec fix they disagreed.
func TestEnvConnExtraAndValueAgreeOnLargeIntegers(t *testing.T) {
	const id = "1234567890123456789"

	capture := func(t *testing.T, args ...string) *astrov1.CreateEnvironmentObjectConnectionRequest {
		t.Helper()
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		defer resetEnvFlags()

		var got *astrov1.CreateEnvironmentObjectConnectionRequest
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		expectAbsent(mc, "sf")
		mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
				got = body.Connection
				return true
			}),
		).Return(&astrov1.CreateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.CreateEnvironmentObject{Id: "cabc12def0123456789012345"},
		}, nil).Once()
		astroV1Client = mc

		full := append([]string{"connection", "set", "sf", "--workspace-id", "ws-test"}, args...)
		_, err := execEnvCmd(full...)
		assert.NoError(t, err)
		mc.AssertExpectations(t)
		return got
	}

	viaExtra := capture(t, "--type", "snowflake", "--extra", `{"account":`+id+`}`)
	viaValue := capture(t, "--value", `{"conn_type":"snowflake","extra":{"account":`+id+`}}`)

	for name, got := range map[string]*astrov1.CreateEnvironmentObjectConnectionRequest{
		"--extra": viaExtra, "--value": viaValue,
	} {
		if assert.NotNil(t, got) && assert.NotNil(t, got.Extra) {
			assert.Equal(t, id, fmt.Sprint((*got.Extra)["account"]), "%s rewrote the account id", name)
		}
	}
}
