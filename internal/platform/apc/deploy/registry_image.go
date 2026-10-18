package deploy

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/internal/deploybuild"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// ErrRuntimeVersionNotPassedForRemoteImage is --remote without
// --runtime-version: Houston's updateDeploymentImage takes the image's
// runtime, and an image in a registry has no label here to read it from.
var ErrRuntimeVersionNotPassedForRemoteImage = errors.New("if --image-name and --remote is passed, it's mandatory to pass --runtime-version")

// houstonUpdateImageDisabled is what Houston answers updateDeploymentImage
// with on a platform that has not turned the mutation on (houston-api
// UpdateDeploymentImageDisabledError).
const houstonUpdateImageDisabled = "Update deployment image endpoint is disabled"

// The setting that turns updateDeploymentImage on, as Houston 2.0.0 and
// later, and Houston before it, spell it under astronomer.houston.config.
const (
	updateImageEndpointSetting       = "deployments.deploymentImagesRegistry.updateDeploymentImageEndpoint.enabled"
	updateImageEndpointSettingLegacy = "deployments.enableUpdateDeploymentImageEndpoint"
)

// deployRegistryImage points a Deployment at an image already in a registry
// (--image-name --remote), with no pull and no push: Houston's
// updateDeploymentImage mutation takes the reference, the runtime the image
// carries (--runtime-version) and the deploy's description. The runtime is
// held to the rules a pushed image is (checkRuntime) before the mutation,
// and the DAGs then go as an image deploy's do (dagsAfterImage).
//
// It reads the cluster config only to place the Deployment's DAGs: one it
// cannot read does not stop the deploy, as it never did, and the Deployment
// is then placed by its own type alone.
func deployRegistryImage(client houston.ClientInterface, in *ManifestImage, progress io.Writer, warn func(string)) (ManifestImageResult, error) {
	if in.RuntimeVersion == "" {
		return ManifestImageResult{}, ErrRuntimeVersionNotPassedForRemoteImage
	}
	t, err := lookUpDeployment(client, in.WorkspaceID, in.DeploymentID)
	if err != nil {
		return ManifestImageResult{}, err
	}
	t.appConfig, err = houston.Call(client.GetAppConfig)(houston.GetAppConfigRequest{ClusterID: t.info.ClusterID, WorkspaceUUID: t.workspace, DeploymentUUID: in.DeploymentID})
	if err != nil {
		logger.Debugf("could not read the cluster config to place the Deployment's Dags: %s", err.Error())
		t.appConfig = nil
	}
	t.dags = dagsFrom(t.info, t.appConfig)
	if err := refuseImageOnly(in, t); err != nil {
		return ManifestImageResult{}, err
	}

	listed := listedRuntimes(client, t.info.ClusterID, t.workspace, in.DeploymentID)
	planned := deploybuild.PlanRuntime(in.ImageName, &imagebuild.Request{}, &in.Build)
	current := t.info.RuntimeVersion
	upgrade, err := checkRuntime(in, &planned, current, in.RuntimeVersion, listed.versions(), warn)
	if err != nil {
		return ManifestImageResult{}, err
	}

	fmt.Fprintf(progress, houstonDeploymentPrompt, t.release)
	fmt.Fprintf(progress, "Pointing the Deployment at %s, which is already in its registry: nothing is built or pushed\n", in.ImageName)
	req := houston.UpdateDeploymentImageRequest{ReleaseName: t.release, Image: in.ImageName, RuntimeVersion: in.RuntimeVersion, DeployRevisionDescription: in.Description}
	if _, err := houston.Call(client.UpdateDeploymentImage)(req); err != nil {
		return ManifestImageResult{}, explainUpdateImageErr(err, t.appConfig, true)
	}
	fmt.Fprintln(progress, "Image successfully updated")

	res := ManifestImageResult{
		WorkspaceID:     in.WorkspaceID,
		RuntimeVersion:  in.RuntimeVersion,
		Image:           in.ImageName,
		URL:             getAirflowUILink(in.DeploymentID, t.info.Urls),
		Airflow3Upgrade: upgrade,
	}
	err = dagsAfterImage(client, in, t, &res, progress, warn)
	return res, err
}

// explainUpdateImageErr says what to do about a platform that has not turned
// updateDeploymentImage on, which Houston itself only calls disabled: a
// platform admin turns it on in Houston's config. Any other error is
// returned as it is.
func explainUpdateImageErr(err error, appConfig *houston.AppConfig, remote bool) error {
	if err == nil || !strings.Contains(err.Error(), houstonUpdateImageDisabled) {
		return err
	}
	advice := "Until then, deploy an image on this machine with astro deploy --image-name <image>, without --remote"
	if !remote {
		advice = "A Deployment on a cluster with a registry of the customer's own takes every image through it"
	}
	setting := updateImageEndpointSettingLegacy
	if appConfig != nil && isHoustonPlatformVersionGTE200(appConfig.Version) {
		setting = updateImageEndpointSetting
	}
	return fmt.Errorf("this Astro Private Cloud platform does not let the CLI point a Deployment at an image in a registry: Houston's updateDeploymentImage is turned off. "+
		"A platform admin turns it on by setting %s to true under astronomer.houston.config. "+
		"%s: %w", setting, advice, err)
}
