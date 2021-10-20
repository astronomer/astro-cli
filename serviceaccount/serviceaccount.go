package deployment

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
func CreateUsingDeploymentUUID(deploymentUUID, label, category, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.CreateDeploymentServiceAccountRequest,
		Variables: map[string]interface{}{
			"label":          label,
			"category":       category,
			"deploymentUuid": deploymentUUID,
			"role":           role,
		},
	}
	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	sa := resp.Data.CreateDeploymentServiceAccount
	tab := newTableOut()
	tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	tab.SuccessMsg = serviceAccountSuccessMsg

	return tab.Print(out)
}

// nolint:dupl
func CreateUsingWorkspaceUUID(workspaceUUID, label, category, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.CreateWorkspaceServiceAccountRequest,
		Variables: map[string]interface{}{
			"label":         label,
			"category":      category,
			"workspaceUuid": workspaceUUID,
			"role":          role,
		},
	}
	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	sa := resp.Data.CreateWorkspaceServiceAccount
	tab := newTableOut()
	tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	tab.SuccessMsg = serviceAccountSuccessMsg

	return tab.Print(out)
}

func DeleteUsingWorkspaceUUID(serviceAccountID, workspaceID string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceServiceAccountDeleteRequest,
		Variables: map[string]interface{}{"serviceAccountUuid": serviceAccountID, "workspaceUuid": workspaceID},
	}

	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	sa := resp.Data.DeleteWorkspaceServiceAccount

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.ID)
	fmt.Fprintln(out, msg)

	return nil
}

func DeleteUsingDeploymentUUID(serviceAccountID, deploymentID string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.DeploymentServiceAccountDeleteRequest,
		Variables: map[string]interface{}{"serviceAccountUuid": serviceAccountID, "deploymentUuid": deploymentID},
	}

	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	sa := resp.Data.DeleteDeploymentServiceAccount

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.ID)
	fmt.Fprintln(out, msg)

	return nil
}

// get all deployment service accounts
func GetDeploymentServiceAccounts(id string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.DeploymentServiceAccountsGetRequest,
		Variables: map[string]interface{}{"deploymentUuid": id},
	}

	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	sas := resp.Data.GetDeploymentServiceAccounts
	fmt.Print(len(sas))
	tab := newTableOut()
	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	}

	return tab.Print(out)
}

// get all workspace service accounts
func GetWorkspaceServiceAccounts(id string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceServiceAccountsGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": id},
	}

	resp, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	sas := resp.Data.GetWorkspaceServiceAccounts
	tab := newTableOut()
	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.ID, sa.APIKey}, false)
	}

	return tab.Print(out)
}
