package env

import (
	"sort"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// WithSetFields returns o with set_fields filled in from its own values when
// the platform gave none, as for the object a create builds from its inputs:
// the create endpoint answers with an id alone. The names are the platform's
// (its json fields, a map's members as dotted paths such as
// "extra.aws_secret"), and a secret is named although MaskSecrets will take
// its value out, which is what set_fields is for. It is [] when nothing is
// set, never null.
func WithSetFields(o *astrov1.EnvironmentObject) *astrov1.EnvironmentObject {
	if o == nil || o.SetFields != nil {
		return o
	}
	m := *o
	m.SetFields = setFieldsOf(o)
	return &m
}

func setFieldsOf(o *astrov1.EnvironmentObject) []string {
	fields := []string{}
	add := func(name string, set bool) {
		if set {
			fields = append(fields, name)
		}
	}
	addMap := func(prefix string, keys []string) {
		for _, k := range keys {
			fields = append(fields, prefix+"."+k)
		}
	}
	if v := o.EnvironmentVariable; v != nil {
		add("value", v.Value != "")
	}
	if v := o.AirflowVariable; v != nil {
		add("value", v.Value != "")
	}
	if c := o.Connection; c != nil {
		add("type", c.Type != "")
		add("host", nonEmpty(c.Host))
		add("login", nonEmpty(c.Login))
		add("password", nonEmpty(c.Password))
		add("schema", nonEmpty(c.Schema))
		add("port", c.Port != nil)
		if c.Extra != nil {
			addMap("extra", keysOf(*c.Extra))
		}
	}
	if me := o.MetricsExport; me != nil {
		add("endpoint", me.Endpoint != "")
		add("exporterType", me.ExporterType != "")
		add("authType", me.AuthType != nil && *me.AuthType != "")
		add("username", nonEmpty(me.Username))
		add("password", nonEmpty(me.Password))
		add("basicToken", nonEmpty(me.BasicToken))
		add("sigV4AssumeArn", nonEmpty(me.SigV4AssumeArn))
		add("sigV4StsRegion", nonEmpty(me.SigV4StsRegion))
		if me.Headers != nil {
			addMap("headers", keysOf(*me.Headers))
		}
		if me.Labels != nil {
			addMap("labels", keysOf(*me.Labels))
		}
	}
	sort.Strings(fields)
	return fields
}

func nonEmpty(s *string) bool { return s != nil && *s != "" }

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
