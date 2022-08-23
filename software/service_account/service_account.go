package serviceaccount

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var serviceAccountSuccessMsg = "\n Service account successfully created."

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{40, 40, 50, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "CATEGORY", "ID", "APIKEY"},
	}
}

// nolint:dupl
func CreateUsingDeploymentUUID(deploymentUUID, label, category, role string, client houston.ClientInterface, out io.Writer) error {
	createServiceAccountRequest := &houston.CreateServiceAccountRequest{
		DeploymentID: deploymentUUID,
		Label:        label,
		Category:     category,
		Role:         role,
	}
	sa, err := houston.Call(client.CreateDeploymentServiceAccount, createServiceAccountRequest)
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	tab.SuccessMsg = serviceAccountSuccessMsg

	return tab.Print(out)
}

// nolint:dupl
func CreateUsingWorkspaceUUID(workspaceUUID, label, category, role string, client houston.ClientInterface, out io.Writer) error {
	request := &houston.CreateServiceAccountRequest{
		WorkspaceID: workspaceUUID,
		Label:       label,
		Category:    category,
		Role:        role,
	}
	sa, err := houston.Call(client.CreateWorkspaceServiceAccount, request)
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	tab.SuccessMsg = serviceAccountSuccessMsg

	return tab.Print(out)
}

func DeleteUsingWorkspaceUUID(serviceAccountID, workspaceID string, client houston.ClientInterface, out io.Writer) error {
	sa, err := houston.Call(client.DeleteWorkspaceServiceAccount, houston.DeleteServiceAccountRequest{ServiceAccountID: serviceAccountID, WorkspaceID: workspaceID})
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.ID)
	fmt.Fprintln(out, msg)

	return nil
}

func DeleteUsingDeploymentUUID(serviceAccountID, deploymentID string, client houston.ClientInterface, out io.Writer) error {
	sa, err := houston.Call(client.DeleteDeploymentServiceAccount, houston.DeleteServiceAccountRequest{DeploymentID: deploymentID, ServiceAccountID: serviceAccountID})
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.ID)
	fmt.Fprintln(out, msg)

	return nil
}

// get all deployment service accounts
func GetDeploymentServiceAccounts(id string, client houston.ClientInterface, out io.Writer) error {
	sas, err := houston.Call(client.ListDeploymentServiceAccounts, id)
	if err != nil {
		return err
	}

	tab := newTableOut()
	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	}

	return tab.Print(out)
}

// get all workspace service accounts
func GetWorkspaceServiceAccounts(id string, client houston.ClientInterface, out io.Writer) error {
	sas, err := houston.Call(client.ListWorkspaceServiceAccounts, id)
	if err != nil {
		return err
	}

	tab := newTableOut()
	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	}

	return tab.Print(out)
}
