package deploy

import (
	"errors"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

type hibernatingError struct {
	deploymentID string
	name         string
	refusal      error
}

// Error keeps Astro's own text when it does not name hibernation, since then
// only the earlier status read did, and the refusal may be for another reason.
func (e *hibernatingError) Error() string {
	named := e.deploymentID
	// A caller that knew only the id (a bundle delete named by id) has no
	// name to add.
	if e.name != "" {
		named = fmt.Sprintf("%s (%q)", e.deploymentID, e.name)
	}
	hint := fmt.Sprintf("Astro Deployment %s is hibernating, so it cannot take a deploy — wake it with `astro deployment wake-up %s`, which takes about a minute, then deploy again",
		named, e.deploymentID)
	if namesHibernation(e.refusal) {
		return hint
	}
	return hint + "\n" + e.refusal.Error()
}

func (e *hibernatingError) Unwrap() error { return e.refusal }

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
