package cloud

import (
	"fmt"
	"io"
	"strings"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	connID             string
	connType           string
	host               string
	login              string
	password           string
	schema             string
	extra              string
	fromDeploymentID   string
	fromDeploymentName string
	toDeploymentID     string
	toDeploymentName   string
	port               int
	varValue           string
	key                string
	slots              int
	includeDeferred    string
)

const (
	webserverURLField        = "deployment.metadata.airflow_api_url"
	warningConnectionCopyCMD = "WARNING! The password and extra field are not copied over. You will need to manually add these values"
	warningVariableCopyCMD   = "WARNING! Secret values are not copied over. You will need to manually add these values"
)

func newDeploymentConnectionRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connection",
		Aliases: []string{"con", "connections"},
		Short:   "Manage deployment connections",
		Long:    "Manage connections for an Astro Deployment.",
	}
	cmd.AddCommand(
		newDeploymentConnectionListCmd(out),
		newDeploymentConnectionCreateCmd(out),
		newDeploymentConnectionUpdateCmd(out),
		newDeploymentConnectionCopyCmd(out),
	)
	return cmd
}

func newDeploymentConnectionListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's connections",
		Long:    "list connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create connections for a Deployment",
		Long:    "Create Airflow connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection ID. Required.")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection type. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection description.")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection host.")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection login or username.")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection password.")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection schema.")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection port.")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The extra field configuration, defined as a stringified JSON object.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update connections for a Deployment",
		Long:    "Update existing Airflow connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection ID. Required.")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection type. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection description.")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection host.")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection login or username.")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection password.")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection schema.")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection port.")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The extra field configuration, defined as a stringified JSON object.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy connections from one Deployment to another",
		Long:    "Copy Airflow connections from one Astro Deployment to another. Passwords and extra configurations will not copy over. If a connection already exits with same connection ID in the target Deployment, that connection will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy connections from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy connections from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied connections")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied connections")

	return cmd
}

func newDeploymentAirflowVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow-variable",
		Aliases: []string{"var"},
		Short:   "Manage Airflow variables in an Astro Deployment",
		Long:    "Manage Airflow variables stored in an Astro Deployment's metadata database.",
	}
	cmd.AddCommand(
		newDeploymentAirflowVariableListCmd(out),
		newDeploymentAirflowVariableCreateCmd(out),
		newDeploymentAirflowVariableUpdateCmd(out),
		newDeploymentAirflowVariableCopyCmd(out),
	)
	return cmd
}

func newDeploymentAirflowVariableListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's Airflow variables",
		Long:    "list Airflow variables stored in an Astro Deployment's metadata database",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create Airflow variables for a Deployment",
		Long:    "Create Airflow variables for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The Airflow variable value. Required.")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The Airflow variable key. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow variable description.")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update Airflow variables for a Deployment",
		Long:    "Update Airflow variables for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The Airflow variable value. Required.")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The Airflow variable key. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow variable description.")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy the Airflow variables from one Deployment to another",
		Long:    "Copy Airflow variables from one Astro Deployment to another Astro Deployment. If a variable already exits with same Key it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow variables from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow variables from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow variables")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow variables")

	return cmd
}

func newDeploymentPoolRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pool",
		Aliases: []string{"pl", "pools"},
		Short:   "Manage Deployment's Airflow pools",
		Long:    "Manage Airflow pools stored for an Astro Deployment",
	}
	cmd.AddCommand(
		newDeploymentPoolListCmd(out),
		newDeploymentPoolCreateCmd(out),
		newDeploymentPoolUpdateCmd(out),
		newDeploymentPoolCopyCmd(out),
	)
	return cmd
}

func newDeploymentPoolListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's Airflow pools",
		Long:    "list Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create Airflow pools for an Astro Deployment",
		Long:    "Create Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "The Airflow pool value. Required")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The Airflow pool key. Required")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow pool description")
	cmd.Flags().StringVarP(&includeDeferred, "include-deferred", "", "", "If set to 'enable', deferred tasks are considered when calculating open pool slots. Default is 'disable'. Possible values are disable/enable.")

	return cmd
}

