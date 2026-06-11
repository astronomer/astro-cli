package env

import (
	httpcontext "context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeMetrics = astrov1.METRICSEXPORT

// MetricsInput is the user-supplied data needed to create or update a metrics export.
// Endpoint and ExporterType are required on create. AutoLinkDeployments,
// when non-nil, sets the workspace object's "auto-link to all deployments" flag.
type MetricsInput struct {
	Endpoint            string
	ExporterType        string
	AuthType            string
	BasicToken          *string
	Username            *string
	Password            *string
	SigV4AssumeArn      *string
	SigV4StsRegion      *string
	Headers             *map[string]string
	Labels              *map[string]string
	AutoLinkDeployments *bool
}

// ListMetricsExports returns METRICS_EXPORT objects for the given scope.
func ListMetricsExports(scope Scope, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) ([]astrov1.EnvironmentObject, error) {
	return listObjects(scope, objectTypeMetrics, resolveLinked, includeSecrets, astroV1Client)
}

// GetMetricsExport fetches a single metrics export by ID or key.
func GetMetricsExport(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeMetrics, includeSecrets, astroV1Client)
}

// CreateMetricsExport creates a new METRICS_EXPORT object.
func CreateMetricsExport(scope Scope, key string, in *MetricsInput, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := validateAutoLink(scope, in.AutoLinkDeployments); err != nil {
		return nil, err
	}
	if in.Endpoint == "" {
		return nil, errors.New("metrics export endpoint is required (--endpoint)")
	}
	if in.ExporterType == "" {
		return nil, errors.New("metrics export exporter type is required (e.g. --exporter-type PROMETHEUS)")
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	scopeType, scopeEntityID := scopeRequest(scope)
	body := astrov1.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrov1.CreateEnvironmentObjectRequestObjectTypeMETRICSEXPORT,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		MetricsExport: &astrov1.CreateEnvironmentObjectMetricsExportRequest{
			Endpoint:       in.Endpoint,
			ExporterType:   astrov1.CreateEnvironmentObjectMetricsExportRequestExporterType(in.ExporterType),
			BasicToken:     in.BasicToken,
			Username:       in.Username,
			Password:       in.Password,
			SigV4AssumeArn: in.SigV4AssumeArn,
			SigV4StsRegion: in.SigV4StsRegion,
			Headers:        in.Headers,
			Labels:         in.Labels,
		},
		AutoLinkDeployments: in.AutoLinkDeployments,
	}
	if in.AuthType != "" {
		at := astrov1.CreateEnvironmentObjectMetricsExportRequestAuthType(in.AuthType)
		body.MetricsExport.AuthType = &at
	}

	resp, err := astroV1Client.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	id := resp.JSON200.Id
	metrics := &astrov1.EnvironmentObjectMetricsExport{
		Endpoint:       in.Endpoint,
		ExporterType:   astrov1.EnvironmentObjectMetricsExportExporterType(in.ExporterType),
		BasicToken:     in.BasicToken,
		Username:       in.Username,
		Password:       in.Password,
		SigV4AssumeArn: in.SigV4AssumeArn,
		SigV4StsRegion: in.SigV4StsRegion,
		Headers:        in.Headers,
		Labels:         in.Labels,
	}
	if in.AuthType != "" {
		at := astrov1.EnvironmentObjectMetricsExportAuthType(in.AuthType)
		metrics.AuthType = &at
	}
	return &astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     key,
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.METRICSEXPORT),
		Scope:         astrov1.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		MetricsExport: metrics,
	}, nil
}

// UpdateMetricsExport updates an existing metrics export. All MetricsInput
// fields are treated as a partial update.
func UpdateMetricsExport(idOrKey string, scope Scope, in *MetricsInput, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if err := validateAutoLink(scope, in.AutoLinkDeployments); err != nil {
		return nil, err
	}
	// Fetch the full object (not just the ID): the update body must round-trip
	// the existing Links/ExcludeLinks and auto-link flag or the platform drops
	// them. See echoPreservedFields.
	current, err := getObject(idOrKey, scope, objectTypeMetrics, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if current.Id == nil || *current.Id == "" {
		return nil, fmt.Errorf("environment object %q has no id", idOrKey)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	req := &astrov1.UpdateEnvironmentObjectMetricsExportRequest{
		BasicToken:     in.BasicToken,
		Username:       in.Username,
		Password:       in.Password,
		SigV4AssumeArn: in.SigV4AssumeArn,
		SigV4StsRegion: in.SigV4StsRegion,
		Headers:        in.Headers,
		Labels:         in.Labels,
	}
	if in.Endpoint != "" {
		req.Endpoint = &in.Endpoint
	}
	if in.ExporterType != "" {
		et := astrov1.UpdateEnvironmentObjectMetricsExportRequestExporterType(in.ExporterType)
		req.ExporterType = &et
	}
	if in.AuthType != "" {
		at := astrov1.UpdateEnvironmentObjectMetricsExportRequestAuthType(in.AuthType)
		req.AuthType = &at
	}
	body := astrov1.UpdateEnvironmentObjectJSONRequestBody{
		MetricsExport:       req,
		AutoLinkDeployments: in.AutoLinkDeployments,
	}
	echoPreservedFields(&body, current)

	resp, err := astroV1Client.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("update returned empty response body")
	}
	return resp.JSON200, nil
}

// DeleteMetricsExport deletes a metrics export by ID or key.
func DeleteMetricsExport(idOrKey string, scope Scope, astroV1Client astrov1.APIClient) error {
	return deleteObject(idOrKey, scope, objectTypeMetrics, astroV1Client)
}
