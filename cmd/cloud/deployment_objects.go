package cloud

import (
	"fmt"
	"io"

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
)

const requestString = "metadata.webserver_url"

func newDeploymentConnectionRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connection",
		Aliases: []string{"con"},
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
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to list connections from.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to list connections from")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create connections for a Deployment",
		Long:    "Create connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to create connections for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to create connections for")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection's connection ID. Required to create a connection")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection's connection type. Required to create a connection")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection's description")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection's host")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection's login or username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection's password")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection's schema")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection's port")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The connection's extra field. Pass a stringified json object to this flag")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update connections for a Deployment",
		Long:    "Update connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to update connections for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to update connections for")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection's connection ID. Required to update a connection")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection's connection type")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection's description")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection's host")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection's login or username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection's password")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection's schema")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection's port")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The connection's extra field. Pass a stringified json object to this flag")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy the connections from one Deployment to another",
		Long:    "Copy connections from one Astro Deployment to another Astro Deployment. The password and extra fields will not copy over. If a connection already exits with same connection ID it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The deployment id to copy connections from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "Name of the deployment to copy connections from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The id of the deployment where copied connnections are created or updated")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "Name of the deployment where copied connections are created or updated")

	return cmd
}

func newDeploymentAirflowVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow-variable",
		Aliases: []string{"var"},
		Short:   "Manage deployment's airflow variables",
		Long:    "Manage airflow variables stored in an Astro Deployment's metadata database.",
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
		Short:   "list a Deployment's airflow variables",
		Long:    "list airflow variables stored in an Astro Deployment's Airflow metadata database",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to list airflow variables from.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to list airflow variables from")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create airflow variables for a Deployment",
		Long:    "Create airflow variables for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to create airflow variables for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to create airflow variables for")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The airflow variables's value. Required to create an airflow variable")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The airflow variables's key. Required to create an airflow variable")
	cmd.Flags().StringVarP(&description, "description", "", "", "The airflow variable's description")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update airflow variables for a Deployment",
		Long:    "Update airflow variables for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to update airflow variables for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to update airflow variables for")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The airflow variables's value")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The airflow variables's key. Required to update an airflow variable")
	cmd.Flags().StringVarP(&description, "description", "", "", "The airflow variable's description")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy the airflow variables from one Deployment to another",
		Long:    "Copy airflow variables from one Astro Deployment to another Astro Deployment. If a variable already exits with same Key it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The deployment id to copy airflow variables from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "Name of the deployment to copy airflow variables from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The id of the deployment where copied airflow variables are created or updated")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "Name of the deployment where copied airflow variables are created or updated")

	return cmd
}

func newDeploymentPoolRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pool",
		Aliases: []string{"pl"},
		Short:   "Manage deployment's pools",
		Long:    "Manage pools stored for an Astro Deployment",
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
		Short:   "list a Deployment's Pools",
		Long:    "list pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to list pools from.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to list pools from")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create pools for a Deployment",
		Long:    "Create pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to create pools for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to create pools for")
	cmd.Flags().StringVarP(&name, "name", "", "", "The pools's value. Required to create an pool")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The pools's key. Required to create an pool")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool's description")

	return cmd
}

//nolint:dupl
func newDeploymentPoolUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update pools for a Deployment",
		Long:    "Update pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The deployment id to update pools for")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to update pools for")
	cmd.Flags().StringVarP(&name, "name", "", "", "The pools's value")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The pools's key. Required to update an pool")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool's description")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy the pools from one Deployment to another",
		Long:    "Copy pools from one Astro Deployment to another Astro Deployment. If a pool already exits with same name it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The deployment id to copy pools from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "Name of the deployment to copy pools from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The id of the deployment where copied pools are created or updated")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "Name of the deployment where copied pools are created or updated")

	return cmd
}

func deploymentConnectionList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.ConnectionList(airlfowURL, airflowAPIClient, out)
}

func deploymentConnectionCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	if connType == "" {
		return errors.New("a connection type is needed to create a connection. Please use the '--conn-type' flag to specify a connection type")
	}

	return deployment.ConnectionCreate(airlfowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

func deploymentConnectionUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	return deployment.ConnectionUpdate(airlfowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

//nolint:dupl
func deploymentConnectionCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should connections be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should connections be pasted to?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)

	return deployment.CopyConnection(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.AirflowVariableList(airlfowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key and varValue
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	if varValue == "" {
		return errors.New("a variable value is needed to create an airflow variable. Please use the '--value' flag to specify a value")
	}

	return deployment.VariableCreate(airlfowURL, varValue, key, description, airflowAPIClient, out)
}

func deploymentAirflowVariableUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	return deployment.VariableUpdate(airlfowURL, varValue, key, description, airflowAPIClient, out)
}

//nolint:dupl
func deploymentAirflowVariableCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be pasted to?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)

	return deployment.CopyVariable(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}

func deploymentPoolList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.PoolList(airlfowURL, airflowAPIClient, out)
}

func deploymentPoolCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check name
	if name == "" {
		return errors.New("a pool name is needed to create a pool. Please use the '--name' flag to specify a name")
	}

	return deployment.PoolCreate(airlfowURL, name, description, slots, airflowAPIClient, out)
}

func deploymentPoolUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key
	if name == "" {
		return errors.New("a pool name is needed to update a pool. Please use the '--name' flag to specify a name")
	}

	return deployment.PoolUpdate(airlfowURL, name, description, slots, airflowAPIClient, out)
}

//nolint:dupl
func deploymentPoolCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should pools be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should pools be pasted to?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, astroClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)

	return deployment.CopyPool(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}
