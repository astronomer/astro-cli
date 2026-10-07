package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func ptr[T any](v T) *T { return &v }

// connObj is a workspace-scoped connection as GET returns it without secrets:
// no password, and only the extras its auth type marks public.
func connObj(id string, links []astrov1.EnvironmentObjectLink) *astrov1.EnvironmentObject {
	o := astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     "db",
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.CONNECTION),
		Scope:         astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId: cuid.New(),
		Connection: &astrov1.EnvironmentObjectConnection{
			Type:               "postgres",
			Host:               ptr("db.internal"),
			Login:              ptr("app"),
			Schema:             ptr("public"),
			Port:               ptr(5432),
			Extra:              &map[string]any{"sslmode": "require"},
			ConnectionAuthType: &astrov1.ConnectionAuthType{Id: "auth-1", AirflowType: "postgres"},
		},
	}
	if links != nil {
		o.Links = &links
	}
	return &o
}

func airflowVarObj(id, value string, links []astrov1.EnvironmentObjectLink) *astrov1.EnvironmentObject {
	o := astrov1.EnvironmentObject{
		Id:              &id,
		ObjectKey:       "region",
		ObjectType:      astrov1.EnvironmentObjectObjectType(astrov1.AIRFLOWVARIABLE),
		Scope:           astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:   cuid.New(),
		AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: value},
	}
	if links != nil {
		o.Links = &links
	}
	return &o
}

func (s *Suite) mockGet(mc *astrov1_mocks.ClientWithResponsesInterface, obj *astrov1.EnvironmentObject) {
	ctx, _ := config.GetCurrentContext()
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{*obj}},
	}, nil).Once()
}

// mockUpdate captures the update body for assertions after the call.
func (s *Suite) mockUpdate(mc *astrov1_mocks.ClientWithResponsesInterface, id string) *astrov1.UpdateEnvironmentObjectJSONRequestBody {
	ctx, _ := config.GetCurrentContext()
	var got astrov1.UpdateEnvironmentObjectJSONRequestBody
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id, mock.Anything).
		Run(func(args mock.Arguments) { got = args.Get(3).(astrov1.UpdateEnvironmentObjectJSONRequestBody) }).
		Return(&astrov1.UpdateEnvironmentObjectResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "x"},
		}, nil).Once()
	return &got
}

// A connection update is not a patch: the platform stores host, login,
// schema, port and auth type as sent. Linking must send them all back, or a
// link change would blank the connection. The password is left out — GET
// masks it, and the platform keeps a password an update omits.
func (s *Suite) TestLinkConnEchoesTheWholeConnection() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(id, nil))
	got := s.mockUpdate(mc, id)

	s.NoError(errOf(Link(LinkConnection, "db", Scope{WorkspaceID: cuid.New()}, depID,
		&LinkOverride{Connection: &ConnOverride{Host: ptr("db.prod"), Port: ptr(6432)}}, false, mc)))
	mc.AssertExpectations(s.T())

	c := got.Connection
	s.Require().NotNil(c, "the typed connection field is required even for a link-only update")
	s.Equal("postgres", c.Type)
	s.Equal("db.internal", *c.Host)
	s.Equal("app", *c.Login)
	s.Equal("public", *c.Schema)
	s.Equal(5432, *c.Port)
	s.Equal(map[string]any{"sslmode": "require"}, *c.Extra)
	s.Equal("auth-1", *c.AuthTypeId)
	s.Nil(c.Password)

	s.Require().Len(*got.Links, 1)
	l := (*got.Links)[0]
	s.Equal(depID, l.ScopeEntityId)
	s.Equal("db.prod", *l.Overrides.Connection.Host)
	s.Equal(6432, *l.Overrides.Connection.Port)
	s.Nil(l.Overrides.UnsetFields, "a new link has nothing to unset")
}

// The override describes the whole override: re-linking clears every field
// the link had that the new override leaves out, including a secret one GET
// masks, which only setFields reveals.
func (s *Suite) TestLinkConnUnsetsWhatTheNewOverrideOmits() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	existing := astrov1.EnvironmentObjectLink{
		Scope:               astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:       depID,
		ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: ptr("db.old"), Extra: &map[string]any{"sslmode": "disable"}},
		SetFields:           []string{"extra.sslmode", "extra.token", "host", "password"},
	}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(id, []astrov1.EnvironmentObjectLink{existing}))
	got := s.mockUpdate(mc, id)

	s.NoError(errOf(Link(LinkConnection, "db", Scope{WorkspaceID: cuid.New()}, depID,
		&LinkOverride{Connection: &ConnOverride{Host: ptr("db.new")}}, true, mc)))
	mc.AssertExpectations(s.T())

	o := (*got.Links)[0].Overrides
	s.Equal("db.new", *o.Connection.Host)
	s.Equal([]string{"extra.sslmode", "extra.token", "password"}, *o.UnsetFields)
}