//nolint:dupl
func newDeploymentPoolUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update Airflow pools for an Astro Deployment",
		Long:    "Update Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "The pool value.  Required.")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The pool slots.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool description.")
	cmd.Flags().StringVarP(&includeDeferred, "include-deferred", "", "", "If set to 'enable', deferred tasks are considered when calculating open pool slots. Required for Airflow 3+ deployments. Possible values disable/enable.")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy Airflow pools from one Astro Deployment to another",
		Long:    "Copy Airflow pools from one Astro Deployment to another Astro Deployment. If a pool already exits with same name it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow pools from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow pools from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow pools")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow pools")

	return cmd
}

//nolint:dupl
func deploymentConnectionList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	return deployment.ConnectionList(airflowURL, airflowAPIClient, out)
}

func deploymentConnectionCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	if connType == "" {
		return errors.New("a connection type is needed to create a connection. Please use the '--conn-type' flag to specify a connection type")
	}

	return deployment.ConnectionCreate(airflowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

func deploymentConnectionUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	return deployment.ConnectionUpdate(airflowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

//nolint:dupl
func deploymentConnectionCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which Deployment should connections be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment Airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which Deployment should receive the connections?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment Airflow webserver URL")
	}

	fmt.Println(warningConnectionCopyCMD)
	return deployment.CopyConnection(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

//nolint:dupl
func deploymentAirflowVariableList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	return deployment.AirflowVariableList(airflowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check key and varValue
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	if varValue == "" {
		return errors.New("a variable value is needed to create an airflow variable. Please use the '--value' flag to specify a value")
	}

	return deployment.VariableCreate(airflowURL, varValue, key, description, airflowAPIClient, out)
}

func deploymentAirflowVariableUpdate(cmd *cobra.Command, out io.Writer) error { //nolint
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check key
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	return deployment.VariableUpdate(airflowURL, varValue, key, description, airflowAPIClient, out)
}

//nolint:dupl
func deploymentAirflowVariableCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be pasted to?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target deployments airflow webserver URL")
	}

	fmt.Println(warningVariableCopyCMD)
	return deployment.CopyVariable(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

//nolint:dupl
func deploymentPoolList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	return deployment.PoolList(airflowURL, airflowAPIClient, out)
}

func deploymentPoolCreate(cmd *cobra.Command, out io.Writer) error { //nolint
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check name
	if name == "" {
		return errors.New("a pool name is needed to create a pool. Please use the '--name' flag to specify a name")
	}

	// check includeDeferred
	var includeDeferredValue bool
	switch includeDeferred {
	case enable:
		includeDeferredValue = true
	case disable, "":
		includeDeferredValue = false
	default:
		return errors.New("Invalid --include-deferred value")
	}

	return deployment.PoolCreate(airflowURL, name, description, slots, includeDeferredValue, airflowAPIClient, out)
}

func deploymentPoolUpdate(cmd *cobra.Command, out io.Writer) error { //nolint
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get or select the deployment
	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return err
	}

	// check name
	if name == "" {
		return errors.New("a pool name is needed to update a pool. Please use the '--name' flag to specify a name")
	}

	// check includeDeferred
	if airflowversions.AirflowMajorVersionForRuntimeVersion(requestedDeployment.RuntimeVersion) >= "3" && includeDeferred == "" {
		return errors.New("an include deferred value is needed to update a pool. Please use the '--include-deferred' flag to specify a value")
	}
	var includeDeferredValue bool
	switch includeDeferred {
	case enable:
		includeDeferredValue = true
	case disable, "":
		includeDeferredValue = false
	default:
		return errors.New("Invalid --include-deferred value")
	}

	return deployment.PoolUpdate(airflowURL, name, description, slots, includeDeferredValue, airflowAPIClient, out)
}

//nolint:dupl
func deploymentPoolCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should pools be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should pools be pasted to?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, platformCoreClient, astroCoreClient)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target deployments airflow webserver URL")
	}

	return deployment.CopyPool(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

func getAirflowURL(depl *astroplatformcore.Deployment) (string, error) {
	value, err := inspect.ReturnSpecifiedValue(depl, webserverURLField, platformCoreClient)
	if err != nil {
		return "", errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airflowURL := fmt.Sprintf("%v", value)
	splitAirflowURL := strings.Split(airflowURL, "?")[0]

	return splitAirflowURL, nil
}
