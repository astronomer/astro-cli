package serviceaccount

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// errNoServiceAccount is a create or a delete Houston answered with no
// service account and no error. The schema allows it (each mutation returns
// a nullable ServiceAccount), though the resolvers answer with the row.
var errNoServiceAccount = errors.New("the platform answered without the service account")

// ServiceAccount is a Workspace or Deployment service account as Houston
// returns it. APIKey is the full key only when the account was just created.
type ServiceAccount struct {
	ID         string
	Label      string
	Category   string
	APIKey     string
	Active     bool
	CreatedAt  string
	LastUsedAt string
}

// convert is the one conversion: the Workspace and Deployment answers of a
// create carry the same ServiceAccount, embedded, and a list and a delete
// answer with it bare.
func convert(sa *houston.ServiceAccount) ServiceAccount {
	return ServiceAccount{
		ID: sa.ID, Label: sa.Label, Category: sa.Category, APIKey: sa.APIKey,
		Active: sa.Active, CreatedAt: sa.CreatedAt, LastUsedAt: sa.LastUsedAt,
	}
}

// fromHouston is a create's or a delete's answer, refused when there is none.
func fromHouston(sa *houston.ServiceAccount) (ServiceAccount, error) {
	if sa == nil {
		return ServiceAccount{}, errNoServiceAccount
	}
	return convert(sa), nil
}

// CreateUsingDeploymentUUID creates a service account with role on the
// Deployment, and returns it with its API key.
func CreateUsingDeploymentUUID(deploymentUUID, label, category, role string, client houston.ClientInterface) (ServiceAccount, error) {
	sa, err := houston.Call(client.CreateDeploymentServiceAccount)(&houston.CreateServiceAccountRequest{
		DeploymentID: deploymentUUID,
		Label:        label,
		Category:     category,
		Role:         role,
	})
	if err != nil {
		return ServiceAccount{}, err
	}
	if sa == nil {
		return ServiceAccount{}, errNoServiceAccount
	}
	return convert(&sa.ServiceAccount), nil
}

// CreateUsingWorkspaceUUID creates a service account with role on the
// Workspace, and returns it with its API key.
func CreateUsingWorkspaceUUID(workspaceUUID, label, category, role string, client houston.ClientInterface) (ServiceAccount, error) {
	sa, err := houston.Call(client.CreateWorkspaceServiceAccount)(&houston.CreateServiceAccountRequest{
		WorkspaceID: workspaceUUID,
		Label:       label,
		Category:    category,
		Role:        role,
	})
	if err != nil {
		return ServiceAccount{}, err
	}
	if sa == nil {
		return ServiceAccount{}, errNoServiceAccount
	}
	return convert(&sa.ServiceAccount), nil
}

// DeleteUsingWorkspaceUUID deletes a Workspace's service account, and returns
// what it deleted.
func DeleteUsingWorkspaceUUID(serviceAccountID, workspaceID string, client houston.ClientInterface) (ServiceAccount, error) {
	sa, err := houston.Call(client.DeleteWorkspaceServiceAccount)(houston.DeleteServiceAccountRequest{ServiceAccountID: serviceAccountID, WorkspaceID: workspaceID})
	if err != nil {
		return ServiceAccount{}, err
	}
	return fromHouston(sa)
}

// DeleteUsingDeploymentUUID deletes a Deployment's service account, and
// returns what it deleted.
func DeleteUsingDeploymentUUID(serviceAccountID, deploymentID string, client houston.ClientInterface) (ServiceAccount, error) {
	sa, err := houston.Call(client.DeleteDeploymentServiceAccount)(houston.DeleteServiceAccountRequest{DeploymentID: deploymentID, ServiceAccountID: serviceAccountID})
	if err != nil {
		return ServiceAccount{}, err
	}
	return fromHouston(sa)
}

// GetDeploymentServiceAccounts returns a Deployment's service accounts.
func GetDeploymentServiceAccounts(id string, client houston.ClientInterface) ([]ServiceAccount, error) {
	sas, err := houston.Call(client.ListDeploymentServiceAccounts)(id)
	if err != nil {
		return nil, err
	}
	return list(sas), nil
}

// GetWorkspaceServiceAccounts returns a Workspace's service accounts.
func GetWorkspaceServiceAccounts(id string, client houston.ClientInterface) ([]ServiceAccount, error) {
	sas, err := houston.Call(client.ListWorkspaceServiceAccounts)(id)
	if err != nil {
		return nil, err
	}
	return list(sas), nil
}

func list(sas []houston.ServiceAccount) []ServiceAccount {
	out := make([]ServiceAccount, 0, len(sas))
	for i := range sas {
		out = append(out, convert(&sas[i]))
	}
	return out
}
