package deployment

import (
	"errors"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// HibernationSince is the first Houston that takes a hibernation override:
// the gate Call applies to the mutation, checked here before the question.
const HibernationSince = houston.HibernationOverrideSince

var (
	errHibernationNotServed = errors.New("hibernating or waking up a Deployment needs Astro Private Cloud " + HibernationSince + " or later")
	errHibernationDisabled  = errors.New("hibernation is not enabled for this Deployment. " +
		"A platform admin turns it on with deployments.hibernation.enabled, for the platform, or for the Deployment's cluster, Workspace or the Deployment itself")
)

// CheckHibernation reports whether the Deployment can take a hibernation
// override, before anything asks the user to confirm one: the platform must
// serve the override, and hibernation must be on under the settings that apply
// to the Deployment. Houston merges those settings per cluster, Workspace and
// Deployment, so they are read for this Deployment rather than taken from the
// platform-wide answer the command tree was built with.
//
// A Houston that does not report hibernationEnabled (2.2.0 itself) is let
// through, and upsertDeployment refuses there when hibernation is off.
func CheckHibernation(id string, client houston.ClientInterface) error {
	d, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}
	cfg, err := houston.Call(client.GetAppConfig)(houston.GetAppConfigRequest{
		ClusterID:      d.ClusterID,
		WorkspaceUUID:  d.Workspace.ID,
		DeploymentUUID: id,
	})
	if err != nil {
		return fmt.Errorf("failed to get app config: %w", err)
	}
	if cfg == nil {
		return nil
	}
	if cfg.Version != "" && !houston.VerifyVersionMatch(cfg.Version, houston.VersionRestrictions{GTE: HibernationSince}) {
		return errHibernationNotServed
	}
	if enabled := cfg.Flags.HibernationEnabled; enabled != nil && !*enabled {
		return errHibernationDisabled
	}
	return nil
}

// SetHibernationOverride holds the Deployment hibernating, or awake, against
// its hibernation schedule until until, or until the override is removed when
// until is nil. It returns the Deployment as Houston now has it, override
// included.
func SetHibernationOverride(id string, hibernate bool, until *time.Time, client houston.ClientInterface) (*houston.Deployment, error) {
	override := map[string]interface{}{"hibernate": hibernate}
	if until != nil {
		override["overrideUntil"] = until.UTC().Format(time.RFC3339)
	}
	return hibernationOverride(map[string]interface{}{"deploymentId": id, "hibernationOverride": override}, client)
}

// RemoveHibernationOverride removes the Deployment's override, so its
// hibernation schedule, if it has one, applies again.
func RemoveHibernationOverride(id string, client houston.ClientInterface) (*houston.Deployment, error) {
	// A null hibernationOverride clears the override; an absent one would
	// leave it as it is.
	return hibernationOverride(map[string]interface{}{"deploymentId": id, "hibernationOverride": nil}, client)
}

func hibernationOverride(vars map[string]interface{}, client houston.ClientInterface) (*houston.Deployment, error) {
	d, err := houston.Call(client.HibernateOverrideDeployment)(vars)
	var notServed houston.ErrAPINotImplemented
	if errors.As(err, &notServed) {
		return nil, errHibernationNotServed
	}
	return d, err
}
