package env

import (
	httpcontext "context"
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeMetrics = astrocore.METRICSEXPORT

// MetricsInput is the user-supplied data needed to create or update a metrics export.
// Endpoint and ExporterType are required on create.
type MetricsInput struct {
	Endpoint       string
	ExporterType   string
	AuthType       string
	BasicToken     *string
	Username       *string
	Password       *string
	SigV4AssumeArn *string
	SigV4StsRegion *string
	Headers        *map[string]string
	Labels         *map[string]string
}

// ListMetricsExports returns METRICS_EXPORT objects for the given scope.
func ListMetricsExports(scope Scope, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	return listObjects(scope, objectTypeMetrics, resolveLinked, includeSecrets, coreClient)
}

// GetMetricsExport fetches a single metrics export by ID or key.
func GetMetricsExport(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeMetrics, includeSecrets, coreClient)
}

// CreateMetricsExport creates a new METRICS_EXPORT object.
func CreateMetricsExport(scope Scope, key string, in *MetricsInput, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
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
	body := astrocore.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrocore.CreateEnvironmentObjectRequestObjectTypeMETRICSEXPORT,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		MetricsExport: &astrocore.CreateEnvironmentObjectMetricsExportRequest{
			Endpoint:       in.Endpoint,
			ExporterType:   astrocore.CreateEnvironmentObjectMetricsExportRequestExporterType(in.ExporterType),
			BasicToken:     in.BasicToken,
			Username:       in.Username,
			Password:       in.Password,
			SigV4AssumeArn: in.SigV4AssumeArn,
			SigV4StsRegion: in.SigV4StsRegion,
			Headers:        in.Headers,
			Labels:         in.Labels,
		},
	}
	if in.AuthType != "" {
		at := astrocore.CreateEnvironmentObjectMetricsExportRequestAuthType(in.AuthType)
		body.MetricsExport.AuthType = &at
	}

	resp, err := coreClient.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return followCreate(resp.JSON200.Id, coreClient)
}

// UpdateMetricsExport updates an existing metrics export. All MetricsInput
// fields are treated as a partial update.
func UpdateMetricsExport(idOrKey string, scope Scope, in *MetricsInput, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	id, err := resolveID(idOrKey, scope, objectTypeMetrics, coreClient)
	if err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	req := &astrocore.UpdateEnvironmentObjectMetricsExportRequest{
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
		et := astrocore.UpdateEnvironmentObjectMetricsExportRequestExporterType(in.ExporterType)
		req.ExporterType = &et
	}
	if in.AuthType != "" {
		at := astrocore.UpdateEnvironmentObjectMetricsExportRequestAuthType(in.AuthType)
		req.AuthType = &at
	}
	body := astrocore.UpdateEnvironmentObjectJSONRequestBody{MetricsExport: req}

	resp, err := coreClient.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id, body)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("update returned empty response body")
	}
	return resp.JSON200, nil
}

// DeleteMetricsExport deletes a metrics export by ID or key.
func DeleteMetricsExport(idOrKey string, scope Scope, coreClient astrocore.CoreClient) error {
	return deleteObject(idOrKey, scope, objectTypeMetrics, coreClient)
}
