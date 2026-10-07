package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// MaskSecrets takes out every secret value an object can carry, on the object
// and on its links' overrides, and leaves what is not secret alone. It works
// on a copy: the object it was given, which the text path may still print
// from, keeps its values.
func TestMaskSecrets(t *testing.T) {
	t.Run("variables", func(t *testing.T) {
		for _, secret := range []bool{true, false} {
			o := &astrov1.EnvironmentObject{
				EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "v", IsSecret: secret},
				Links: &[]astrov1.EnvironmentObjectLink{{
					ScopeEntityId:                "dep",
					EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: "o"},
				}},
			}
			m := MaskSecrets(o)
			want, wantOverride := "v", "o"
			if secret {
				want, wantOverride = "", ""
			}
			assert.Equal(t, want, m.EnvironmentVariable.Value)
			require.NotNil(t, (*m.Links)[0].EnvironmentVariableOverrides, "a masked override is still there")
			assert.Equal(t, wantOverride, (*m.Links)[0].EnvironmentVariableOverrides.Value)
			assert.Equal(t, "v", o.EnvironmentVariable.Value, "the original keeps its value")
			assert.Equal(t, "o", (*o.Links)[0].EnvironmentVariableOverrides.Value)

			a := MaskSecrets(&astrov1.EnvironmentObject{
				AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: "v", IsSecret: secret},
				Links: &[]astrov1.EnvironmentObjectLink{{
					AirflowVariableOverrides: &astrov1.EnvironmentObjectAirflowVariableOverrides{Value: "o"},
				}},
			})
			assert.Equal(t, want, a.AirflowVariable.Value)
			assert.Equal(t, wantOverride, (*a.Links)[0].AirflowVariableOverrides.Value)
		}
	})

	t.Run("a connection", func(t *testing.T) {
		pw, host := "pw", "h"
		extra := map[string]any{"token": "t", "region": "eu"}
		o := &astrov1.EnvironmentObject{
			Connection: &astrov1.EnvironmentObjectConnection{
				Type: "aws", Host: &host, Password: &pw, Extra: &extra,
				ConnectionAuthType: &astrov1.ConnectionAuthType{Parameters: []astrov1.ConnectionAuthTypeParameter{
					{AirflowParamName: "token", IsSecret: true, IsInExtra: true},
					{AirflowParamName: "region", IsInExtra: true},
				}},
			},
			Links: &[]astrov1.EnvironmentObjectLink{{
				ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Password: &pw, Host: &host, Extra: &extra},
			}},
		}
		m := MaskSecrets(o)
		assert.Nil(t, m.Connection.Password)
		assert.Equal(t, &host, m.Connection.Host)
		assert.Equal(t, map[string]any{"region": "eu"}, *m.Connection.Extra)
		ov := (*m.Links)[0].ConnectionOverrides
		assert.Nil(t, ov.Password)
		assert.Equal(t, map[string]any{"region": "eu"}, *ov.Extra)
		assert.Equal(t, &pw, o.Connection.Password, "the original keeps its password")
		assert.Len(t, extra, 2, "and its extra")
	})

	// With no auth type, nothing says which extra keys are secret, so every
	// value is hidden (nil, which json publishes as null) and the keys kept,
	// on the object and its links.
	t.Run("a connection with no auth type", func(t *testing.T) {
		extra := map[string]any{"aws_secret_access_key": "AKIAsecret", "region": "eu"}
		for _, auth := range []*astrov1.ConnectionAuthType{nil, {}} {
			o := &astrov1.EnvironmentObject{
				Connection: &astrov1.EnvironmentObjectConnection{Type: "aws", Extra: &extra, ConnectionAuthType: auth},
				Links: &[]astrov1.EnvironmentObjectLink{{
					ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Extra: &extra},
				}},
			}
			m := MaskSecrets(o)
			want := map[string]any{"aws_secret_access_key": nil, "region": nil}
			assert.Equal(t, want, *m.Connection.Extra)
			assert.Equal(t, want, *(*m.Links)[0].ConnectionOverrides.Extra)
			assert.Equal(t, "AKIAsecret", extra["aws_secret_access_key"], "the original keeps its extra")
		}
	})

	t.Run("a metrics export", func(t *testing.T) {
		pw, tok, user := "pw", "tok", "u"
		m := MaskSecrets(&astrov1.EnvironmentObject{
			MetricsExport: &astrov1.EnvironmentObjectMetricsExport{Password: &pw, BasicToken: &tok, Username: &user},
			Links: &[]astrov1.EnvironmentObjectLink{{
				MetricsExportOverrides: &astrov1.EnvironmentObjectMetricsExportOverrides{Password: &pw, BasicToken: &tok},
			}},
		})
		assert.Nil(t, m.MetricsExport.Password)
		assert.Nil(t, m.MetricsExport.BasicToken)
		assert.Equal(t, &user, m.MetricsExport.Username)
		assert.Nil(t, (*m.Links)[0].MetricsExportOverrides.Password)
		assert.Nil(t, (*m.Links)[0].MetricsExportOverrides.BasicToken)
	})
}

