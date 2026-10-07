package env

import (
	"maps"
	"slices"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// MaskSecrets returns a copy of o with every secret value taken out, the way
// the platform leaves them out of a read without --include-secrets: a secret
// variable's or Airflow variable's value is "", and so is a link's override
// of it; a connection's and a metrics export's password and basic token are
// absent, on the object and on each link's override; and so are the keys of
// a connection's extra that its auth type marks secret. A connection with no
// auth type to say which extra keys are secret (the object a create builds
// from its inputs, or one made field by field) has every extra value blanked
// to "", its keys kept, on the object and on its links: an extra holds keys
// such as aws_secret_access_key as readily as a region, and nothing else
// tells them apart. set_fields still names what is set.
//
// What a write publishes goes through it, whatever the API answered with.
// The platform masks its answers to a write today, but a write's output is
// the one a script is most likely to log, and that is no place to depend on
// another service for it.
func MaskSecrets(o *astrov1.EnvironmentObject) *astrov1.EnvironmentObject {
	if o == nil {
		return nil
	}
	m := *o
	varSecret := o.EnvironmentVariable != nil && o.EnvironmentVariable.IsSecret
	if varSecret {
		v := *o.EnvironmentVariable
		v.Value = ""
		m.EnvironmentVariable = &v
	}
	afSecret := o.AirflowVariable != nil && o.AirflowVariable.IsSecret
	if afSecret {
		v := *o.AirflowVariable
		v.Value = ""
		m.AirflowVariable = &v
	}
	var extra extraMask
	if c := o.Connection; c != nil {
		extra = newExtraMask(c.ConnectionAuthType)
		cc := *c
		cc.Password = nil
		cc.Extra = extra.apply(c.Extra)
		m.Connection = &cc
	}
	if me := o.MetricsExport; me != nil {
		mc := *me
		mc.Password, mc.BasicToken = nil, nil
		m.MetricsExport = &mc
	}
	if o.Links != nil {
		links := slices.Clone(*o.Links)
		for i := range links {
			maskLink(&links[i], varSecret, afSecret, extra)
		}
		m.Links = &links
	}
	return &m
}

// maskLink takes the secrets out of one link's overrides, replacing each
// override it changes rather than writing through it.
func maskLink(l *astrov1.EnvironmentObjectLink, varSecret, afSecret bool, extra extraMask) {
	if l.EnvironmentVariableOverrides != nil && varSecret {
		l.EnvironmentVariableOverrides = &astrov1.EnvironmentObjectEnvironmentVariableOverrides{}
	}
	if l.AirflowVariableOverrides != nil && afSecret {
		l.AirflowVariableOverrides = &astrov1.EnvironmentObjectAirflowVariableOverrides{}
	}
	if c := l.ConnectionOverrides; c != nil {
		cc := *c
		cc.Password = nil
		cc.Extra = extra.apply(c.Extra)
		l.ConnectionOverrides = &cc
	}
	if me := l.MetricsExportOverrides; me != nil {
		mc := *me
		mc.Password, mc.BasicToken = nil, nil
		l.MetricsExportOverrides = &mc
	}
}

// extraMask is how a connection's extra is masked: by the keys its auth type
// marks secret, or, when there is no auth type to say, every value.
type extraMask struct {
	known  bool
	secret []string
}

func newExtraMask(a *astrov1.ConnectionAuthType) extraMask {
	if a == nil || len(a.Parameters) == 0 {
		return extraMask{}
	}
	m := extraMask{known: true}
	for _, p := range a.Parameters {
		if p.IsSecret && p.IsInExtra {
			m.secret = append(m.secret, p.AirflowParamName)
		}
	}
	return m
}

// apply returns a masked copy of an optional extra map, or the map itself
// when nothing in it is secret.
func (e extraMask) apply(m *map[string]any) *map[string]any { //nolint:gocritic // the API's optional map is a pointer
	if m == nil {
		return nil
	}
	if !e.known {
		out := make(map[string]any, len(*m))
		for k := range *m {
			out[k] = ""
		}
		return &out
	}
	if len(e.secret) == 0 {
		return m
	}
	out := maps.Clone(*m)
	for _, k := range e.secret {
		delete(out, k)
	}
	return &out
}
