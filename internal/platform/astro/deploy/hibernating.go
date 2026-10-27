package deploy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// hibernatingError is a deploy refused because the Deployment is hibernating:
// by the status read before the build, whose refusal is
// instancelocate.ErrDeploymentHibernating, or by Astro when the deploy was
// created, whose refusal is its *httputil.StatusError.
type hibernatingError struct {
	deploymentID string
	name         string
	refusal      error
}

// Error keeps Astro's own text when the refusal does not itself name
// hibernation (certain), since then only the earlier status read did, and the
// refusal may be for another reason.
func (e *hibernatingError) Error() string {
	named := e.deploymentID
	// A caller that knew only the id (a bundle delete named by id) has no
	// name to add.
	if e.name != "" {
		named = fmt.Sprintf("%s (%q)", e.deploymentID, e.name)
	}
	hint := fmt.Sprintf("Astro Deployment %s is hibernating, so it cannot take a deploy — %s, then deploy again",
		named, instancelocate.WakeUpStep(e.deploymentID))
	if e.certain() {
		return hint
	}
	return hint + "\n" + e.refusal.Error()
}

func (e *hibernatingError) Unwrap() error { return e.refusal }

// Is makes a refusal that itself names hibernation (certain)
// instancelocate.ErrDeploymentHibernating, the sentinel behind the
// deployment_hibernating problem kind, so --output json names the kind. One
// that does not keeps the kind of its own status, a 403 its forbidden, since
// waking the Deployment would not clear it.
func (e *hibernatingError) Is(target error) bool {
	return target == instancelocate.ErrDeploymentHibernating && e.certain()
}

// certain reports whether the refusal itself names hibernation: the status
// read before the build, or Astro's text. Otherwise only the earlier status
// read said so, and the refusal may be for another reason.
func (e *hibernatingError) certain() bool {
	return errors.Is(e.refusal, instancelocate.ErrDeploymentHibernating) || namesHibernation(e.refusal)
}

func namesHibernation(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "HIBERNATING")
}

// explainHibernating turns Astro's refusal to create a deploy into the wake-up
// step when the Deployment is hibernating. Astro's error body carries no code,
// so either the status read before the deploy or the refusal's text decides.
func explainHibernating(err error, dep *astrov1.Deployment) error {
	var refusal *httputil.StatusError
	if !errors.As(err, &refusal) {
		return err
	}
	if dep.Status != astrov1.DeploymentStatusHIBERNATING && !namesHibernation(refusal) {
		return err
	}
	return &hibernatingError{deploymentID: dep.Id, name: dep.Name, refusal: err}
}