// Linking one deployment must leave the others as they are. An entry sent
// without overrides keeps only its secret fields, so each carries back the
// override GET showed.
func (s *Suite) TestLinkConnRoundTripsOtherLinksOverrides() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, other, depID := cuid.New(), cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(id, []astrov1.EnvironmentObjectLink{{
		Scope:               astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:       other,
		ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: ptr("db.staging"), Port: ptr(15432)},
	}}))
	got := s.mockUpdate(mc, id)

	s.NoError(errOf(Link(LinkConnection, "db", Scope{WorkspaceID: cuid.New()}, depID, nil, false, mc)))
	mc.AssertExpectations(s.T())

	s.Require().Len(*got.Links, 2)
	kept := (*got.Links)[0]
	s.Equal(other, kept.ScopeEntityId)
	s.Equal("db.staging", *kept.Overrides.Connection.Host)
	s.Equal(15432, *kept.Overrides.Connection.Port)
	s.Nil((*got.Links)[1].Overrides, "a link given no override sends none")
}

// `set` on a linked connection used to echo its links with only env-var
// overrides copied, so every update cleared the host, port and other
// non-secret overrides each deployment had.
func (s *Suite) TestUpdateConnKeepsLinkOverrides() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(id, []astrov1.EnvironmentObjectLink{{
		Scope:               astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:       depID,
		ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: ptr("db.prod")},
	}}))
	got := s.mockUpdate(mc, id)

	_, err := UpdateConn("db", Scope{WorkspaceID: cuid.New()}, ConnInput{Type: "postgres", Login: ptr("svc")}, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())
	s.Equal("db.prod", *(*got.Links)[0].Overrides.Connection.Host)
}

// The same for an Airflow variable's value override.
func (s *Suite) TestUpdateAirflowVarKeepsLinkOverrides() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, airflowVarObj(id, "us-east-1", []astrov1.EnvironmentObjectLink{{
		Scope:                    astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:            depID,
		AirflowVariableOverrides: &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: "eu-west-1"},
	}}))
	got := s.mockUpdate(mc, id)

	_, err := UpdateAirflowVar("region", Scope{WorkspaceID: cuid.New()}, "us-west-2", nil, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())
	s.Equal("eu-west-1", *(*got.Links)[0].Overrides.AirflowVariable.Value)
}

func (s *Suite) TestLinkAirflowVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()

	s.Run("echoes the value and sets the override", func() {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		s.mockGet(mc, airflowVarObj(id, "us-east-1", nil))
		got := s.mockUpdate(mc, id)

		s.NoError(errOf(Link(LinkAirflowVariable, "region", Scope{WorkspaceID: cuid.New()}, depID, &LinkOverride{Value: ptr("eu-west-1")}, false, mc)))
		mc.AssertExpectations(s.T())
		s.Equal("us-east-1", *got.AirflowVariable.Value)
		s.Equal("eu-west-1", *(*got.Links)[0].Overrides.AirflowVariable.Value)
	})

	s.Run("re-linking without a value clears the override", func() {
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		s.mockGet(mc, airflowVarObj(id, "us-east-1", []astrov1.EnvironmentObjectLink{{
			Scope:                    astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
			ScopeEntityId:            depID,
			AirflowVariableOverrides: &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: "eu-west-1"},
		}}))
		got := s.mockUpdate(mc, id)

		s.NoError(errOf(Link(LinkAirflowVariable, "region", Scope{WorkspaceID: cuid.New()}, depID, nil, false, mc)))
		mc.AssertExpectations(s.T())
		o := (*got.Links)[0].Overrides
		s.Nil(o.AirflowVariable)
		s.Equal([]string{"value"}, *o.UnsetFields)
	})
}

// Errors name the object by its kind, not as an environment variable.
func (s *Suite) TestUnlinkNamesTheKind() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(cuid.New(), nil))

	_, err := Unlink(LinkConnection, "db", Scope{WorkspaceID: cuid.New()}, cuid.New(), mc)
	s.ErrorContains(err, `connection "db" is not linked`)
	mc.AssertExpectations(s.T())
}

// The report lists visible overrides by field and names the set ones, so a
// masked password still shows as set.
func (s *Suite) TestListConnLinks() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, connObj(id, []astrov1.EnvironmentObjectLink{{
		Scope:               astrov1.EnvironmentObjectLinkScope(astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT),
		ScopeEntityId:       depID,
		ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Host: ptr("db.prod"), Port: ptr(6432)},
		SetFields:           []string{"host", "password", "port"},
	}}))

	report, err := ListLinks(LinkConnection, "db", Scope{WorkspaceID: cuid.New()}, false, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())
	s.Equal(id, report.ObjectID)
	s.Equal([]string{}, report.ExcludeLinks)
	s.Require().Len(report.Links, 1)
	s.Equal(map[string]any{"host": "db.prod", "port": float64(6432)}, report.Links[0].Overrides)
	s.Equal([]string{"host", "password", "port"}, report.Links[0].SetFields)
}