// WithSetFields names what an object built from a create's inputs has set,
// secrets included, as the platform's set_fields would; [] when nothing is,
// and the platform's own list left alone.
func TestWithSetFields(t *testing.T) {
	pw, host, empty := "pw", "h", ""
	extra := map[string]any{"aws_secret": "s"}
	got := WithSetFields(&astrov1.EnvironmentObject{
		Connection: &astrov1.EnvironmentObjectConnection{Type: "aws", Host: &host, Password: &pw, Login: &empty, Extra: &extra},
	})
	assert.Equal(t, []string{"extra.aws_secret", "host", "password", "type"}, got.SetFields)

	tok, user := "t", "u"
	headers := map[string]string{"X-Key": "k"}
	got = WithSetFields(&astrov1.EnvironmentObject{
		MetricsExport: &astrov1.EnvironmentObjectMetricsExport{Endpoint: "e", ExporterType: "PROMETHEUS", BasicToken: &tok, Username: &user, Headers: &headers},
	})
	assert.Equal(t, []string{"basicToken", "endpoint", "exporterType", "headers.X-Key", "username"}, got.SetFields)

	got = WithSetFields(&astrov1.EnvironmentObject{EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{IsSecret: true}})
	assert.NotNil(t, got.SetFields)
	assert.Empty(t, got.SetFields)

	got = WithSetFields(&astrov1.EnvironmentObject{SetFields: []string{"value"}, EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{}})
	assert.Equal(t, []string{"value"}, got.SetFields)
}

// A read's mask takes out what MaskSecrets does, except a connection with no
// auth type keeps its extra: on a read the platform has already taken out
// what it holds secret. Its password still goes.
func TestMaskReadKeepsAnExtraWithNoAuthType(t *testing.T) {
	pw := "pw"
	extra := map[string]any{"region": "eu"}
	o := &astrov1.EnvironmentObject{
		Connection: &astrov1.EnvironmentObjectConnection{Type: "aws", Password: &pw, Extra: &extra},
		Links: &[]astrov1.EnvironmentObjectLink{{
			ConnectionOverrides: &astrov1.EnvironmentObjectConnectionOverrides{Password: &pw, Extra: &extra},
		}},
	}
	m := maskRead(o)
	assert.Nil(t, m.Connection.Password)
	assert.Equal(t, extra, *m.Connection.Extra)
	assert.Nil(t, (*m.Links)[0].ConnectionOverrides.Password)
	assert.Equal(t, extra, *(*m.Links)[0].ConnectionOverrides.Extra)
	assert.Equal(t, map[string]any{"region": nil}, *MaskSecrets(o).Connection.Extra, "a write hides it")
}
