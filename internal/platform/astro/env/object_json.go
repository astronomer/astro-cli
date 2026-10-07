package env

import (
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// ObjectInfo is an environment object as `-o json` publishes it: every list
// row and every `get`, whatever the kind. It carries every field of the
// API's EnvironmentObject under snake_case keys, the CLI's convention, where
// the API's own model is camelCase; publishing the generated type as it came
// would have put the API's spelling into the CLI's contract.
//
// It is a copy, not a view: no field is dropped, and pointer for pointer an
// optional field the API left out stays absent. Secret values are whatever
// the API returned, untouched: without --include-secrets the platform blanks
// a secret variable's value and leaves a connection's password and masked
// extra keys out, and with it the platform returns them, so the json says
// exactly what the response said.
type ObjectInfo struct {
	ID                  *string `json:"id,omitempty"`
	ObjectKey           string  `json:"object_key"`
	ObjectType          string  `json:"object_type"`
	Scope               string  `json:"scope"`
	ScopeEntityID       string  `json:"scope_entity_id"`
	SourceScope         *string `json:"source_scope,omitempty"`
	SourceScopeEntityID *string `json:"source_scope_entity_id,omitempty"`
	Description         *string `json:"description,omitempty"`
	AutoLinkDeployments *bool   `json:"auto_link_deployments,omitempty"`
	// SetFields names the fields that have a value, secret ones included, so
	// a reader can tell a masked secret from an unset field. Its entries are
	// the API's field names, data rather than keys of this object.
	SetFields []string `json:"set_fields"`

	// One of these four is set, as object_type says.
	EnvironmentVariable *VariableInfo      `json:"environment_variable,omitempty"`
	AirflowVariable     *VariableInfo      `json:"airflow_variable,omitempty"`
	Connection          *ConnectionInfo    `json:"connection,omitempty"`
	MetricsExport       *MetricsExportInfo `json:"metrics_export,omitempty"`

	Links        *[]DeploymentLinkInfo `json:"links,omitempty"`
	ExcludeLinks *[]ExcludeLinkInfo    `json:"exclude_links,omitempty"`

	CreatedAt *string      `json:"created_at,omitempty"`
	CreatedBy *SubjectInfo `json:"created_by,omitempty"`
	UpdatedAt *string      `json:"updated_at,omitempty"`
	UpdatedBy *SubjectInfo `json:"updated_by,omitempty"`
}

// VariableInfo is the value of an environment variable or an Airflow
// variable. A secret's value is "" unless the platform was asked for it.
type VariableInfo struct {
	IsSecret bool   `json:"is_secret"`
	Value    string `json:"value"`
}

// ConnectionInfo is a connection's fields. Extra's keys are the connection's
// own, published as stored.
type ConnectionInfo struct {
	Type               string                  `json:"type"`
	Host               *string                 `json:"host,omitempty"`
	Port               *int                    `json:"port,omitempty"`
	Login              *string                 `json:"login,omitempty"`
	Password           *string                 `json:"password,omitempty"`
	Schema             *string                 `json:"schema,omitempty"`
	Extra              *map[string]any         `json:"extra,omitempty"`
	ConnectionAuthType *ConnectionAuthTypeInfo `json:"connection_auth_type,omitempty"`
}

// ConnectionAuthTypeInfo is the auth method a connection was made with.
type ConnectionAuthTypeInfo struct {
	ID                  string                    `json:"id"`
	Name                string                    `json:"name"`
	Description         string                    `json:"description"`
	AirflowType         string                    `json:"airflow_type"`
	AuthMethodName      string                    `json:"auth_method_name"`
	ProviderPackageName string                    `json:"provider_package_name"`
	ProviderLogo        *string                   `json:"provider_logo,omitempty"`
	GuidePath           *string                   `json:"guide_path,omitempty"`
	Parameters          []ConnectionAuthParamInfo `json:"parameters"`
}

// ConnectionAuthParamInfo is one parameter of a connection auth type.
type ConnectionAuthParamInfo struct {
	AirflowParamName string  `json:"airflow_param_name"`
	FriendlyName     string  `json:"friendly_name"`
	Description      string  `json:"description"`
	DataType         string  `json:"data_type"`
	Example          *string `json:"example,omitempty"`
	Pattern          *string `json:"pattern,omitempty"`
	IsInExtra        bool    `json:"is_in_extra"`
	IsRequired       bool    `json:"is_required"`
	IsSecret         bool    `json:"is_secret"`
}

// MetricsExportInfo is a metrics export's fields. Headers and labels are the
// export's own keys, published as stored.
type MetricsExportInfo struct {
	ExporterType   string             `json:"exporter_type"`
	Endpoint       string             `json:"endpoint"`
	AuthType       *string            `json:"auth_type,omitempty"`
	Username       *string            `json:"username,omitempty"`
	Password       *string            `json:"password,omitempty"`
	BasicToken     *string            `json:"basic_token,omitempty"`
	SigV4AssumeArn *string            `json:"sig_v4_assume_arn,omitempty"`
	SigV4StsRegion *string            `json:"sig_v4_sts_region,omitempty"`
	Headers        *map[string]string `json:"headers,omitempty"`
	Labels         *map[string]string `json:"labels,omitempty"`
}

// DeploymentLinkInfo is one link of a workspace object to a deployment, with
// the override it gives that deployment, if any.
type DeploymentLinkInfo struct {
	Scope         string   `json:"scope"`
	ScopeEntityID string   `json:"scope_entity_id"`
	SetFields     []string `json:"set_fields"`

	EnvironmentVariableOverrides *ValueOverrideInfo          `json:"environment_variable_overrides,omitempty"`
	AirflowVariableOverrides     *ValueOverrideInfo          `json:"airflow_variable_overrides,omitempty"`
	ConnectionOverrides          *ConnectionOverridesInfo    `json:"connection_overrides,omitempty"`
	MetricsExportOverrides       *MetricsExportOverridesInfo `json:"metrics_export_overrides,omitempty"`
}

// ValueOverrideInfo is a link's override of a variable's value.
type ValueOverrideInfo struct {
	Value string `json:"value"`
}

// ConnectionOverridesInfo is a link's override of a connection: only the
// fields it sets.
type ConnectionOverridesInfo struct {
	Type     *string         `json:"type,omitempty"`
	Host     *string         `json:"host,omitempty"`
	Port     *int            `json:"port,omitempty"`
	Login    *string         `json:"login,omitempty"`
	Password *string         `json:"password,omitempty"`
	Schema   *string         `json:"schema,omitempty"`
	Extra    *map[string]any `json:"extra,omitempty"`
}

// MetricsExportOverridesInfo is a link's override of a metrics export: only
// the fields it sets.
type MetricsExportOverridesInfo struct {
	ExporterType   *string            `json:"exporter_type,omitempty"`
	Endpoint       *string            `json:"endpoint,omitempty"`
	AuthType       *string            `json:"auth_type,omitempty"`
	Username       *string            `json:"username,omitempty"`
	Password       *string            `json:"password,omitempty"`
	BasicToken     *string            `json:"basic_token,omitempty"`
	SigV4AssumeArn *string            `json:"sig_v4_assume_arn,omitempty"`
	SigV4StsRegion *string            `json:"sig_v4_sts_region,omitempty"`
	Headers        *map[string]string `json:"headers,omitempty"`
	Labels         *map[string]string `json:"labels,omitempty"`
}

// ExcludeLinkInfo is one deployment a workspace object is kept away from.
type ExcludeLinkInfo struct {
	Scope         string `json:"scope"`
	ScopeEntityID string `json:"scope_entity_id"`
}

// SubjectInfo is who created or last updated an object: a user or an API
// token, as subject_type says.
type SubjectInfo struct {
	ID           string  `json:"id"`
	SubjectType  *string `json:"subject_type,omitempty"`
	Username     *string `json:"username,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	APITokenName *string `json:"api_token_name,omitempty"`
}

// newObjectInfos converts a list for -o json, [] when it is empty.
func newObjectInfos(objs []astrov1.EnvironmentObject) []ObjectInfo {
	out := make([]ObjectInfo, len(objs))
	for i := range objs {
		out[i] = NewObjectInfo(&objs[i])
	}
	return out
}

// NewObjectInfo converts one object for -o json: a `get`, and what a `set`
// left or a `delete` removed.
func NewObjectInfo(o *astrov1.EnvironmentObject) ObjectInfo {
	info := ObjectInfo{
		ID:                  o.Id,
		ObjectKey:           o.ObjectKey,
		ObjectType:          string(o.ObjectType),
		Scope:               string(o.Scope),
		ScopeEntityID:       o.ScopeEntityId,
		SourceScope:         enumPtr(o.SourceScope),
		SourceScopeEntityID: o.SourceScopeEntityId,
		Description:         o.Description,
		AutoLinkDeployments: o.AutoLinkDeployments,
		SetFields:           o.SetFields,
		CreatedAt:           o.CreatedAt,
		CreatedBy:           newSubjectInfo(o.CreatedBy),
		UpdatedAt:           o.UpdatedAt,
		UpdatedBy:           newSubjectInfo(o.UpdatedBy),
	}
	if v := o.EnvironmentVariable; v != nil {
		info.EnvironmentVariable = &VariableInfo{IsSecret: v.IsSecret, Value: v.Value}
	}
	if v := o.AirflowVariable; v != nil {
		info.AirflowVariable = &VariableInfo{IsSecret: v.IsSecret, Value: v.Value}
	}
	if c := o.Connection; c != nil {
		info.Connection = &ConnectionInfo{
			Type: c.Type, Host: c.Host, Port: c.Port, Login: c.Login,
			Password: c.Password, Schema: c.Schema, Extra: c.Extra,
			ConnectionAuthType: newConnectionAuthTypeInfo(c.ConnectionAuthType),
		}
	}
	if m := o.MetricsExport; m != nil {
		info.MetricsExport = &MetricsExportInfo{
			ExporterType: string(m.ExporterType), Endpoint: m.Endpoint, AuthType: enumPtr(m.AuthType),
			Username: m.Username, Password: m.Password, BasicToken: m.BasicToken,
			SigV4AssumeArn: m.SigV4AssumeArn, SigV4StsRegion: m.SigV4StsRegion,
			Headers: m.Headers, Labels: m.Labels,
		}
	}
	if o.Links != nil {
		links := make([]DeploymentLinkInfo, len(*o.Links))
		for i := range *o.Links {
			links[i] = newDeploymentLinkInfo(&(*o.Links)[i])
		}
		info.Links = &links
	}
	if o.ExcludeLinks != nil {
		excludes := make([]ExcludeLinkInfo, len(*o.ExcludeLinks))
		for i, e := range *o.ExcludeLinks {
			excludes[i] = ExcludeLinkInfo{Scope: string(e.Scope), ScopeEntityID: e.ScopeEntityId}
		}
		info.ExcludeLinks = &excludes
	}
	return info
}

func newDeploymentLinkInfo(l *astrov1.EnvironmentObjectLink) DeploymentLinkInfo {
	info := DeploymentLinkInfo{Scope: string(l.Scope), ScopeEntityID: l.ScopeEntityId, SetFields: l.SetFields}
	if v := l.EnvironmentVariableOverrides; v != nil {
		info.EnvironmentVariableOverrides = &ValueOverrideInfo{Value: v.Value}
	}
	if v := l.AirflowVariableOverrides; v != nil {
		info.AirflowVariableOverrides = &ValueOverrideInfo{Value: v.Value}
	}
	if c := l.ConnectionOverrides; c != nil {
		info.ConnectionOverrides = &ConnectionOverridesInfo{
			Type: c.Type, Host: c.Host, Port: c.Port, Login: c.Login,
			Password: c.Password, Schema: c.Schema, Extra: c.Extra,
		}
	}
	if m := l.MetricsExportOverrides; m != nil {
		info.MetricsExportOverrides = &MetricsExportOverridesInfo{
			ExporterType: enumPtr(m.ExporterType), Endpoint: m.Endpoint, AuthType: enumPtr(m.AuthType),
			Username: m.Username, Password: m.Password, BasicToken: m.BasicToken,
			SigV4AssumeArn: m.SigV4AssumeArn, SigV4StsRegion: m.SigV4StsRegion,
			Headers: m.Headers, Labels: m.Labels,
		}
	}
	return info
}

func newConnectionAuthTypeInfo(a *astrov1.ConnectionAuthType) *ConnectionAuthTypeInfo {
	if a == nil {
		return nil
	}
	info := &ConnectionAuthTypeInfo{
		ID: a.Id, Name: a.Name, Description: a.Description, AirflowType: a.AirflowType,
		AuthMethodName: a.AuthMethodName, ProviderPackageName: a.ProviderPackageName,
		ProviderLogo: a.ProviderLogo, GuidePath: a.GuidePath,
	}
	if a.Parameters != nil {
		info.Parameters = make([]ConnectionAuthParamInfo, len(a.Parameters))
		for i, p := range a.Parameters {
			info.Parameters[i] = ConnectionAuthParamInfo{
				AirflowParamName: p.AirflowParamName, FriendlyName: p.FriendlyName,
				Description: p.Description, DataType: p.DataType, Example: p.Example,
				Pattern: p.Pattern, IsInExtra: p.IsInExtra, IsRequired: p.IsRequired, IsSecret: p.IsSecret,
			}
		}
	}
	return info
}

func newSubjectInfo(s *astrov1.BasicSubjectProfile) *SubjectInfo {
	if s == nil {
		return nil
	}
	return &SubjectInfo{
		ID: s.Id, SubjectType: enumPtr(s.SubjectType), Username: s.Username,
		FullName: s.FullName, AvatarURL: s.AvatarUrl, APITokenName: s.ApiTokenName,
	}
}

// enumPtr turns an optional API enum into an optional string.
func enumPtr[T ~string](p *T) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}