// The platform stores a connection's host, login, schema, port and auth type
// as sent, so `set` fills each one it was not given from the current
// connection. Before, `set db --type postgres --login svc` blanked the host,
// schema and port.
func (s *Suite) TestUpdateConnPatches() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	for _, tc := range []struct {
		name  string
		in    ConnInput
		check func(c *astrov1.UpdateEnvironmentObjectConnectionRequest)
	}{
		{"keeps what it was not given", ConnInput{Type: "postgres", Login: ptr("svc")}, func(c *astrov1.UpdateEnvironmentObjectConnectionRequest) {
			s.Equal("svc", *c.Login)
			s.Equal("db.internal", *c.Host)
			s.Equal("public", *c.Schema)
			s.Equal(5432, *c.Port)
			s.Equal("auth-1", *c.AuthTypeId)
			s.Nil(c.Password, "GET masks it; the platform keeps a password an update omits")
			s.Nil(c.Extra, "the platform keeps extra keys an update omits")
		}},
		{"an empty value still clears", ConnInput{Type: "postgres", Host: ptr("")}, func(c *astrov1.UpdateEnvironmentObjectConnectionRequest) {
			s.Empty(*c.Host)
			s.Equal("app", *c.Login)
		}},
		{"a new type drops the auth type", ConnInput{Type: "mysql"}, func(c *astrov1.UpdateEnvironmentObjectConnectionRequest) {
			s.Equal("mysql", c.Type)
			s.Nil(c.AuthTypeId)
			s.Equal("db.internal", *c.Host)
		}},
		{"a URI without a port keeps the stored one", ConnInput{Type: "postgres", Host: ptr("db.new"), Login: ptr(""), Schema: ptr("")}, func(c *astrov1.UpdateEnvironmentObjectConnectionRequest) {
			s.Equal("db.new", *c.Host)
			s.Empty(*c.Login)
			s.Equal(5432, *c.Port)
		}},
	} {
		s.Run(tc.name, func() {
			id := cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			s.mockGet(mc, connObj(id, nil))
			got := s.mockUpdate(mc, id)

			_, err := UpdateConn("db", Scope{WorkspaceID: cuid.New()}, tc.in, mc)
			s.Require().NoError(err)
			mc.AssertExpectations(s.T())
			s.Require().NotNil(got.Connection)
			tc.check(got.Connection)
		})
	}
}

// A metrics export's update used to echo its links without overrides, which
// the platform takes as "no override" for everything but its secrets: each
// deployment's endpoint, exporter type, headers and the rest were cleared.
func (s *Suite) TestUpdateMetricsExportKeepsLinkOverrides() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	id, depID := cuid.New(), cuid.New()
	basic := astrov1.EnvironmentObjectMetricsExportOverridesAuthTypeBASIC
	prom := astrov1.EnvironmentObjectMetricsExportOverridesExporterTypePROMETHEUS
	links := []astrov1.EnvironmentObjectLink{{
		Scope:         astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
		ScopeEntityId: depID,
		MetricsExportOverrides: &astrov1.EnvironmentObjectMetricsExportOverrides{
			Endpoint:     ptr("https://prom.prod"),
			ExporterType: &prom,
			AuthType:     &basic,
			Username:     ptr("scraper"),
			Headers:      &map[string]string{"X-Team": "data"},
		},
	}}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.mockGet(mc, &astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     "prom_main",
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.METRICSEXPORT),
		Scope:         astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId: cuid.New(),
		Links:         &links,
	})
	got := s.mockUpdate(mc, id)

	_, err := UpdateMetricsExport("prom_main", Scope{WorkspaceID: cuid.New()}, &MetricsInput{Endpoint: "https://prom2"}, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())

	s.Require().Len(*got.Links, 1)
	o := (*got.Links)[0].Overrides
	s.Require().NotNil(o)
	s.Require().NotNil(o.MetricsExport)
	m := o.MetricsExport
	s.Equal("https://prom.prod", *m.Endpoint)
	s.Equal("PROMETHEUS", string(*m.ExporterType))
	s.Equal("BASIC", string(*m.AuthType))
	s.Equal("scraper", *m.Username)
	s.Equal(map[string]string{"X-Team": "data"}, *m.Headers)
	s.Nil(m.Password, "GET masks it")
}
